package main

import (
	"os"
	"time"
	"fmt"
	"encoding/hex"
	"github.com/piotrnar/gocoin/blockdb"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/btc/leveldb"
	"flag"
)

var testnet *bool = flag.Bool("t", false, "use testnet")
var rescan *bool = flag.Bool("r", false, "rescan")

var GenesisBlock *btc.Uint256

var Magic [4]byte
var BlockDatabase *blockdb.BlockDB

func main() {
	flag.Parse()
	var addr string

	var dir string
	
	if *testnet { // testnet3
		dir = os.Getenv("HOME")+"/btc/testnet3/blocks"
		GenesisBlock = btc.NewUint256FromString("000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943")
		Magic = [4]byte{0x0B,0x11,0x09,0x07}
		leveldb.Testnet = true
		leveldb.Testnet = true
		addr = "mwZSC78JGfS6NY7R57aFeJQp4HgRCadHze"
	} else {
		dir = os.Getenv("HOME")+"/btc/blocks"
		GenesisBlock = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
		Magic = [4]byte{0xF9,0xBE,0xB4,0xD9}
		//addr = "19vPUYV7JE45ZP9z11RZCFcBHU1KXpUcNv"
		addr = "1MBfj713pjbtFHeegKwxrB8oZwYPHgC9mL"
	}

	BlockDatabase = blockdb.NewBlockDB(dir, Magic)

	//btc.TestRollback = true
	chain := btc.NewChain(GenesisBlock, *rescan)
	//println(chain.Stats(), addr)

	var bl *btc.Block
	var er error
	var dat []byte
	var blkcnt, totbytes uint64

	start := time.Now().UnixNano()
	for {
		dat, er = BlockDatabase.FetchNextBlock()
		if dat==nil || er!=nil {
			println("END of DB file")
			break
		}

		bl, er = btc.NewBlock(dat[:])
		if er != nil {
			println("Block inconsistent:", er.Error())
			break
		}

		if GenesisBlock.Equal(bl.Hash) {
			fmt.Println("Skip genesis block")
			continue
		}

		er = bl.CheckBlock()
		if er != nil {
			println("CheckBlock failed:", er.Error())
			break
		}

		er = chain.AcceptBlock(bl)
		if er != nil {
			println("AcceptBlock failed:", er.Error())
			break
		}

		blkcnt++
		totbytes += uint64(len(bl.Raw))

		if (blkcnt % 10000) == 0 {
			stop := time.Now().UnixNano()
			fmt.Printf("%.3fs, read %d blocks containing %dMB of data...\n", 
				float64(stop-start)/1e9, blkcnt, totbytes>>20)
		}

		// for testing:
		/*if chain.BlockTreeEnd.Height>=177778 {
			break
		}*/
	}
	stop := time.Now().UnixNano()
	fmt.Printf("%.3fs, read %d blocks containing %dMB of data... height=%d\n", 
		float64(stop-start)/1e9, blkcnt, totbytes>>20, chain.BlockTreeEnd.Height)

	a, e := btc.NewAddrFromString(addr)
	if e != nil {
		println(e.Error())
		return
	}
	fmt.Println(hex.EncodeToString(a.OutScript()[:]))
	unsp := chain.GetUnspentFromPkScr(a.OutScript())
	var sum uint64
	for i := range unsp {
		fmt.Println(unsp[i].Output.String())
		sum += unsp[i].Value
	}
	fmt.Printf("Total %.8f unspent BTC at address %s\n", float64(sum)/1e8, a.Enc58str);
	
}

