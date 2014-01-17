package main

import (
	"os"
	"fmt"
	"time"
	"bytes"
	"runtime"
	"os/signal"
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
	Magic [4]byte
	StartTime time.Time
	GocoinHomeDir string
	TheBlockChain *btc.Chain

	GenesisBlock *btc.Uint256 = btc.NewUint256FromString(TheGenesis)
	HighestTrustedBlock *btc.Uint256 = btc.NewUint256FromString(LastTrusted)
	TrustUpTo uint32
	GlobalExit bool
	killchan chan os.Signal = make(chan os.Signal)
)


func main() {
	fmt.Println("Gocoin blockchain downloader version", btc.SourcesTag)

	StartTime = time.Now()
	runtime.GOMAXPROCS(runtime.NumCPU()) // It seems that Go does not do it by default
	debug.SetGCPercent(50)

	add_ip_str("46.253.195.50") // seed node
	load_ips() // other seed nodes

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

	// Disable Ctrl+C
	signal.Notify(killchan, os.Interrupt, os.Kill)
	fmt.Println("Opening blockchain... (Ctrl-C to interrupt)")
	__exit := make(chan bool)
	__done := make(chan bool)
	go func() {
		for {
			select {
				case s := <-killchan:
					fmt.Println(s)
					btc.AbortNow = true
				case <-__exit:
					__done <- true
					return
			}
		}
	}()
	sta := time.Now().UnixNano()
	TheBlockChain = btc.NewChain(GocoinHomeDir, GenesisBlock, false)
	sto := time.Now().UnixNano()
	if btc.AbortNow {
		fmt.Printf("Blockchain opening aborted after %.3f seconds\n", float64(sto-sta)/1e9)
		goto finito
		return
	}
	__exit <- true
	_ = <- __done
	fmt.Printf("Blockchain open in %.3f seconds\n", float64(sto-sta)/1e9)

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

finito:
	StartTime = time.Now()
	fmt.Print("All blocks done - defrag unspent")
	for {
		if !TheBlockChain.Unspent.Idle() {
			break
		}
		fmt.Print(".")
	}
	fmt.Println("\nDefrag unspent done in", time.Now().Sub(StartTime).String())
	TheBlockChain.Close()

	return
}
