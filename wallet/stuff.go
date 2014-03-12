package main

import (
	"os"
	"fmt"
	"bufio"
	"bytes"
	"strings"
	"io/ioutil"
	"encoding/hex"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
)

var secrespass func() string


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


// Reads a password from stdin
func readpass() string {
	if secrespass != nil {
		return secrespass()
	}
	return getline()
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
func getpass() string {
	f, e := os.Open(PassSeedFilename)
	if e != nil {
		fmt.Println("Seed file", PassSeedFilename, "not found")
		fmt.Print("Enter your wallet's seed password: ")
		pass := readpass()
		if pass!="" && *dump {
			if !*singleask {
				fmt.Print("Re-enter the seed password (to be sure): ")
				if pass!=readpass() {
					println("The two passwords you entered do not match")
					os.Exit(1)
				}
			}
			// Maybe he wants to save the password?
			if ask_yes_no("Save the password on disk, so you won't be asked for it later?") {
				f, e = os.Create(PassSeedFilename)
				if e == nil {
					f.Write([]byte(pass))
					f.Close()
				} else {
					println("Could not save the password:", e.Error())
				}
			}
		}
		return pass
	}
	le, _ := f.Seek(0, os.SEEK_END)
	buf := make([]byte, le)
	f.Seek(0, os.SEEK_SET)
	n, e := f.Read(buf[:])
	if e != nil {
		println("Reading secret file:", e.Error())
		os.Exit(1)
	}
	if int64(n)!=le {
		println("Something is wrong with the password file. Aborting.")
		os.Exit(1)
	}
	for i := range buf {
		if buf[i]<' ' || buf[i]>126 {
			fmt.Println("WARNING: Your secret contains non-printable characters")
			break
		}
	}
	return string(buf)
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
			chng = publ_addrs[j]
			return
		}
	}

	fmt.Println("You do not own the address of the first input")
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
				if len(publ_addrs[i].Pubkey)==33 {
					fmt.Println(sec2b58com(priv_keys[i]), publ_addrs[i].String(), labels[i])
				} else {
					fmt.Println(sec2b58unc(priv_keys[i]), publ_addrs[i].String(), labels[i])
				}
				return
			}
		}
		println("Dump Private Key:", a.String(), "not found it the wallet")
	}
}


func raw_tx_from_file(fn string) *btc.Tx {
	d, er := ioutil.ReadFile(fn)
	if er != nil {
		fmt.Println(er.Error())
		return nil
	}

	dat, er := hex.DecodeString(string(d))
	if er != nil {
		fmt.Println("hex.DecodeString failed - assume binary transaction file")
		dat = d
	}
	tx, txle := btc.NewTx(dat)

	if tx != nil && txle != len(dat) {
		fmt.Println("WARNING: Raw transaction length mismatch", txle, len(dat))
	}

	return tx
}


// add P2SH pre-signing data into a raw tx
func make_p2sh() {
	tx := raw_tx_from_file(*rawtx)
	if tx == nil {
		fmt.Println("ERROR: Cannot decode the raw transaction")
		return
	}

	d, er := hex.DecodeString(*p2sh)
	if er != nil {
		println("P2SH hex data:", er.Error())
	}
	var pkscr [23]byte
	pkscr[0] = 0xa9
	pkscr[1] = 20
	btc.RimpHash(d, pkscr[2:22])
	pkscr[22] = 0x87

	fmt.Println("The P2SH data points to address", btc.NewAddrFromHash160(pkscr[2:22], btc.AddrVerScript(*testnet)))

	for i := range tx.TxIn {
		if len(tx.TxIn[i].ScriptSig) > 0 {
			fmt.Println("WARNING: Input at index", i, "already has a sigScript - owerwrite!")
		}
		buf := new(bytes.Buffer)
		buf.WriteByte(0x00) // push OP_FALSE (must be there for P2SH transaction)
		if len(d) > 0x4b {
			if len(d) > 0xff {
				buf.WriteByte(btc.OP_PUSHDATA2)
				binary.Write(buf, binary.LittleEndian, uint16(len(d)))
			} else {
				buf.WriteByte(btc.OP_PUSHDATA1)
				buf.WriteByte(byte(len(d)))
			}
		} else {
			buf.WriteByte(byte(len(d)))
		}
		buf.Write(d)
		tx.TxIn[i].ScriptSig = buf.Bytes()
		fmt.Println("Input number", i, " - hash to sign:", hex.EncodeToString(tx.SignatureHash(d, i, btc.SIGHASH_ALL)))
	}
	ioutil.WriteFile(MultiToSignOut, []byte(hex.EncodeToString(tx.Serialize())), 0666)
	fmt.Println("Transaction with", len(tx.TxIn), "inputs ready for multi-signing, stored in", MultiToSignOut)
}
