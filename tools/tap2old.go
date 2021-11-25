package main

// Convert TAP-public-key address to the old P2KH address

import (
	"encoding/hex"
	"fmt"
	"github.com/piotrnar/gocoin/lib/secp256k1"
	"os"

	"github.com/piotrnar/gocoin/lib/btc"
)

func main() {
	if len(os.Args) < 2 {
		println("Specify bech32 encoded Taproot deposit address")
		return
	}
	ad, er := btc.NewAddrFromString(os.Args[1])
	if er != nil {
		println(er.Error())
		return
	}
	if ad.SegwitProg == nil {
		println("This is not a segwit type address")
		return
	}
	fmt.Println("Version:", ad.SegwitProg.Version)
	fmt.Println("Program:", hex.EncodeToString(ad.SegwitProg.Program))
	if ad.SegwitProg.Version != 1 {
		println("This is not segwit version 1 address")
		return
	}
	if len(ad.SegwitProg.Program) != 32 {
		println("Program length must be 32 bytes")
		return
	}
	fmt.Println("Possible P2KH addresses:")
	var pk secp256k1.XY
	var pkb [33]byte
	pk.X.SetB32(ad.SegwitProg.Program)
	pk.SetXO(&pk.X, false)
	if pk.IsValid() {
		pk.GetPublicKey(pkb[:])
		fmt.Println("e:", btc.NewAddrFromPubkey(pkb[:], btc.AddrVerPubkey(false)))
	} else {
		fmt.Println("e: Not valid")
	}
	pk.X.SetB32(ad.SegwitProg.Program)
	pk.SetXO(&pk.X, true)
	if pk.IsValid() {
		pk.GetPublicKey(pkb[:])
		fmt.Println("o:", btc.NewAddrFromPubkey(pkb[:], btc.AddrVerPubkey(false)))
	} else {
		fmt.Println("o: Not valid")
	}
	//fmt.Println("", btc.NewAddrFromPubkey(append([]byte{0x02}, ad.SegwitProg.Program...), btc.AddrVerPubkey(false)))
	//fmt.Println("", btc.NewAddrFromPubkey(append([]byte{0x03}, ad.SegwitProg.Program...), btc.AddrVerPubkey(false)))
}
