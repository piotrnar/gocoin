package main

import (
	"fmt"
	"os"
	"flag"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/btc/leveldb"
	"bufio"
	"strings"
	"strconv"
)


var (
	//host *string = flag.String("c", "blockchain.info:8333", "Connect to specified host")
	host *string = flag.String("c", "192.168.2.85:18333", "Connect to specified host")
	listen *bool = flag.Bool("l", false, "Listen insted of connecting")
	verbose *bool = flag.Bool("v", false, "Verbose mode")
	testnet *bool = flag.Bool("t", true, "Use Testnet")


	GenesisBlock *btc.Uint256
	Magic [4]byte
	BlockChain *btc.Chain

	dbg uint64
)


type command struct {
	src string
	str string
	dat []byte
}


func init_blockchain() {
	if *testnet { // testnet3
		GenesisBlock = btc.NewUint256FromString("000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943")
		Magic = [4]byte{0x0B,0x11,0x09,0x07}
		leveldb.Testnet = true
	} else {
		GenesisBlock = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
		Magic = [4]byte{0xF9,0xBE,0xB4,0xD9}
	}

	BlockChain = btc.NewChain(GenesisBlock, true)
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
