package main

import (
	"os"
	"fmt"
	"flag"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib"
	"github.com/piotrnar/gocoin/lib/others/sys"
)

var (
	PassSeedFilename = ".secret"
	RawKeysFilename = ".others"
)

var (
	// Command line switches

	// Wallet options
	dump *bool = flag.Bool("l", false, "List public addressses from the wallet")
	singleask *bool = flag.Bool("1", false, "Do not re-ask for the password (when used along with -l)")
	noverify *bool = flag.Bool("q", false, "Do not verify keys while listing them")
	verbose *bool = flag.Bool("v", false, "Verbose version (print more info)")
	ask4pass *bool = flag.Bool("p", false, "Force the wallet to ask for seed password")
	nosseed *bool = flag.Bool("is", false, "Ignore seed from the config file")
	onlvalid *bool = flag.Bool("o", false, "Process only P2KH outputs that you have a key for")
	subfee *bool = flag.Bool("f", false, "Substract fee from the first value")

	dumppriv *string = flag.String("dump", "", "Export a private key of a given address (use * for all)")

	// Spending money options
	send *string  = flag.String("send", "", "Send money to list of comma separated pairs: address=amount")
	batch *string  = flag.String("batch", "", "Send money as per the given batch file (each line: address=amount)")
	change *string  = flag.String("change", "", "Send any change to this address (otherwise return to 1st input)")

	// Message signing options
	signaddr *string  = flag.String("sign", "", "Request a sign operation with a given bitcoin address")
	message *string  = flag.String("msg", "", "Defines a message to be signed (otherwise take it from stdin)")

	useallinputs *bool = flag.Bool("useallinputs", false, "Use all the unspent outputs as the transaction inputs")

	// Sign raw TX
	rawtx *string  = flag.String("raw", "", "Sign a raw transaction (use hex-encoded string)")
	hashes *bool = flag.Bool("hashes", false, "Instead of signing, just print hashes to be signed")

	// Decode raw tx
	dumptxfn *string  = flag.String("d", "", "Decode raw transaction from the specified file")

	// Sign raw message
	signhash *string  = flag.String("hash", "", "Sign a raw hash (use together with -sign parameter)")

	// Print a public key of a give bitcoin address
	pubkey *string  = flag.String("pub", "", "Print public key of the give bitcoin address")

	// Print a public key of a give bitcoin address
	p2sh *string  = flag.String("p2sh", "", "Insert P2SH script into each transaction input (use together with -raw)")
	multisign *string  = flag.String("msign", "", "Sign multisig transaction with given bitcoin address (use with -raw)")
	allowextramsigns *bool = flag.Bool("xtramsigs", false, "Allow to put more signatures than needed (for multisig txs)")

	// set in parse_spend():
	spendBtc, feeBtc, changeBtc uint64
	sendTo []oneSendTo
)


func main() {
	// Print the logo to stderr
	println("Gocoin Wallet version", lib.Version)
	println("This program comes with ABSOLUTELY NO WARRANTY")
	println()

	parse_config()
	if flag.Lookup("h") != nil {
		flag.PrintDefaults()
		os.Exit(0)
	}

	flag.Parse()

	if val, e := btc.StringToSatoshis(fee); e != nil {
		println("Incorrect fee value", fee)
		os.Exit(1)
	} else {
		curFee = val
	}

	if *dumptxfn!="" {
		//load_balance(false)
		dump_raw_tx()
		return
	}

	defer func() {
		// cleanup private keys in RAM before exiting
		if *verbose {
			fmt.Println("Cleaning up private keys")
		}
		for k := range keys {
			sys.ClearBuffer(keys[k].Key)
		}
		sys.ClearBuffer(type2_secret)
	}()

	if *pubkey!="" || *scankey!="" {
		make_wallet()
		return
	}

	if *dump {
		make_wallet()
		dump_addrs()
		return
	}

	if *dumppriv!="" {
		make_wallet()
		dump_prvkey()
		return
	}

	if *signaddr!="" {
		make_wallet()
		sign_message()
		if *send=="" {
			// Don't load_balance if he did not want to spend coins as well
			return
		}
	}


	if *rawtx!="" {
		if *p2sh!="" {
			make_p2sh()
			return
		}

		if !*hashes {
			make_wallet()
		}

		if *multisign!="" {
			multisig_sign()
			return
		}

		load_balance(false)
		process_raw_tx()
		return
	}

	if send_request() {
		if !*hashes {
			make_wallet()
		}
		load_balance(!*hashes)
		if spendBtc + feeBtc > totBtc {
			fmt.Println("ERROR: You are trying to spend more than you own")
			return
		}
		make_signed_tx()
		return
	}

	// If no command specified, just print the balance
	make_wallet()
	load_balance(true)
}
