package main

import (
	"os"
	"fmt"
	"flag"
	"math/big"
	"github.com/piotrnar/gocoin/btc"
)

const (
	PassSeedFilename = ".secret"
	RawKeysFilename = ".others"
)

var (
	// Command line switches

	// Wallet options
	dump *bool = flag.Bool("l", false, "List public addressses from the wallet")
	singleask *bool = flag.Bool("1", false, "Do not re-ask for the password (when used along with -l)")
	noverify *bool = flag.Bool("q", false, "Do not verify keys while listing them")
	keycnt *uint = flag.Uint("n", 50, "Set the number of keys to be used")
	uncompressed *bool = flag.Bool("u", false, "Use uncompressed public keys")
	testnet *bool = flag.Bool("t", false, "Force work with testnet addresses")
	verbose *bool = flag.Bool("v", false, "Verbose version (print more info)")
	apply2bal *bool = flag.Bool("a", true, "Apply changes to the balance folder")

	waltype *uint = flag.Uint("type", 3, "Choose a type of the deterministic wallet (1, 2 or 3)")
	type2sec *string  = flag.String("t2sec", "", "Enforce using this secret for Type-2 method (hex encoded)")
	dumppriv *string = flag.String("dump", "", "Export a private key of a given address (use * for all)")

	// Spending money options
	fee *float64 = flag.Float64("fee", 0.0001, "Transaction fee")
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

	// set in load_balance():
	unspentOuts []*btc.TxPrevOut
	unspentOutsLabel []string
	loadedTxs map[[32]byte] *btc.Tx = make(map[[32]byte] *btc.Tx)
	totBtc uint64

	verbyte, privver byte  // address version for public and private key

	// set in make_wallet():
	priv_keys [][]byte
	labels []string
	publ_addrs []*btc.BtcAddr
	compressed_key []bool

	// set in parse_spend():
	spendBtc, feeBtc, changeBtc uint64
	sendTo []oneSendTo

	type2_secret *big.Int // used to type-2 wallets
)


func main() {
	fmt.Println("Gocoin Wallet version", btc.SourcesTag)
	fmt.Println("This program comes with ABSOLUTELY NO WARRANTY")
	fmt.Println()

	if flag.Lookup("h") != nil {
		flag.PrintDefaults()
		os.Exit(0)
	}

	parse_config()
	flag.Parse()

	if *dumptxfn!="" {
		load_balance(false)
		dump_raw_tx()
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
		if !*hashes {
			make_wallet()
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
