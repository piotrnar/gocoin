package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"

	"github.com/piotrnar/gocoin/lib/btc"
)

type unspRec struct {
	btc.TxPrevOut
	label string
	key   *btc.PrivateAddr
	spent bool
}

var (
	// set in load_balance():
	unspentOuts []*unspRec
)

func (u *unspRec) String() string {
	return fmt.Sprint(u.TxPrevOut.String(), " ", u.label)
}

func NewUnspRec(l []byte) (uns *unspRec) {
	if len(l) < 65 || l[64] != '-' {
		return nil
	}

	txid := btc.NewUint256FromString(string(l[:64]))
	if txid == nil {
		return nil
	}

	rst := strings.SplitN(string(l[65:]), " ", 2)
	vout, e := strconv.ParseUint(rst[0], 10, 32)
	if e != nil {
		return nil
	}

	uns = new(unspRec)
	uns.TxPrevOut.Hash = txid.Hash
	uns.TxPrevOut.Vout = uint32(vout)
	if len(rst) > 1 {
		uns.label = rst[1]
	}

	return
}

// load_balance loads the content of the "balance/" folder.
func load_balance() error {
	f, e := os.Open("balance/unspent.txt")
	if e != nil {
		return e
	}
	rd := bufio.NewReader(f)
	for {
		l, _, e := rd.ReadLine()
		if len(l) == 0 && e != nil {
			break
		}
		if uns := NewUnspRec(l); uns != nil {
			if uns.key == nil {
				uns.key = pkscr_to_key(getUO(&uns.TxPrevOut).Pk_script)
			}
			unspentOuts = append(unspentOuts, uns)
		} else {
			println("ERROR in unspent.txt: ", string(l))
		}
	}
	f.Close()
	return nil
}

func show_balance() {
	var totBtc, msBtc, knownInputs, unknownInputs, multisigInputs uint64
	for i := range unspentOuts {
		uo := getUO(&unspentOuts[i].TxPrevOut)

		if unspentOuts[i].key != nil {
			totBtc += uo.Value
			knownInputs++
			continue
		}

		if btc.IsP2SH(uo.Pk_script) {
			msBtc += uo.Value
			multisigInputs++
			continue
		}

		unknownInputs++
		if *verbose {
			fmt.Println("WARNING: Don't know how to sign", unspentOuts[i].TxPrevOut.String())
		}
	}
	fmt.Printf("You have %.8f BTC in %d keyhash outputs\n", float64(totBtc)/1e8, knownInputs)
	if multisigInputs > 0 {
		fmt.Printf("There is %.8f BTC in %d multisig outputs\n", float64(msBtc)/1e8, multisigInputs)
	}
	if unknownInputs > 0 {
		fmt.Println("WARNING:", unknownInputs, "unspendable inputs (-v to print them).")
	}
}

// apply_to_balance applies the changes to the balance folder.
func apply_to_balance(tx *btc.Tx) {
	f, _ := os.Create("balance/unspent.txt")
	if f != nil {
		// append new outputs at the end of unspentOuts
		ioutil.WriteFile("balance/"+tx.Hash.String()+".tx", tx.Serialize(), 0600)

		fmt.Println("Adding", len(tx.TxOut), "new output(s) to the balance/ folder...")
		for out := range tx.TxOut {
			if k := pkscr_to_key(tx.TxOut[out].Pk_script); k != nil {
				uns := new(unspRec)
				uns.key = k
				uns.TxPrevOut.Hash = tx.Hash.Hash
				uns.TxPrevOut.Vout = uint32(out)
				uns.label = fmt.Sprint("# ", btc.UintToBtc(tx.TxOut[out].Value), " BTC @ ", k.BtcAddr.String())
				unspentOuts = append(unspentOuts, uns)
			}
		}

		for j := range unspentOuts {
			if !unspentOuts[j].spent {
				fmt.Fprintln(f, unspentOuts[j].String())
			}
		}
		f.Close()
	} else {
		println("ERROR: Cannot create balance/unspent.txt")
	}
}
