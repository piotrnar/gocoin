package main

import (
	"os"
	//"fmt"
	"time"
	//"os/signal"
	"github.com/piotrnar/gocoin/btc"
	_ "github.com/piotrnar/gocoin/btc/qdb"
)


var (
	killchan chan os.Signal = make(chan os.Signal)
	Magic [4]byte
	StartTime time.Time
	GocoinHomeDir string
	BlockChain *btc.Chain
)


func printstats() {
	println("stats")
}

func main() {
	StartTime = time.Now()

	GenesisBlock := btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
	Magic = [4]byte{0xF9,0xBE,0xB4,0xD9}
	GocoinHomeDir = "btcnet"+string(os.PathSeparator)

	BlockChain = btc.NewChain(GocoinHomeDir, GenesisBlock, false)
	if btc.AbortNow || BlockChain==nil {
		return
	}

	new_connection("46.4.121.99")
	//new_connection("198.12.127.2")
	//new_connection("85.17.239.32")
	//new_connection("94.23.228.130")
	//new_connection("129.132.230.75")
	//new_connection("178.63.63.214")
	get_headers()
	return
}
