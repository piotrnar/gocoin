package main

import (
	"os"
	"fmt"
	"time"
	"bytes"
	"runtime"
	"runtime/debug"
	"github.com/piotrnar/gocoin/btc"
	_ "github.com/piotrnar/gocoin/btc/qdb"
	"github.com/piotrnar/gocoin/tools/utils"
)


const (
	TheGenesis  = "000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f"
	LastTrusted = "00000000000000015fba0ce44dae9ebc974d3b7d49349711149314a8cdd9fadc" // #280880
)


var (
	MAX_CONNECTIONS uint32 = 20
	killchan chan os.Signal = make(chan os.Signal)
	Magic [4]byte
	StartTime time.Time
	GocoinHomeDir string
	TheBlockChain *btc.Chain

	GenesisBlock *btc.Uint256 = btc.NewUint256FromString(TheGenesis)
	HighestTrustedBlock *btc.Uint256 = btc.NewUint256FromString(LastTrusted)
	TrustUpTo uint32
	GlobalExit bool
)


func main() {
	fmt.Println("Gocoin blockchain downloader version", btc.SourcesTag)

	StartTime = time.Now()
	runtime.GOMAXPROCS(runtime.NumCPU()) // It seems that Go does not do it by default
	debug.SetGCPercent(50)

	add_ip_str("46.253.195.50") // seed node

	Magic = [4]byte{0xF9,0xBE,0xB4,0xD9}
	if len(os.Args)<2 {
		GocoinHomeDir = utils.BitcoinHome() + "gocoin" + string(os.PathSeparator)
	} else {
		GocoinHomeDir = os.Args[1]
		if GocoinHomeDir[0]!=os.PathSeparator {
			GocoinHomeDir += string(os.PathSeparator)
		}
	}
	GocoinHomeDir += "btcnet" + string(os.PathSeparator)
	fmt.Println("GocoinHomeDir:", GocoinHomeDir)

	utils.LockDatabaseDir(GocoinHomeDir)
	defer utils.UnlockDatabaseDir()

	TheBlockChain = btc.NewChain(GocoinHomeDir, GenesisBlock, false)

	go do_usif()

	download_headers()
	if GlobalExit {
		return
	}

	/*
	fmt.Println("tuning to the fastest peers... (enter 'g' to continue)")
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
			fmt.Println("All the blocks up to", TrustUpTo, "are assumed trusted")
			break
		}
	}

	for n:=TheBlockChain.BlockTreeEnd; n!=nil && n.Height>TheBlockChain.BlockTreeEnd.Height-BSLEN; n=n.Parent {
		blocksize_update(int(n.BlockSize))
	}

	fmt.Println("Downloading blocks - BlocksToGet:", len(BlocksToGet), "  avg_size:", avg_block_size())
	usif_prompt()
	StartTime = time.Now()
	get_blocks()
	fmt.Println("Up to block", TheBlockChain.BlockTreeEnd.Height, "in", time.Now().Sub(StartTime).String())
	close_all_connections()

	StartTime = time.Now()
	fmt.Println("All blocks done - defrag unspent...")
	for i:=0; i<1000; i++ {
		TheBlockChain.Unspent.Idle()
	}
	fmt.Println("Defrag unspent done in", time.Now().Sub(StartTime).String())
	TheBlockChain.Sync()
	TheBlockChain.Close()

	return
}
