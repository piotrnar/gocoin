package main

import (
	"os"
	"fmt"
	"time"
	"flag"
	"bytes"
	"runtime"
	"os/signal"
	//"runtime/debug"
	"github.com/piotrnar/gocoin/btc"
	//"github.com/piotrnar/gocoin/qdb"
	_ "github.com/piotrnar/gocoin/btc/qdb"
	"github.com/piotrnar/gocoin/tools/utils"
)


const (
	TheGenesis  = "000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f"
)


var (
	MAX_CONNECTIONS uint32 = 20
	Magic [4]byte = [4]byte{0xF9,0xBE,0xB4,0xD9}
	StartTime time.Time
	TheBlockChain *btc.Chain

	GenesisBlock *btc.Uint256 = btc.NewUint256FromString(TheGenesis)
	TrustUpTo uint32
	GlobalExit bool

	// CommandLineSwitches
	LastTrustedBlock = "00000000000000021b07704899dd81d92bb288b47a95004f3ef82565a55ffb1f" // #281296
	GocoinHomeDir string
	OnlyStoreBlocks bool
)


func open_blockchain() (abort bool) {
	// Disable Ctrl+C
	killchan := make(chan os.Signal)
	signal.Notify(killchan, os.Interrupt, os.Kill)
	fmt.Println("Opening blockchain... (Ctrl-C to interrupt)")
	__exit := make(chan bool)
	go func() {
		for {
			select {
				case s := <-killchan:
					fmt.Println(s)
					abort = true
					btc.AbortNow = true
				case <-__exit:
					return
			}
		}
	}()
	TheBlockChain = btc.NewChain(GocoinHomeDir, GenesisBlock, false)
	__exit <- true
	return
}

func close_blockchain() {
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
}


func main() {
	fmt.Println("Gocoin blockchain downloader version", btc.SourcesTag)

	GocoinHomeDir = utils.BitcoinHome() + "gocoin" + string(os.PathSeparator)

	var help bool
	flag.BoolVar(&OnlyStoreBlocks, "b", false, "Only store blocks, without parsing them into UTXO database")
	flag.StringVar(&GocoinHomeDir, "d", GocoinHomeDir, "Specify the home directory")
	flag.BoolVar(&help, "h", false, "Show this help")
	flag.Parse()
	if help {
		flag.PrintDefaults()
		return
	}

	// Setup runtime variables
	runtime.GOMAXPROCS(runtime.NumCPU()) // It seems that Go does not do it by default
	//debug.SetGCPercent(100)
	//qdb.SetDefragPercent(100)
	//qdb.SetMaxPending(1000, 10000)

	add_ip_str("46.253.195.50") // seed node
	load_ips() // other seed nodes

	if len(GocoinHomeDir)>0 && GocoinHomeDir[len(GocoinHomeDir)-1]!=os.PathSeparator {
		GocoinHomeDir += string(os.PathSeparator)
	}
	GocoinHomeDir += "btcnet" + string(os.PathSeparator)
	fmt.Println("GocoinHomeDir:", GocoinHomeDir)

	utils.LockDatabaseDir(GocoinHomeDir)
	defer utils.UnlockDatabaseDir()

	StartTime = time.Now()
	if open_blockchain() {
		fmt.Printf("Blockchain opening aborted\n")
		close_blockchain()
		return
	}
	fmt.Println("Blockchain open in", time.Now().Sub(StartTime))

	go do_usif()

	download_headers()
	if GlobalExit {
		close_blockchain()
		return
	}

	//do_pings()

	HighestTrustedBlock := btc.NewUint256FromString(LastTrustedBlock)
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

	go BlocksMutex_Monitor()

	fmt.Println("Downloading blocks - BlocksToGet:", len(BlocksToGet), "  avg_size:", avg_block_size())
	usif_prompt()
	StartTime = time.Now()
	get_blocks()
	fmt.Println("Up to block", TheBlockChain.BlockTreeEnd.Height, "in", time.Now().Sub(StartTime).String())
	close_all_connections()

	close_blockchain()

	return
}
