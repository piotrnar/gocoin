package main

import (
	"os"
	"bufio"
	"strings"
	"github.com/piotrnar/gocoin/btc"
)

type oneWallet struct {
	addrs []*btc.BtcAddr
}

func NewWallet(fn string) (wal *oneWallet) {
	f, e := os.Open(fn)
	if e != nil {
		println(e.Error())
		return
	}
	defer f.Close()
	wal = new(oneWallet)
	rd := bufio.NewReader(f)
	for {
		var l string
		l, e = rd.ReadString('\n')
		l = strings.Trim(l, " \t\r\n")
		if len(l)>0 && l[0]!='#' {
			a, e := btc.NewAddrFromString(l)
			if e != nil {
				println(l, ": ", e.Error())
			} else {
				wal.addrs = append(wal.addrs, a)
			}
		}
		if e != nil {
			break
		}
	}
	println(len(wal.addrs), "addresses loaded from", fn)
	return
}
