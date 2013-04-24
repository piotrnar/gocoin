package main

import (
	"os"
	"fmt"
//	"time"
	"flag"
	"bufio"
	"strconv"
//	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
	_ "github.com/piotrnar/gocoin/btc/memdb"
)

var testnet *bool = flag.Bool("t", false, "use testnet")
var autoload *bool = flag.Bool("l", false, "auto load blocks")

var GenesisBlock *btc.Uint256
var BlockChain *btc.Chain

var sin = bufio.NewReader(os.Stdin)


func printstat() {
	fmt.Println(BlockChain.Stats())
}


func askUserInt(ask string) int {
	ask = "Global commands: [I]nfo, [S]ave, [Q]uit\n" + ask + "\nEnter your choice: "
	for {
		fmt.Println("============================================")
		fmt.Print(ask)
		li, _, _ := sin.ReadLine()
		fmt.Println("............................................")
		n, e := strconv.ParseInt(string(li[:]), 10, 32)
		if e == nil {
			fmt.Println()
			return int(n)
		}
		switch string(li[:]) {
			case "s": BlockChain.Unspent.Save()
			case "i": printstat()
			case "q":
				BlockChain.Close()
				os.Exit(0)
		}
	}
	return 0
}


func show_unspent() {
	fmt.Print("Enter btcoin address: ")
	li, _, _ := sin.ReadLine()
	if len(li)>0 {
		s := string(li[:])
		ad, e := btc.NewAddrFromString(s)
		if e != nil {
			fmt.Println(e.Error())
			return
		}
		fmt.Println("dobry adres:", ad.Enc58str)
		/*res := BlockChain.GetAllUnspent(ad)
		var tot uint64
		for i := range res {
			fmt.Printf("%3d) %s - %15.8f BTC\n", i, res[i].Output.String(), float64(res[i].Value)/1e8)
			tot += res[i].Value
		}
		fmt.Printf("Total : %.8f BTC\n", float64(tot)/1e8)*/
	}
}


func menu_main() {
	for {
		cmd := askUserInt(" 1) show unspent\n 2) nothing\n 3) nothing")
		switch cmd {
			case 1:
				show_unspent()
		}
	}
}


func main() {
	flag.Parse()

	if *testnet { // testnet3
		GenesisBlock = btc.NewUint256FromString("000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943")
	} else {
		GenesisBlock = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
	}

	fmt.Println("Opening blockchain....")
	BlockChain = btc.NewChain(GenesisBlock, false)
	menu_main()
}


