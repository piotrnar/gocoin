package main

import (
	"fmt"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/btc/wierddb"
	"flag"
)

var testnet *bool = flag.Bool("t", false, "use testnet")

var GenesisBlock *btc.Uint256

func main() {
	flag.Parse()

	if *testnet { // testnet3
		GenesisBlock = btc.NewUint256FromString("000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943")
		wierddb.Testnet = true
	} else {
		GenesisBlock = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
	}

	chain := btc.NewChain(GenesisBlock)
	chain.Rescan()
	println(chain.Stats())

	// List all unspent
	//chain.Db.ListUnspent()
	//a, e := btc.NewAddrFromString("mwZSC78JGfS6NY7R57aFeJQp4HgRCadHze")
	a, e := btc.NewAddrFromString("19vPUYV7JE45ZP9z11RZCFcBHU1KXpUcNv")
	if e != nil {
		println(e.Error())
		return
	}
	fmt.Println(hex.EncodeToString(a.OutScript()[:]))
	unsp := chain.GetUnspentFromPkScr(a.OutScript())
	var sum uint64
	for i := range unsp {
		fmt.Println(unsp[i].String())
		sum += unsp[i].Value
	}
	fmt.Printf("Total %.8f unspent BTC at address %s\n", float64(sum)/1e8, a.Enc58str);
}

