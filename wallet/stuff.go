package main

import (
	"os"
	"fmt"
	"bufio"
	"bytes"
	"strings"
	"io/ioutil"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/others/utils"
)

// Get TxOut record, by the given TxPrevOut
func UO(uns *btc.TxPrevOut) *btc.TxOut {
	tx, _ := loadedTxs[uns.Hash]
	return tx.TxOut[uns.Vout]
}


// Read a line from stdin
func getline() string {
	li, _, _ := bufio.NewReader(os.Stdin).ReadLine()
	return string(li)
}


func ask_yes_no(msg string) bool {
	for {
		fmt.Print(msg, " (y/n) : ")
		l := strings.ToLower(getline())
		if l=="y" {
			return true
		} else if l=="n" {
			return false
		}
	}
	return false
}


// Input the password (that is the secret seed to your wallet)
func getseed(seed []byte) bool {
	var pass [1024]byte
	var n int
	var e error
	var f *os.File

	if !*ask4pass {
		f, e = os.Open(PassSeedFilename)
		if e == nil {
			n, e = f.Read(pass[:])
			f.Close()
			if n <= 0 {
				return false
			}
			goto calc_seed
		}

		fmt.Println("Seed file", PassSeedFilename, "not found")
	}

	fmt.Print("Enter your wallet's seed password: ")
	n = utils.ReadPassword(pass[:])
	if n<=0 {
		return false
	}

	if *dump {
		if !*singleask {
			fmt.Print("Re-enter the seed password (to be sure): ")
			var pass2 [1024]byte
			p2len := utils.ReadPassword(pass2[:])
			if p2len!=n || !bytes.Equal(pass[:n], pass2[:p2len]) {
				utils.ClearBuffer(pass2[:p2len])
				println("The two passwords you entered do not match")
				return false
			}
			utils.ClearBuffer(pass2[:p2len])
		}
		// Maybe he wants to save the password?
		if ask_yes_no("Save the password on disk, so you won't be asked for it later?") {
			e = ioutil.WriteFile(PassSeedFilename, pass[:n], 0600)
			if e != nil {
				fmt.Println("WARNING: Could not save the password", e.Error())
			}
		}
	}
calc_seed:
	for i:=0; i<n; i++ {
		if pass[i]<' ' || pass[i]>126 {
			fmt.Println("WARNING: Your secret contains non-printable characters")
			break
		}
	}
	btc.ShaHash(pass[:n], seed)
	utils.ClearBuffer(pass[:n])
	return true
}


// return the change addrress or nil if there will be no change
func get_change_addr() (chng *btc.BtcAddr) {
	if *change!="" {
		var e error
		chng, e = btc.NewAddrFromString(*change)
		if e != nil {
			println("Change address:", e.Error())
			os.Exit(1)
		}
		return
	}

	// If change address not specified, send it back to the first input
	uo := UO(unspentOuts[0])
	for j := range publ_addrs {
		if publ_addrs[j].Owns(uo.Pk_script) {
			if is_stealth[j] {
				println("Cannot send change to a stealth address. Use -change param")
				os.Exit(1)
			}
			chng = publ_addrs[j]
			return
		}
	}

	fmt.Println("You do not own the address of the first input, nor specified -change")
	os.Exit(1)
	return
}


// Uncompressed private key
func sec2b58unc(pk []byte) string {
	var dat [37]byte
	dat[0] = privver
	copy(dat[1:33], pk)
	sh := btc.Sha2Sum(dat[0:33])
	copy(dat[33:37], sh[:4])
	return btc.Encodeb58(dat[:])
}


// Compressed private key
func sec2b58com(pk []byte) string {
	var dat [38]byte
	dat[0] = privver
	copy(dat[1:33], pk)
	dat[33] = 1 // compressed
	sh := btc.Sha2Sum(dat[0:34])
	copy(dat[34:38], sh[:4])
	return btc.Encodeb58(dat[:])
}


func dump_prvkey() {
	if *dumppriv=="*" {
		// Dump all private keys
		for i := range priv_keys {
			if len(publ_addrs[i].Pubkey)==33 {
				fmt.Println(sec2b58com(priv_keys[i]), publ_addrs[i].String(), labels[i])
			} else {
				fmt.Println(sec2b58unc(priv_keys[i]), publ_addrs[i].String(), labels[i])
			}
		}
	} else {
		// single key
		a, e := btc.NewAddrFromString(*dumppriv)
		if e!=nil {
			println("Dump Private Key:", e.Error())
			return
		}
		if a.Version != verbyte {
			println("Dump Private Key: Version byte mismatch", a.Version, verbyte)
			return
		}
		for i := range priv_keys {
			if publ_addrs[i].Hash160==a.Hash160 {
				fmt.Println("Public address:", publ_addrs[i].String(), labels[i])
				fmt.Println("Public hexdump:", hex.EncodeToString(publ_addrs[i].Pubkey))
				fmt.Println("Public compressed:", len(publ_addrs[i].Pubkey)==33)
				if len(publ_addrs[i].Pubkey)==33 {
					fmt.Println("Private encoded:", sec2b58com(priv_keys[i]))
				} else {
					fmt.Println("Private encoded:", sec2b58unc(priv_keys[i]))
				}
				fmt.Println("Private hexdump:", hex.EncodeToString(priv_keys[i]))
				return
			}
		}
		println("Dump Private Key:", a.String(), "not found it the wallet")
	}
}


func raw_tx_from_file(fn string) *btc.Tx {
	dat := utils.GetRawData(fn)
	if dat==nil {
		fmt.Println("Cannot fetch raw transaction data")
		return nil
	}
	tx, txle := btc.NewTx(dat)
	if tx != nil {
		tx.Hash = btc.NewSha2Hash(dat)
		if txle != len(dat) {
			fmt.Println("WARNING: Raw transaction length mismatch", txle, len(dat))
		}
	}
	return tx
}
