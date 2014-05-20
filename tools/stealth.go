package main

import (
	"os"
	"fmt"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
)

func main() {
	if len(os.Args)!=2 {
		fmt.Println("Specify Stealth Address to decode")
		return
	}
	a, e := btc.NewStealthAddrFromString(os.Args[1])
	if e != nil {
		println(e.Error())
		return
	}
	fmt.Println("Version:", fmt.Sprintf("0x%02x", a.Version), "=", a.Version)
	fmt.Println("Options:", fmt.Sprintf("0x%02x", a.Options), "=", a.Options)
	fmt.Println("scanKey:", hex.EncodeToString(a.ScanKey[:]))
	for i := range a.SpendKeys {
		fmt.Println("spndKey:", hex.EncodeToString(a.SpendKeys[i][:]))
	}
	fmt.Println("sigNeed:", a.Sigs)
	if len(a.Prefix)>0 {
		fmt.Println("Prefix :", fmt.Sprint(hex.EncodeToString(a.Prefix[1:]), "/", a.Prefix[0]))
	}
}
