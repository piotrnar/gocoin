package main

import (
	"os"
	"fmt"
	"bufio"
	"strconv"
	"strings"
	"github.com/piotrnar/gocoin/btc"
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
func getpass() string {
	f, e := os.Open(PassSeedFilename)
	if e != nil {
		fmt.Println("Seed file", PassSeedFilename, "not found")
		fmt.Print("Enter your wallet's seed password: ")
		pass := getline()
		if pass!="" && *dump {
			fmt.Print("Re-enter the seed password (to be sure): ")
			if pass!=getline() {
				println("The two passwords you entered do not match")
				os.Exit(1)
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


// Parses floating number abount to return an int value expressed in Satoshi's
// Using strconv.ParseFloat followed by uint64(val*1e8) is not precise enough.
func ParseAmount(s string) uint64 {
	ss := strings.Split(s, ".")
	if len(ss)==1 {
		val, er := strconv.ParseUint(ss[0], 10, 64)
		if er != nil {
			println("Incorrect amount", s, er.Error())
			os.Exit(1)
		}
		return 1e8*val
	}
	if len(ss)!=2 {
		println("Incorrect amount", s)
		os.Exit(1)
	}

	if len(ss[1])>8 {
		println("Incorrect amount", s)
		os.Exit(1)
	}
	if len(ss[1])<8 {
		ss[1] += strings.Repeat("0", 8-len(ss[1]))
	}

	small, er := strconv.ParseUint(ss[1], 10, 64)
	if er != nil {
		println("Incorrect amount", s, er.Error())
		os.Exit(1)
	}

	big, er := strconv.ParseUint(ss[0], 10, 64)
	if er != nil {
		println("Incorrect amount", s, er.Error())
		os.Exit(1)
	}

	return 1e8*big + small
}
