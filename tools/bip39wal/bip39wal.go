package main

import (
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strings"

	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/bip39"
)

/*

This tool provides same functgionality as the Mnemonic Code Converter - https://iancoleman.io/bip39/

It only generates 24 words mnemonics, but it can read also 12, 15, 18 and 21 words.

It only follows BIP32 Derivation Path.

*/

func main() {
	var mnemonic string
	var er error
	var seed []byte

	if len(os.Args) > 1 {
		fmt.Println("Reading mnemonic from file", os.Args[1], "...")
		seed, er = ioutil.ReadFile(os.Args[1])
		if er != nil {
			println(er.Error())
			return
		}
		mnemonic = string(seed)
		re := regexp.MustCompile("[^a-zA-Z]")
		a := re.ReplaceAll([]byte(mnemonic), []byte(" "))
		lns := strings.Split(string(a), " ")
		mnemonic = ""
		for _, l := range lns {
			if l != "" {
				if mnemonic != "" {
					mnemonic = mnemonic + " " + l
				} else {
					mnemonic = l
				}
			}
		}
	} else {
		entropy, _ := bip39.NewEntropy(256)
		mnemonic, er = bip39.NewMnemonic(entropy)
		if er != nil {
			println(er.Error())
			return
		}
		fmt.Print(mnemonic)
		return
	}
	seed, er = bip39.NewSeedWithErrorChecking(mnemonic, "")
	if er != nil {
		println("NewSeedWithErrorChecking: ", er.Error())
		return
	}

	fmt.Println()
	fmt.Println("BIP39 Seed:")
	wal := btc.MasterKey(seed, false)
	fmt.Println(hex.EncodeToString(seed))

	fmt.Println()
	fmt.Println("Extended Master Private Key:")
	fmt.Println("", wal.String())

	wal.Prefix = btc.PrivateZ
	fmt.Println()
	fmt.Println("Extended Master zprv:")
	fmt.Println("", wal.String())

	wasad := wal.Child(84 | 0x80000000).Child(0x80000000).Child(0x80000000) // m/84'/0'/0'

	wasad.Prefix = btc.Private
	fmt.Println()
	fmt.Println("Extended Account Private Key:")
	fmt.Println("", wasad.String())

	wasad.Prefix = btc.PrivateZ
	fmt.Println()
	fmt.Println("Extended Account zprv:")
	fmt.Println("", wasad.String())

	wasad.Prefix = btc.Private
	fmt.Println()
	fmt.Println("Extended Account Public Key:")
	fmt.Println("", wasad.Pub().String())

	wasad.Prefix = btc.PrivateZ
	fmt.Println()
	fmt.Println("Extended Account zpub:")
	fmt.Println("", wasad.Pub().String())

	ch := wasad.Child(0) // 84'/0'/0'/0

	// 84'/0'/0'/0/0 to 84'/0'/0'/0/5
	fmt.Println()
	for i := uint32(0); i <= 5; i++ {
		cc := ch.Child(i)
		puba := cc.PubAddr()
		prva := btc.NewPrivateAddr(cc.Key[1:], btc.AddrVerPubkey(false)|0x80 /*private key version*/, true /*comprtessed*/)
		fmt.Print("84'/0'/0'/0/", i)
		fmt.Print(" \t")
		fmt.Print(puba.String())
		fmt.Print(" \t")
		fmt.Print(hex.EncodeToString(puba.Pubkey))
		fmt.Print(" \t")
		fmt.Print(prva.String())
		fmt.Println()
	}

}
