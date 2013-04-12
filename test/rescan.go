package main

import (
	"fmt"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/btc/leveldb"
	"flag"
)

var testnet *bool = flag.Bool("t", true, "use testnet")

var GenesisBlock *btc.Uint256

func main() {
	flag.Parse()
	var addr string

	if *testnet { // testnet3
		GenesisBlock = btc.NewUint256FromString("000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943")
		leveldb.Testnet = true
		addr = "mwZSC78JGfS6NY7R57aFeJQp4HgRCadHze"
	} else {
		GenesisBlock = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
		addr = "19vPUYV7JE45ZP9z11RZCFcBHU1KXpUcNv"
	}

	chain := btc.NewChain(GenesisBlock, true)
	println(chain.Stats())

	a, e := btc.NewAddrFromString(addr)
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

