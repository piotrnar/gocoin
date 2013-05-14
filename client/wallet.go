package main

import (
	"os"
	"bufio"
	"strings"
	"github.com/piotrnar/gocoin/btc"
)

type oneWallet struct {
	filename string
	addrs []*btc.BtcAddr
	label []string
}

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
					wal.addrs = append(wal.addrs, a)
					if len(ls)>1 {
						wal.label = append(wal.label, strings.Trim(ls[1], " \n\t\t"))
					} else {
						wal.label = append(wal.label, "")
					}
				}
			}
		}
		if e != nil {
			break
		}
	}
	println(len(wal.addrs), "addresses loaded from", fn)
	return
}
