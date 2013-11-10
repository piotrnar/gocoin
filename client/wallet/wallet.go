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


func LoadWalfile(fn string, included bool) (addrs []*btc.BtcAddr) {
	waldir := filepath.Dir(fn) + string(os.PathSeparator)
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
		l = strings.Trim(l, " \t\r\n")
		linenr++
		//println(fmt.Sprint(fn, ":", linenr), "...")
		if len(l)>0 {
			if l[0]=='@' {
				if included {
					println(fmt.Sprint(fn, ":", linenr), "You cannot include wallets recursively")
				} else {
					ifn := strings.Trim(l[1:], " \n\t\t")
					addrs = append(addrs, LoadWalfile(waldir+ifn, true)...)
				}
			} else if l[0]!='#' {
				ls := strings.SplitN(l, " ", 2)
				if len(ls)>0 {
					a, e := btc.NewAddrFromString(ls[0])
					if e != nil {
						println(fmt.Sprint(fn, ":", linenr), e.Error())
					} else {
						if len(ls)>1 {
							a.Label = strings.Trim(ls[1], " \n\t\t")
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
	addrs := LoadWalfile(fn, false)
	if len(addrs)>0 {
		wal = new(OneWallet)
		wal.FileName = fn
		wal.Addrs = addrs
	}
	return
}
