package main

import (
	"os"
	"fmt"
	"bufio"
	"strings"
	"github.com/piotrnar/gocoin/btc"
)

type oneWallet struct {
	filename string
	addrs []*btc.BtcAddr
}

// Load public wallet from a text file
func NewWallet(fn string) (wal *oneWallet) {
	f, e := os.Open(fn)
	if e != nil {
		println(e.Error())
		return
	}
	defer f.Close()
	wal = new(oneWallet)
	wal.filename = fn
	rd := bufio.NewReader(f)
	for {
		var l string
		l, e = rd.ReadString('\n')
		l = strings.Trim(l, " \t\r\n")
		if len(l)>0 && l[0]!='#' {
			ls := strings.SplitN(l, " ", 2)
			if len(ls)>0 {
				a, e := btc.NewAddrFromString(ls[0])
				if e != nil {
					println(l, ": ", e.Error())
				} else {
					if len(ls)>1 {
						a.Label = strings.Trim(ls[1], " \n\t\t")
					}
					wal.addrs = append(wal.addrs, a)
				}
			}
		}
		if e != nil {
			break
		}
	}
	if len(wal.addrs)==0 {
		wal = nil
	} else {
		fmt.Println(len(wal.addrs), "addresses loaded from", fn)
	}
	return
}


func LoadWallet(fn string) {
	MyWallet = NewWallet(fn)
	BalanceInvalid = true
}


func load_wallet(fn string) {
	if fn=="." {
		fmt.Println("Default wallet from", GocoinHomeDir+"wallet.txt")
		LoadWallet(GocoinHomeDir+"wallet.txt")
	} else if fn != "" {
		fmt.Println("Switching to wallet from", fn)
		LoadWallet(fn)
	}

	if MyWallet==nil {
		fmt.Println("No wallet loaded")
		return
	}

	if fn == "-" {
		fmt.Println("Reloading wallet from", MyWallet.filename)
		LoadWallet(MyWallet.filename)
		fmt.Println("Dumping current wallet from", MyWallet.filename)
	}

	for i := range MyWallet.addrs {
		fmt.Println(" ", MyWallet.addrs[i].StringLab())
	}
}


func init() {
	newUi("wallet wal", true, load_wallet, "Load wallet from given file (or re-load the last one) and display its addrs")
}
