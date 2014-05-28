package main

import (
	"os"
	"fmt"
	"flag"
	"crypto/rand"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
)

var (
	scankey *string = flag.String("scankey", "", "Generate a new stealth using this public scan-key")
	prefix *uint = flag.Uint("prefix", 0, "Stealth prefix length in bits (maximum 24)")
	is_stealth map[int] bool = make(map[int]bool)
)


// Generate a new stealth address
func new_stealth_address(prv_key []byte) {
	sk, er := hex.DecodeString(*scankey)
	if er != nil {
		println(er.Error())
		os.Exit(1)
	}
	if len(sk)!=33 || sk[0]!=2 && sk[0]!=3 {
		println("scankey must be a compressed public key (33 bytes long)")
		os.Exit(1)
	}

	if *prefix>16 {
		if *prefix>24 {
			fmt.Println("The stealth prefix cannot be bigger than 24", *prefix)
			os.Exit(1)
		}
		fmt.Println("WARNING: You chose a prefix length of", *prefix)
		fmt.Println(" Long prefixes endanger anonymity of stealth address.")
	}

	pub := btc.PublicFromPrivate(prv_key, true)
	if pub == nil {
		println("PublicFromPrivate error 2")
		os.Exit(1)
	}

	sa := new(btc.StealthAddr)
	sa.Version = btc.StealthAddressVersion(testnet)
	sa.Options = 0
	copy(sa.ScanKey[:], sk)
	sa.SpendKeys = make([][33]byte, 1)
	copy(sa.SpendKeys[0][:], pub)
	sa.Sigs = 1
	sa.Prefix = make([]byte, 1+(byte(*prefix)+7)>>3)
	if *prefix > 0 {
		sa.Prefix[0] = byte(*prefix)
		rand.Read(sa.Prefix[1:])
	}
	fmt.Println(sa.String())
}
