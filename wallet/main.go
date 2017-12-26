package main

import (
	"os"
	"fmt"
	"flag"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin"
	"github.com/piotrnar/gocoin/lib/others/sys"
)

var (
	PassSeedFilename = ".secret"
	RawKeysFilename = ".others"
)

var (
	// Command line switches

	// Wallet options
	list *bool = flag.Bool("l", false, "List public addressses from the wallet")
	singleask *bool = flag.Bool("1", false, "Do not re-ask for the password (when used along with -l)")
	noverify *bool = flag.Bool("q", false, "Do not verify keys while listing them")
	verbose *bool = flag.Bool("v", false, "Verbose version (print more info)")
	ask4pass *bool = flag.Bool("p", false, "Force the wallet to ask for seed password")
	nosseed *bool = flag.Bool("is", false, "Ignore seed from the config file")
	subfee *bool = flag.Bool("f", false, "Substract fee from the first value")

	dumppriv *string = flag.String("dump", "", "Export a private key of a given address (use * for all)")

	// Spending money options
	send *string  = flag.String("send", "", "Send money to list of comma separated pairs: address=amount")
	batch *string  = flag.String("batch", "", "Send money as per the given batch file (each line: address=amount)")
	change *string  = flag.String("change", "", "Send any change to this address (otherwise return to 1st input)")

	// Message signing options
	signaddr *string  = flag.String("sign", "", "Request a sign operation with a given bitcoin address")
	message *string  = flag.String("msg", "", "Message to be signed or included into transaction")

	useallinputs *bool = flag.Bool("useallinputs", false, "Use all the unspent outputs as the transaction inputs")

	// Sign raw TX
	rawtx *string  = flag.String("raw", "", "Sign a raw transaction (use hex-encoded string)")

	// Decode raw tx
	dumptxfn *string  = flag.String("d", "", "Decode raw transaction from the specified file")

	// Sign raw message
	signhash *string  = flag.String("hash", "", "Sign a raw hash (use together with -sign parameter)")

	// Print a public key of a give bitcoin address
	pubkey *string  = flag.String("pub", "", "Print public key of the give bitcoin address")

	// Print a public key of a give bitcoin address
	p2sh *string  = flag.String("p2sh", "", "Insert P2SH script into each transaction input (use together with -raw)")
	input *int  = flag.Int("input", -1, "Insert P2SH script only at this intput number (-1 for all inputs)")
	multisign *string  = flag.String("msign", "", "Sign multisig transaction with given bitcoin address (use with -raw)")
	allowextramsigns *bool = flag.Bool("xtramsigs", false, "Allow to put more signatures than needed (for multisig txs)")

	sequence *int = flag.Int("seq", 0, "Use given RBF sequence number (-1 or -2 for final)")

	segwit_mode *bool = flag.Bool("segwit", false, "List SegWit deposit addresses (instead of P2KH)")
	bech32_mode *bool = flag.Bool("bech32", false, "use with -segwit to see P2WPKH deposit addresses (instead of P2SH-WPKH)")
)


// exit after cleaning up private data from memory
func cleanExit(code int) {
	if *verbose {
		fmt.Println("Cleaning up private keys")
	}
	for k := range keys {
		sys.ClearBuffer(keys[k].Key)
	}
	if type2_secret != nil {
		sys.ClearBuffer(type2_secret)
	}
	os.Exit(code)
}


func main() {
	// Print the logo to stderr
	println("Gocoin Wallet version", gocoin.Version)
	println("This program comes with ABSOLUTELY NO WARRANTY")
	println()

	parse_config()
	if flag.Lookup("h") != nil {
		flag.PrintDefaults()
		os.Exit(0)
	}

	flag.Parse()

	if uncompressed {
		println("For SegWit address safety, uncompressed keys are disabled in this version")
		os.Exit(1)
	}

	// convert string fee to uint64
	if val, e := btc.StringToSatoshis(fee); e != nil {
		println("Incorrect fee value", fee)
		os.Exit(1)
	} else {
		curFee = val
	}

	// decode raw transaction?
	if *dumptxfn!="" {
		dump_raw_tx()
		return
	}

	// dump public key or secret scan key?
	if *pubkey!="" || *scankey!="" {
		make_wallet()
		cleanExit(0)
	}

	// list public addresses?
	if *list {
		make_wallet()
		dump_addrs()
		cleanExit(0)
	}

	// dump privete key?
	if *dumppriv!="" {
		make_wallet()
		dump_prvkey()
		cleanExit(0)
	}

	// sign a message or a hash?
	if *signaddr!="" {
		make_wallet()
		sign_message()
		if *send=="" {
			// Don't load_balance if he did not want to spend coins as well
			cleanExit(0)
		}
	}

	// raw transaction?
	if *rawtx!="" {
		// add p2sh sript to it?
		if *p2sh!="" {
			make_p2sh()
			cleanExit(0)
		}

		make_wallet()

		// multisig sign with a specific key?
		if *multisign!="" {
			multisig_sign()
			cleanExit(0)
		}

		// this must be signing of a raw trasnaction
		load_balance()
		process_raw_tx()
		cleanExit(0)
	}

	// make the wallet nad print balance
	make_wallet()
	if e := load_balance(); e != nil {
		fmt.Println("ERROR:", e.Error())
		cleanExit(1)
	}

	// send command?
	if send_request() {
		make_signed_tx()
		cleanExit(0)
	}

	show_balance()
	cleanExit(0)
}
