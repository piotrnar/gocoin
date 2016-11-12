package main

import(
	"os"
	"fmt"
	"github.com/piotrnar/gocoin/lib/btc"
)

func main() {
	var testnet bool
	if len(os.Args)!=2 {
		fmt.Println("Specify one P2KH bitcoin address to see it's P2SH-P2WPKH deposit address")
		fmt.Println("WARNING: Make sure the input address comes from an uncompressed key!!!!!")
		return
	}
	aa, er := btc.NewAddrFromString(os.Args[1])
	if er!=nil {
		println(er.Error())
		return
	}

	if btc.AddrVerPubkey(false)==aa.Version {
	} else if btc.AddrVerPubkey(true)==aa.Version {
		testnet = true
	} else {
		fmt.Println("This does nto seem to be P2KH type address")
		return
	}

	h160 := btc.Rimp160AfterSha256(append([]byte{0,20}, aa.Hash160[:]...))
	aa = btc.NewAddrFromHash160(h160[:], btc.AddrVerScript(testnet))
	fmt.Println(aa.String())
}