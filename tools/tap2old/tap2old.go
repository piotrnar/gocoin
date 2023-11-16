package main

// Convert TAP-public-key address to the old P2KH address
// Or
// COnvert public key to all compatibe BTC deposit addresses

import (
	"encoding/hex"
	"fmt"
	"os"

	"github.com/piotrnar/gocoin/lib/btc"
)

func dump_all_addrs(pk []byte, testnet bool) {
	ad := btc.NewAddrFromPubkey(pk, btc.AddrVerPubkey(testnet))
	if ad == nil {
		println("Unexpected error returned by NewAddrFromPubkey()")
		return
	}
	hrp := btc.GetSegwitHRP(testnet)
	fmt.Println("", ad.String())

	ad.Enc58str = ""
	ad.SegwitProg = &btc.SegwitProg{HRP: hrp, Version: 1, Program: pk[1:]}
	fmt.Println("", ad.String())

	ad.Enc58str = ""
	ad.SegwitProg = &btc.SegwitProg{HRP: hrp, Version: 0, Program: ad.Hash160[:]}
	fmt.Println("", ad.String())

	h160 := btc.Rimp160AfterSha256(append([]byte{0, 20}, ad.Hash160[:]...))
	ad = btc.NewAddrFromHash160(h160[:], btc.AddrVerScript(testnet))
	fmt.Println("", ad.String())
}

func main() {
	if len(os.Args) < 2 {
		println("Specify bech32 encoded Taproot deposit address or hex encoded public key")
		return
	}

	// Try to decode public key first
	if len(os.Args[1]) == 66 && os.Args[1][0] == '0' && (os.Args[1][1] == '2' || os.Args[1][1] == '3') {
		pk, er := hex.DecodeString(os.Args[1])
		if er != nil {
			println(er.Error())
			return
		}
		fmt.Println("Mainnet:")
		dump_all_addrs(pk, false)

		fmt.Println("Testnet:")
		dump_all_addrs(pk, true)
		return
	}

	// if not public key, do the taproot bech32 encoded...
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
	fmt.Println("", btc.NewAddrFromPubkey(append([]byte{0x02}, ad.SegwitProg.Program...), btc.AddrVerPubkey(false)))
	fmt.Println("", btc.NewAddrFromPubkey(append([]byte{0x03}, ad.SegwitProg.Program...), btc.AddrVerPubkey(false)))
}
