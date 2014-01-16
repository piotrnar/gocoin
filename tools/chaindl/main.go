package main

import (
	"os"
	"time"
	"bytes"
	"runtime"
	"runtime/debug"
	"github.com/piotrnar/gocoin/btc"
	_ "github.com/piotrnar/gocoin/btc/qdb"
	"github.com/piotrnar/gocoin/tools/utils"
)


var (
	MAX_CONNECTIONS uint32 = 20
	killchan chan os.Signal = make(chan os.Signal)
	Magic [4]byte
	StartTime time.Time
	GocoinHomeDir string
	TheBlockChain *btc.Chain

	GenesisBlock *btc.Uint256
	HighestTrustedBlock *btc.Uint256 = btc.NewUint256FromString("0000000000000001c091ada69f444dc0282ecaabe4808ddbb2532e5555db0c03")
	TrustUpTo uint32
)


func main() {
	StartTime = time.Now()
	runtime.GOMAXPROCS(runtime.NumCPU()) // It seems that Go does not do it by default
	debug.SetGCPercent(50)

	load_ips()
	if len(AddrDatbase)==0 {
		return
	}

	GenesisBlock = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
	Magic = [4]byte{0xF9,0xBE,0xB4,0xD9}
	GocoinHomeDir = "btcnet"+string(os.PathSeparator)

	utils.LockDatabaseDir(GocoinHomeDir)
	defer utils.UnlockDatabaseDir()

	TheBlockChain = btc.NewChain(GocoinHomeDir, GenesisBlock, false)

	go do_usif()

	download_headers()

	StartTime = time.Now()
	if false {
		do_pings()
		println("pings done")
		usif_prompt()
	}

	for k, h := range BlocksToGet {
		if bytes.Equal(h[:], HighestTrustedBlock.Hash[:]) {
			TrustUpTo = k
			println("All the blocks up to", TrustUpTo, "are assumed trusted")
			break
		}
	}

	println("Downloading blocks - BlocksToGet:", len(BlocksToGet))
	usif_prompt()
	StartTime = time.Now()
	get_blocks()
	println("Sync DB Now...")
	TheBlockChain.Sync()
	TheBlockChain.Close()
	println("Up to block", TheBlockChain.BlockTreeEnd.Height, "in", time.Now().Sub(StartTime).String())

	return
}
