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
	list *bool = flag.Bool("l", false, "List public (deposit) addressses and save them to wallet.txt file")
	singleask *bool = flag.Bool("1", false, "Do not re-ask for the password (when used along with -l)")
	noverify *bool = flag.Bool("q", false, "Do not double check keys while listing them (use with -l)")
	verbose *bool = flag.Bool("v", false, "Verbose version (print more info)")
	ask4pass *bool = flag.Bool("p", false, "Force the wallet to ask for seed password (ignore .secret file)")
	nosseed *bool = flag.Bool("is", false, "Ignore the seed paremeter from the config file")
	subfee *bool = flag.Bool("f", false, "Substract fee from the first value")

	dumppriv *string = flag.String("dump", "", "Export a private key of a given address (use * for all)")

	// Spending money options
	send *string  = flag.String("send", "", "Send money as defined by the list of pairs: address1=amount1[,address2=amount2]")
	batch *string  = flag.String("batch", "", "Send money as defined by the content of the given file (each line: address=amount)")
	change *string  = flag.String("change", "", "Send any change to this address (otherwise return it to the 1st input)")

	// Message signing options
	signaddr *string  = flag.String("sign", "", "Perform sign operation with the given P2KH address (use with -msg or -hash)")
	message *string  = flag.String("msg", "", "Specify text message to be signed or added as transaction's extra OP_RETURN output")

	useallinputs *bool = flag.Bool("useallinputs", false, "Use all the current balance's unspent outputs as the transaction inputs")

	// Sign raw TX
	rawtx *string  = flag.String("raw", "", "Sign a raw transaction (specify filename as the parameter)")

	// Decode raw tx
	dumptxfn *string  = flag.String("d", "", "Decode a raw transaction (specify filename as the parameter)")

	// Sign raw message
	signhash *string  = flag.String("hash", "", "Sign a raw hash value (use together with -sign parameter)")

	// Print a public key of a give bitcoin address
	pubkey *string  = flag.String("pub", "", "Print the public key of the given P2KH address")

	// Print a public key of a give bitcoin address
	p2sh *string  = flag.String("p2sh", "", "Insert given P2SH script into the transaction (use with -raw and optionally -input)")
	input *int  = flag.Int("input", -1, "Insert P2SH script only at this input number (use with -p2sh)")
	multisign *string  = flag.String("msign", "", "Sign multisig transaction with given bitcoin address (use with -raw)")
	allowextramsigns *bool = flag.Bool("xtramsigs", false, "Allow to put more signatures than needed (for multisig txs)")

	sequence *int = flag.Int("seq", 0, "Use given Replace-By-Fee sequence number (-1 or -2 for final)")

	segwit_mode *bool = flag.Bool("segwit", false, "List SegWit deposit addresses (instead of P2KH)")
	bech32_mode *bool = flag.Bool("bech32", false, "use with -segwit to see P2WPKH deposit addresses (instead of P2SH-WPKH)")
	
    dumpxprv *bool  = flag.Bool("xprv", false, "Print HD wallet's private seed and exit")
)


// cleanExit exits after cleaning up private data from memory.
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

	flag.BoolVar(list, "list", false, "Same as -l (above)")

	parse_config()

	flag.Parse() // this one will print defaults and exit in case of any unknown switches (like -h)

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
	if *pubkey!="" {
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
		fmt.Println("Failed to load wallet's balance data. Execute 'wallet -h' for help.")
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
