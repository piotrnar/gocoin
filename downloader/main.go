package main

import (
	"os"
	"fmt"
	"time"
	"flag"
	"bytes"
	"runtime"
	"os/signal"
	"runtime/debug"
	"github.com/piotrnar/gocoin/btc"
	//"github.com/piotrnar/gocoin/qdb"
	"github.com/piotrnar/gocoin/btc/qdb"
	"github.com/piotrnar/gocoin/tools/utils"
)


const (
	TheGenesis  = "000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f"
)


var (
	Magic [4]byte = [4]byte{0xF9,0xBE,0xB4,0xD9}
	StartTime time.Time
	TheBlockChain *btc.Chain

	GenesisBlock *btc.Uint256 = btc.NewUint256FromString(TheGenesis)
	TrustUpTo uint32
	GlobalExit bool

	// CommandLineSwitches
	LastTrustedBlock string     // -t
	GocoinHomeDir string        // -d
	OnlyStoreBlocks bool        // -b
	MaxNetworkConns uint        // -n
	GCPerc int                  // -g
)


func parse_command_line() {
	GocoinHomeDir = utils.BitcoinHome() + "gocoin" + string(os.PathSeparator)

	var help bool
	flag.BoolVar(&OnlyStoreBlocks, "b", false, "Only store blocks, without parsing them into UTXO database")
	flag.StringVar(&GocoinHomeDir, "d", GocoinHomeDir, "Specify the home directory")
	flag.StringVar(&LastTrustedBlock, "t", "auto", "Specify the highest trusted block hash (use \"all\" for all)")
	flag.UintVar(&MaxNetworkConns, "n", 20, "Set maximum number of network connections for chain download")
	flag.IntVar(&GCPerc, "g", 0, "Set waste percentage treshold for Go's garbage collector")
	flag.BoolVar(&help, "h", false, "Show this help")
	flag.Parse()
	if help {
		flag.PrintDefaults()
		os.Exit(0)
	}
}


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
	os.Stdout.Sync()
	for {
		if !TheBlockChain.Unspent.Idle() {
			break
		}
		fmt.Print(".")
		os.Stdout.Sync()
	}
	fmt.Println("\nDefrag unspent done in", time.Now().Sub(StartTime).String())
	os.Stdout.Sync()
	TheBlockChain.Close()
}


func setup_runtime_vars() {
	runtime.GOMAXPROCS(runtime.NumCPU()) // It seems that Go does not do it by default
	qdb.MinBrowsableOutValue = 0
	if GCPerc>0 {
		debug.SetGCPercent(GCPerc)
	}
	//qdb.SetDefragPercent(100)
	//qdb.SetMaxPending(1000, 10000)
}


func main() {
	fmt.Println("Gocoin blockchain downloader version", btc.SourcesTag)
	parse_command_line()
	setup_runtime_vars()

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

	var HighestTrustedBlock *btc.Uint256
	if LastTrustedBlock=="all" {
		HighestTrustedBlock = TheBlockChain.BlockTreeEnd.BlockHash
		fmt.Println("Assume all blocks trusted")
	} else if LastTrustedBlock=="auto" {
		if LastBlockHeight>6 {
			ha := BlocksToGet[LastBlockHeight]
			HighestTrustedBlock = btc.NewUint256(ha[:])
			fmt.Println("Assume last trusted block as", HighestTrustedBlock.String())
		} else {
			fmt.Println("-t=auto ignored since LastBlockHeight is only", LastBlockHeight)
		}
	} else if LastTrustedBlock!="" {
		HighestTrustedBlock = btc.NewUint256FromString(LastTrustedBlock)
	}
	if HighestTrustedBlock != nil {
		for k, h := range BlocksToGet {
			if bytes.Equal(h[:], HighestTrustedBlock.Hash[:]) {
				TrustUpTo = k
				fmt.Println("All the blocks up to", TrustUpTo, "are assumed trusted")
				break
			}
		}
	} else {
		fmt.Println("None of the blocks is to be assumed trusted (it will be very slow).")
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

	close_blockchain()

	return
}
