package wallet

import (
	"os"
	"fmt"
	"bufio"
	"strings"
	"path/filepath"
	"github.com/piotrnar/gocoin/btc"
)

type OneWallet struct {
	FileName string
	Addrs []*btc.BtcAddr
}


func LoadWalfile(fn string, included int) (addrs []*btc.BtcAddr) {
	waldir, walname := filepath.Split(fn)
	if walname[0]=='.' {
		walname = walname[1:] // remove trailing dot (from hidden wallets)
	}
	f, e := os.Open(fn)
	if e != nil {
		println(e.Error())
		return
	}
	defer f.Close()
	rd := bufio.NewReader(f)
	linenr := 0
	for {
		var l string
		l, e = rd.ReadString('\n')
		space_first := len(l)>0 && l[0]==' '
		l = strings.Trim(l, " \t\r\n")
		linenr++
		//println(fmt.Sprint(fn, ":", linenr), "...")
		if len(l)>0 {
			if l[0]=='@' {
				if included>3 {
					println(fmt.Sprint(fn, ":", linenr), "Too many nested wallets")
				} else {
					ifn := strings.Trim(l[1:], " \n\t\t")
					addrs = append(addrs, LoadWalfile(waldir+ifn, included+1)...)
				}
			} else if l[0]!='#' {
				ls := strings.SplitN(l, " ", 2)
				if len(ls)>0 {
					a, e := btc.NewAddrFromString(ls[0])
					if e != nil {
						println(fmt.Sprint(fn, ":", linenr), e.Error())
					} else {
						if len(ls)>1 {
							a.Label = ls[1]
						}
						a.Label += "@"+walname
						if space_first {
							a.Label += "<!--VIRGIN-->"
						}
						addrs = append(addrs, a)
					}
				}
			}
		}
		if e != nil {
			break
		}
	}
	return
}


// Load public wallet from a text file
func NewWallet(fn string) (wal *OneWallet) {
	addrs := LoadWalfile(fn, 0)
	if len(addrs)>0 {
		wal = new(OneWallet)
		wal.FileName = fn
		wal.Addrs = addrs
	}
	return
}
