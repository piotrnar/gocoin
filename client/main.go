package main

import (
	"fmt"
	"os"
	"flag"
	"piotr/btc"
	"time"
	"bufio"
	"strings"
	"strconv"
)


var (
	//host *string = flag.String("c", "blockchain.info:8333", "Connect to specified host")
	host *string = flag.String("c", "127.0.0.1:8333", "Connect to specified host")
	listen *bool = flag.Bool("l", false, "Listen insted of connecting")
	verbose *bool = flag.Bool("v", false, "Verbose mode")
	testnet *bool = flag.Bool("t", false, "Use Testnet")


	GenesisBlock *btc.Uint256
	Magic [4]byte
	BlockChain *btc.Chain
	BlockDatabase *btc.BlockDB

	dbg uint64
)


type command struct {
	src string
	str string
	dat []byte
}


func load_database() {
	println("Loading...")
	start := time.Now().UnixNano()
	BlockChain.Load()
	stop := time.Now().UnixNano()
	fmt.Printf("Operation took: %.3fs\n", float64(stop-start)/1e9)
}


func save_database() {
	println("Saving...")
	start := time.Now().UnixNano()
	BlockChain.Save()
	stop := time.Now().UnixNano()
	fmt.Printf("Operation took: %.3fs\n", float64(stop-start)/1e9)
}


func init_blockchain() {
	var dir string
	
	if *testnet { // testnet3
		dir = os.Getenv("APPDATA")+"/Bitcoin/testnet3/blocks"
		GenesisBlock = btc.NewUint256FromString("000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943")
		Magic = [4]byte{0x0B,0x11,0x09,0x07}
	} else {
		dir = os.Getenv("APPDATA")+"/Bitcoin/blocks"
		GenesisBlock = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
		Magic = [4]byte{0xF9,0xBE,0xB4,0xD9}
	}

	BlockDatabase = btc.NewBlockDB(dir, Magic)
	BlockChain = btc.NewChain(BlockDatabase, GenesisBlock)
	load_database()
}


func do_userif(out chan *command) {
	for {
		li, _, _ := bufio.NewReader(os.Stdin).ReadLine()
		if len(li) > 0 {
			c := new(command)
			c.src = "ui"
			c.str = string(li[:])
			out <- c
		}
	}
}



func list_unspent(addr string) {
	a, e := btc.NewAddrFromString(addr)
	if e != nil {
		println(e.Error())
		return
	}
	unsp := BlockChain.GetUnspentFromPkScr(a.OutScript())
	var sum uint64
	for i := range unsp {
		fmt.Println(unsp[i].String())
		sum += unsp[i].Value
	}
	fmt.Printf("Total %.8f unspent BTC at address %s\n", float64(sum)/1e8, a.Enc58str);
}


func main() {
	if flag.Lookup("h") != nil {
		flag.PrintDefaults()
		os.Exit(0)
	}
	flag.Parse()

	init_blockchain()

	ch := make(chan *command, 100)
	
	go do_network(ch)
	go do_userif(ch)

	for {
		msg := <- ch
		//println("got msg", msg.src)
		if msg.src=="ui" {
			if strings.HasPrefix(msg.str, "unspent") {
				list_unspent(strings.Trim(msg.str[7:], " "))
			} else if strings.HasPrefix(msg.str, "dbg") {
				dbg, _ = strconv.ParseUint(msg.str[3:], 10, 64)
			} else {
				switch msg.str {
					case "i": fmt.Println(BlockChain.Stats())
					case "s": save_database()
					case "q": os.Exit(0) 
				}
			}
		} else if msg.src=="net" {
			switch msg.str {
				case "bl":
					println("New block received - len", len(msg.dat))
					bl, e := btc.NewBlock(msg.dat[:])
					if e == nil {
						e = bl.CheckBlock()
						if e == nil {
							e = BlockChain.AcceptBlock(bl)
							if e == nil {
								println("New block accepted", BlockChain.BlockTreeEnd.Height)
								BlockDatabase.AddToExtraIndex(bl)
							} else {
								println(e.Error())
							}
						} else {
							println(e.Error())
						}
					} else {
						println(e.Error())
					}
			}
		}
	}

}
