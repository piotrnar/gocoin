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
	GlobalExit bool
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
	if len(os.Args)<2 {
		GocoinHomeDir = utils.BitcoinHome()
	} else {
		GocoinHomeDir = os.Args[1]
		if GocoinHomeDir[0]!=os.PathSeparator {
			GocoinHomeDir += string(os.PathSeparator)
		}
	}
	GocoinHomeDir += "btcnet" + string(os.PathSeparator)
	println("GocoinHomeDir:", GocoinHomeDir)

	utils.LockDatabaseDir(GocoinHomeDir)
	defer utils.UnlockDatabaseDir()

	TheBlockChain = btc.NewChain(GocoinHomeDir, GenesisBlock, false)

	go do_usif()

	download_headers()
	if GlobalExit {
		return
	}

	/*
	println("tuning to the fastest peers... (enter 'g' to continue)")
	StartTime = time.Now()
	usif_prompt()
	do_pings()
	if GlobalExit {
		return
	}
	*/

	for k, h := range BlocksToGet {
		if bytes.Equal(h[:], HighestTrustedBlock.Hash[:]) {
			TrustUpTo = k
			println("All the blocks up to", TrustUpTo, "are assumed trusted")
			break
		}
	}

	for n:=TheBlockChain.BlockTreeEnd; n!=nil && n.Height>TheBlockChain.BlockTreeEnd.Height-BSLEN; n=n.Parent {
		blocksize_update(int(n.BlockSize))
	}

	println("Downloading blocks - BlocksToGet:", len(BlocksToGet), "  avg_size:", avg_block_size())
	usif_prompt()
	StartTime = time.Now()
	get_blocks()
	println("Sync DB Now...")
	TheBlockChain.Sync()
	TheBlockChain.Close()
	println("Up to block", TheBlockChain.BlockTreeEnd.Height, "in", time.Now().Sub(StartTime).String())

	return
}
