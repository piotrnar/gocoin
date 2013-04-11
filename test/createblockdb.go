package main

import (
	"os"
	"fmt"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/btc/mysqldb"
	"github.com/piotrnar/gocoin/btc/blockdb"
	"time"
	"flag"
)

var testnet *bool = flag.Bool("t", false, "use testnet")

var GenesisBlock *btc.Uint256
var Magic [4]byte
var BlockDatabase *blockdb.BlockDB

type btn struct {
	Hash btc.Uint256
	Height uint32
	parent *btn
}

var bidx map[btc.Uint256] *btn
var root *btn

func main() {
	flag.Parse()

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

	bidx = make(map[btc.Uint256] *btn, 300000)

	BlockDatabase = blockdb.NewBlockDB(dir, Magic)
	db := mysqldb.NewDb()
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
			println("Block inconsisrent:", er.Error())
			return
		}

		blkcnt++
		totbytes += uint64(len(bl.Raw))

		nod := new(btn)
		nod.Hash = *bl.Hash

		ParHash := btc.NewUint256(bl.GetParent()[:])
		par, ok := bidx[*ParHash]
		if !ok {
			if root == nil {
				root = nod
			} else {
				println("Unknown parent, but root already set")
				println(ParHash.String())
				os.Exit(1)
			}
		} else {
			nod.Height = par.Height+1
			nod.parent = par
		}

		bidx[nod.Hash] = nod
		//println(nod.Hash.String())
		
		if true {
			er = db.BlockAdd(nod.Height, bl)
		}

		if (nod.Height % 10000) == 0 {
			stop := time.Now().UnixNano()
			fmt.Printf("%.3fs, read %d blocks containing %dMB of data...\n", 
				float64(stop-start)/1e9, blkcnt, totbytes>>20)
		}
	}
	db.Close()
	stop := time.Now().UnixNano()
	fmt.Printf("Operation took: %.3fs, read %d blocks containing %dMB of data\n", 
		float64(stop-start)/1e9, blkcnt, totbytes>>20)
}

