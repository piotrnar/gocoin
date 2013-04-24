package main

/*
This tool converts bitcoin's client blockchain,
into bocoin's blockchain data and their index.
*/

import (
	"os"
	"time"
	"fmt"
	"github.com/piotrnar/gocoin/blockdb"
	"github.com/piotrnar/gocoin/btc"
	_ "github.com/piotrnar/gocoin/btc/memdb"
	"flag"
)

var testnet *bool = flag.Bool("t", false, "use testnet")
var rescan *bool = flag.Bool("r", false, "rescan")
var alwaystrust *bool = flag.Bool("at", false, "always trust (do not eval scripts)")
var stopat *int = flag.Int("stop", 0, "stop at this block height (0 to never stop)")

var GenesisBlock *btc.Uint256

var Magic [4]byte
var BlockDatabase *blockdb.BlockDB

var failcnt uint64

func stat(tim int64, blkcnt, totbytes uint64, height uint32) {
	sec := float64(tim)/1e9
	mbs := float64(totbytes) / (1024*1024)
	fmt.Printf("%.3fs, read %d blocks containing %.1fMB of data @ height %d - %.3fMB/sec (%d fails)\n", 
		sec, blkcnt, mbs, height, mbs/sec, failcnt)
	btc.ShowProfileData()
}


func main() {
	flag.Parse()

	var dir string
	
	if *testnet { // testnet3
		//dir = os.Getenv("HOME")+"/btc/testnet3/blocks"
		dir = os.Getenv("APPDATA")+"/Bitcoin/testnet3/blocks"
		GenesisBlock = btc.NewUint256FromString("000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943")
		Magic = [4]byte{0x0B,0x11,0x09,0x07}
	} else {
		//dir = os.Getenv("HOME")+"/btc/blocks"
		dir = os.Getenv("APPDATA")+"/Bitcoin/blocks"
		GenesisBlock = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
		Magic = [4]byte{0xF9,0xBE,0xB4,0xD9}
	}

	BlockDatabase = blockdb.NewBlockDB(dir, Magic)

	//btc.TestRollback = true
	chain := btc.NewChain(GenesisBlock, *rescan)
	chain.DoNotSync = true
	//println(chain.Stats(), addr)

	var bl *btc.Block
	var er error
	var dat []byte
	var blkcnt, totbytes uint64

	start := time.Now().UnixNano()
	prv := start
	for {
		now := time.Now().UnixNano()
		if now-prv >= 10e9 {
			prv = now
			stat(now-start, blkcnt, totbytes, chain.BlockTreeEnd.Height)
		}

		btc.ChSta("db.FetchNextBlock")
		dat, er = BlockDatabase.FetchNextBlock()
		btc.ChSto("db.FetchNextBlock")
		if dat==nil || er!=nil {
			println("END of DB file")
			break
		}

		bl, er = btc.NewBlock(dat[:])
		if er != nil {
			println("Block inconsistent:", er.Error())
			break
		}

		if *alwaystrust {
			bl.Trusted = true
		}

		er = bl.CheckBlock()
		if er != nil {
			println("CheckBlock failed:", er.Error())
			//failcnt++
			//continue
			break
		}

		er = chain.AcceptBlock(bl)
		if er != nil {
			println("AcceptBlock failed:", er.Error())
			break
			failcnt++
			continue
		}

		blkcnt++
		totbytes += uint64(len(bl.Raw))
		//println(blkcnt, totbytes)

		if *stopat != 0 {
			cur := chain.BlockTreeEnd
			if cur.Height>=uint32(*stopat) {
				break
			}
		}
	}

	stop := time.Now().UnixNano()
	stat(stop-start, blkcnt, totbytes, chain.BlockTreeEnd.Height)

	btc.ShowProfileData()
	println("Saving database...")
	chain.Unspent.Save()

	chain.Close()
	fmt.Println("Finished. Press Ctrl+C")
	for {
		time.Sleep(1e9)
	}
}

