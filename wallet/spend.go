package main

import (
	"os"
	"fmt"
	"flag"
	"bytes"
	"bufio"
	"strconv"
	"strings"
//	"math/big"
//	"crypto/rand"
//	"crypto/ecdsa"
//	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
	"code.google.com/p/go.crypto/ripemd160"
)

var (
	keycnt *uint = flag.Uint("c", 100, "Set maximum number of keys")
	testnet *bool = flag.Bool("t", true, "Work with testnet addresses")

	amout *float64 = flag.Float64("amount", 0.0, "Amount to spend")
	fee *float64 = flag.Float64("fee", 0.0005, "Transaction fee")
	toaddr *string  = flag.String("to", "", "Destination address (where to send the money)")
	change *string  = flag.String("change", "", "Send change to this address")

	verbyte byte

	unspentOuts []*btc.TxPrevOut
	amBtc, feeBtc, totBtc uint64
	loadedTxs map[[32]byte] *btc.Tx = make(map[[32]byte] *btc.Tx)
)


func getpass() string {
	f, e := os.Open("wallet.sec")
	if e != nil {
		println("Make sure to create wallet.sec file put your wallet's secret/password into it")
		println(e.Error())
		os.Exit(1)
	}
	le, _ := f.Seek(0, os.SEEK_END)
	buf := make([]byte, le)
	f.Seek(0, os.SEEK_SET)
	n, e := f.Read(buf[:])
	if e != nil {
		println(e.Error())
		os.Exit(1)
	}
	if int64(n)!=le {
		println("Something is wrong with teh password file")
	}
	return string(buf)
}

func getline() string {
	li, _, _ := bufio.NewReader(os.Stdin).ReadLine()
	return string(li)
}

func rimp160(data []byte) (res [20]byte) {
	rim := ripemd160.New()
	rim.Write(data)
	copy(res[:], rim.Sum(nil))
	return
}


func getPubKey(curv *btc.BitCurve, priv_key []byte) (res [65]byte) {
	x, y := curv.ScalarBaseMult(priv_key)
	xd := x.Bytes()
	yd := y.Bytes()

	if len(xd)>32 || len(yd)>32 {
		println("x:", len(xd), "y:", len(yd))
		os.Exit(2)
	}

	res[0] = 4
	copy(res[1+32-len(xd):33], xd)
	copy(res[33+32-len(yd):65], yd)
	return
}

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
			unspentOuts = append(unspentOuts, uns)
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
			totBtc += UO(uns)
		}
	}
	f.Close()
	fmt.Printf("%.8f BTC in %d unspent outputs\n", float64(totBtc)/1e8, len(unspentOuts))
}

func UO(uns *btc.TxPrevOut) uint64 {
	tx, _ := loadedTxs[uns.Hash]
	return tx.TxOut[uns.Vout].Value
}

func main() {
	if flag.Lookup("h") != nil {
		flag.PrintDefaults()
		os.Exit(0)
	}
	flag.Parse()

	if *amout<=0 {
		fmt.Println("Specify -amount parameter (how much you want to spend)")
		return
	}
	
	if *toaddr=="" {
		fmt.Println("Specify -to parameter (where you want to transfer it)")
		return
	}
	
	pass := getpass()
	
	if *testnet {
		verbyte = 0x6f
	} else {
		verbyte = 0
	}

	load_balance()

	amBtc = uint64(*amout*1e8)
	feeBtc = uint64(*fee*1e8)

	if amBtc + feeBtc > totBtc {
		fmt.Println("You want to spend more than you own")
		return
	}

	curv := btc.S256()
	seed_key := btc.Sha2Sum([]byte(pass))
	priv_keys := make([][32]byte, *keycnt)
	publ_keys := make([][65]byte, *keycnt)
	publ_addrs := make([]*btc.BtcAddr, *keycnt)
	fmt.Println("Generating", *keycnt, "keys...")
	var i uint
	for i < *keycnt {
		seed_key = btc.Sha2Sum(seed_key[:])
		priv_keys[i] = seed_key
		publ_keys[i] = getPubKey(curv, seed_key[:])
		h160 := rimp160(publ_keys[i][:])
		publ_addrs[i] = btc.NewAddrFromHash160(h160[:], verbyte)
		i++
	}
	fmt.Println("Private keys re-generated")

	sofar := uint64(0)
	for i=0; i<uint(len(unspentOuts)); i++ {
		sofar += UO(unspentOuts[i])
		if sofar >= amBtc + feeBtc {
			break
		}
	}
	fmt.Printf("Spending %d out of %d outputs...\n", i+1, len(unspentOuts))

	// Make the transaction
	tx := new(btc.Tx)
	tx.Version = 1
	tx.Lock_time = 0xffffffff
}
