package main

import (
//	"os"
//	"fmt"
	"github.com/piotrnar/gocoin/btc"
	_ "github.com/piotrnar/gocoin/btc/mysqldb"
//	"time"
	"flag"
)

var testnet *bool = flag.Bool("t", false, "use testnet")

var GenesisBlock *btc.Uint256

func main() {
	flag.Parse()

	if *testnet { // testnet3
		GenesisBlock = btc.NewUint256FromString("000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943")
	} else {
		GenesisBlock = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
	}

	chain := btc.NewChain(GenesisBlock)
	chain.Rescan()
	println(chain.Stats())
}

