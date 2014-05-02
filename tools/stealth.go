package main

import (
	"os"
	"fmt"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
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
	fmt.Println("vers", a.Version)
	fmt.Println("opts", a.Options)
	fmt.Println("scan", hex.EncodeToString(a.ScanKey[:]))
	for i := range a.SpendKeys {
		fmt.Println("spnd", hex.EncodeToString(a.SpendKeys[i][:]))
	}
	fmt.Println("sign", a.Sigs)
	fmt.Println("pref", hex.EncodeToString(a.Prefix))
}
