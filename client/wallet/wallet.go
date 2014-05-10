package wallet

import (
	"os"
	"fmt"
	"bufio"
	"bytes"
	"strings"
	"path/filepath"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/client/common"
)

const (
	UnusedFileName = "UNUSED"
	DefaultFileName = "DEFAULT"
	AddrBookFileName = "ADDRESS"
)

var PrecachingComplete bool

type OneWallet struct {
	FileName string
	Addrs []*btc.BtcAddr
}


func LoadWalfile(fn string, included int) (addrs []*btc.BtcAddr) {
	waldir, walname := filepath.Split(fn)
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
		if len(l)>0 {
			if l[0]=='@' {
				if included>3 {
					println(fmt.Sprint(fn, ":", linenr), "Too many nested wallets")
				} else {
					ifn := strings.Trim(l[1:], " \n\t\t")
					addrs = append(addrs, LoadWalfile(waldir+ifn, included+1)...)
				}
			} else {
				var s string
				if l[0]!='#' {
					s = l
				} else if !PrecachingComplete && len(l)>10 && l[1]=='1' {
					s = l[1:] // While pre-caching addresses, include ones that are commented out
				}
				if s!="" {
					ls := strings.SplitN(s, " ", 2)
					if len(ls)>0 {
						a, e := btc.NewAddrFromString(ls[0])
						if e != nil {
							println(fmt.Sprint(fn, ":", linenr), e.Error())
						} else {
							a.Extra.Wallet = walname
							if len(ls)>1 {
								a.Extra.Label = ls[1]
							}
							a.Extra.Virgin = space_first
							addrs = append(addrs, a)
						}
					}
				}
			}
		}
		if e != nil {
			break
		}
	}

	// remove duplicated addresses
	for i:=0; i<len(addrs)-1; i++ {
		for j:=i+1; j<len(addrs); {
			if addrs[i].String()==addrs[j].String() {
				if addrs[i].Extra.Wallet==AddrBookFileName {
					// Overwrite wallet name if is was ADDRESS (book)
					addrs[i].Extra.Wallet = addrs[j].Extra.Wallet
				}
				addrs[i].Extra.Label += "*"+addrs[j].Extra.Label
				addrs = append(addrs[:j], addrs[j+1:]...)
			} else {
				j++

				// look for same stealth address with different prefix
				if addrs[i].StealthAddr!=nil && addrs[j].StealthAddr!=nil &&
					!bytes.Equal(addrs[i].StealthAddr.Prefix, addrs[j].StealthAddr.Prefix) {
					a1 := addrs[i].StealthAddr
					a2 := addrs[j].StealthAddr
					a1.Prefix = nil
					a2.Prefix = nil
					if a1.String()==a2.String() {
						fmt.Println("WARNING: You have two identical stealth addresses with different prefixes")
						fmt.Println(" 1st :", a1.String())
						fmt.Println(" 2nd :", a1.String())
						fmt.Println("The balance of any of these adresses will not be showed properly.")
					}
				}

			}
		}
	}
	return
}


// Load public wallet from a text file
func NewWallet(fn string) (wal *OneWallet) {
	wal = new(OneWallet)
	wal.FileName = fn
	wal.Addrs = LoadWalfile(fn, 0)
	return
}


func MoveToUnused(addr, walfil string) bool {
	frwal := common.GocoinHomeDir + "wallet" + string(os.PathSeparator) + walfil
	towal := common.GocoinHomeDir + "wallet" + string(os.PathSeparator) + UnusedFileName
	f, er := os.Open(frwal)
	if er != nil {
		println(er.Error())
		return false
	}
	var foundline string
	var srcwallet []string
	rd := bufio.NewReader(f)
	if rd != nil {
		for {
			ln, _, er := rd.ReadLine()
			if er !=nil {
				break
			}
			if foundline=="" && strings.HasPrefix(string(ln), addr) {
				foundline = string(ln)
			} else {
				srcwallet = append(srcwallet, string(ln))
			}
		}
	}
	f.Close()

	if foundline=="" {
		println(addr, "not found in", frwal)
		return false
	}

	f, er = os.OpenFile(towal, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0666)
	if er != nil {
		println(er.Error())
		return false
	}
	fmt.Fprintln(f, foundline)
	f.Close()

	os.Rename(frwal, frwal+".bak")

	f, er = os.Create(frwal)
	if er != nil {
		println(er.Error())
		return false
	}
	for i := range srcwallet {
		fmt.Fprintln(f, srcwallet[i])
	}
	f.Close()

	os.Remove(frwal+".bak")
	return true
}
