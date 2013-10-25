package wallet

import (
	"os"
	"bufio"
	"strings"
	"github.com/piotrnar/gocoin/btc"
)

type OneWallet struct {
	FileName string
	Addrs []*btc.BtcAddr
}

// Load public wallet from a text file
func NewWallet(fn string) (wal *OneWallet) {
	f, e := os.Open(fn)
	if e != nil {
		println(e.Error())
		return
	}
	defer f.Close()
	wal = new(OneWallet)
	wal.FileName = fn
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
					wal.Addrs = append(wal.Addrs, a)
				}
			}
		}
		if e != nil {
			break
		}
	}
	if len(wal.Addrs)==0 {
		wal = nil
	}
	return
}
