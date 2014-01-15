package main

import (
	"os"
	"time"
	"github.com/piotrnar/gocoin/btc"
	_ "github.com/piotrnar/gocoin/btc/qdb"
)


var (
	killchan chan os.Signal = make(chan os.Signal)
	Magic [4]byte
	StartTime time.Time
	GocoinHomeDir string
	BlockChain *btc.Chain
	GenesisBlock *btc.Uint256
)


func main() {
	load_ips()
	if len(AddrDatbase)==0 {
		return
	}

	GenesisBlock = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
	Magic = [4]byte{0xF9,0xBE,0xB4,0xD9}
	GocoinHomeDir = "btcnet"+string(os.PathSeparator)

	go do_usif()

	if false {
		download_headers()
		save_headers()
	} else {
		load_headers()
	}

	do_pings()
	println("pings done")

	println("Now download", len(BlocksToGet), "blocks")
	println("BlocksToGet:", len(BlocksToGet))
	get_blocks()
	println("AllBlocksDone after", time.Now().Sub(StartTime).String())

	return
}
