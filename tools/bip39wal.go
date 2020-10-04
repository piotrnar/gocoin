package main

import (
	"encoding/hex"
	"fmt"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/bip39"
	"io/ioutil"
	"os"
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
	fmt.Println("BIP32 Root Key:")
	fmt.Println("", wal.String())

	ch := wal.Child(0) // m/0

	fmt.Println()
	fmt.Println("BIP32 Extended Private Key:")
	fmt.Println("", ch.String())

	fmt.Println()
	fmt.Println("BIP32 Extended Public Key:")
	fmt.Println("", ch.Pub().String())

	// m/0/0 to m/0/19
	fmt.Println()
	for i := uint32(0); i < 20; i++ {
		cc := ch.Child(i)
		puba := cc.PubAddr()
		prva := btc.NewPrivateAddr(cc.Key[1:], btc.AddrVerPubkey(false)|0x80 /*private key version*/, true /*comprtessed*/)
		fmt.Print("m/0/", i)
		fmt.Print(" \t")
		fmt.Print(puba.String())
		fmt.Print(" \t")
		fmt.Print(hex.EncodeToString(puba.Pubkey))
		fmt.Print(" \t")
		fmt.Print(prva.String())
		fmt.Println()
	}

}
