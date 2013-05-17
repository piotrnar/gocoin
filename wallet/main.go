package main

import (
	"os"
	"fmt"
	"flag"
	"bytes"
	"bufio"
	"strconv"
	"strings"
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
	noverify *bool = flag.Bool("q", false, "Do not verify keys while listing them")
	keycnt *uint = flag.Uint("n", 25, "Set the number of keys to be used")
	uncompressed *bool = flag.Bool("u", false, "Use uncompressed public keys")
	testnet *bool = flag.Bool("t", false, "Force work with testnet addresses")
	verbose *bool = flag.Bool("v", false, "Verbose bersion (print more info)")

	// Spending money options
	fee *float64 = flag.Float64("fee", 0.0001, "Transaction fee")
	send *string  = flag.String("send", "", "Send money to list of comma separated pairs: address=amount")
	batch *string  = flag.String("batch", "", "Send money as per the given batch file (each line: address=amount)")
	change *string  = flag.String("change", "", "Send any change to this address (otherwise return to 1st input)")

	// Message signing options
	signaddr *string  = flag.String("sign", "", "Request a sign operation with a given bitcoin address")
	message *string  = flag.String("msg", "", "Defines a message to be signed (otherwise take it from stdin)")

	// set in load_balance():
	unspentOuts []*btc.TxPrevOut
	unspentOutsLabel []string
	loadedTxs map[[32]byte] *btc.Tx = make(map[[32]byte] *btc.Tx)
	totBtc uint64

	verbyte, privver byte  // address version for public and private key

	// set in make_wallet():
	priv_keys [][32]byte
	labels []string
	publ_addrs []*btc.BtcAddr

	maxKeyVal *big.Int // used by verify_key

	// set in parse_spend():
	spendBtc, feeBtc, changeBtc uint64
	sendTo []oneSendTo

	curv *btc.BitCurve = btc.S256()
)


// Print all the piblic addresses
func dump_addrs() {
	maxKeyVal, _ = new(big.Int).SetString("FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFEBAAEDCE6AF48A03BBFD25E8CD0364141", 16)
	f, _ := os.Create("wallet.txt")
	for i := range publ_addrs {
		if !*noverify && !verify_key(priv_keys[i][:], publ_addrs[i].Pubkey) {
			println("Something wrong with key at index", i, " - abort!")
			os.Exit(1)
		}
		fmt.Println(publ_addrs[i].String(), labels[i])
		if f != nil {
			fmt.Fprintln(f, publ_addrs[i].String(), labels[i])
		}
	}
	if f != nil {
		f.Close()
		fmt.Println("You can find all the addresses in wallet.txt file")
	}
}

// load the content of the "balance/" folder
func load_balance() {
	f, e := os.Open("balance/unspent.txt")
	if e != nil {
		println(e.Error())
		os.Exit(1)
	}
	rd := bufio.NewReader(f)
	for {
		l, _, e := rd.ReadLine()
		if len(l)==0 && e!=nil {
			break
		}
		if l[64]=='-' {
			txid := btc.NewUint256FromString(string(l[:64]))
			rst := strings.SplitN(string(l[65:]), " ", 2)
			vout, _ := strconv.ParseUint(rst[0], 10, 32)
			uns := new(btc.TxPrevOut)
			copy(uns.Hash[:], txid.Hash[:])
			uns.Vout = uint32(vout)
			lab := ""
			if len(rst)>1 {
				lab = rst[1]
			}

			if _, ok := loadedTxs[txid.Hash]; !ok {
				tf, _ := os.Open("balance/"+txid.String()+".tx")
				if tf != nil {
					siz, _ := tf.Seek(0, os.SEEK_END)
					tf.Seek(0, os.SEEK_SET)
					buf := make([]byte, siz)
					tf.Read(buf)
					tf.Close()
					th := btc.Sha2Sum(buf)
					if bytes.Equal(th[:], txid.Hash[:]) {
						tx, _ := btc.NewTx(buf)
						if tx != nil {
							loadedTxs[txid.Hash] = tx
						} else {
							println("transaction is corrupt:", txid.String())
						}
					} else {
						println("transaction file is corrupt:", txid.String())
						os.Exit(1)
					}
				} else {
					println("transaction file not found:", txid.String())
					os.Exit(1)
				}
			}

			uo := UO(uns)
			fnd := false
			for j := range publ_addrs {
				if publ_addrs[j].Owns(uo.Pk_script) {
					unspentOuts = append(unspentOuts, uns)
					unspentOutsLabel = append(unspentOutsLabel, lab)
					totBtc += UO(uns).Value
					fnd = true
					break
				}
			}

			if !fnd {
				fmt.Println(uns.String(), "does not belogn to your wallet - ignore it")
			}

		}
	}
	f.Close()
	fmt.Printf("You have %.8f BTC in %d unspent outputs\n", float64(totBtc)/1e8, len(unspentOuts))
}


func main() {
	fmt.Println("Gocoin Wallet version", btc.SourcesTag)
	fmt.Println("This program comes with ABSOLUTELY NO WARRANTY")
	fmt.Println()

	if flag.Lookup("h") != nil {
		flag.PrintDefaults()
		os.Exit(0)
	}
	flag.Parse()

	make_wallet()

	if *dump {
		dump_addrs()
		return
	}

	if *signaddr!="" {
		sign_message()
		if *send=="" {
			// Don't load_balnace if he did not want to spend coins as well
			return
		}
	}

	// If no dump, then it should be send money
	load_balance()

	if send_request() {
		if spendBtc + feeBtc > totBtc {
			fmt.Println("You want to spend more than you own")
			return
		}
		make_signed_tx()
	}
}
