package main

import (
	"os"
	"fmt"
	"time"
	"flag"
	"bytes"
	"runtime"
	"os/signal"
	"sync/atomic"
	"runtime/debug"
	"github.com/piotrnar/gocoin/lib"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/qdb"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/lib/others/peersdb"
)



var (
	Magic [4]byte = [4]byte{0xF9,0xBE,0xB4,0xD9}
	StartTime time.Time
	TheBlockChain *chain.Chain

	GenesisBlock *btc.Uint256 = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
	TrustUpTo uint32
	globalexit uint32

	// CommandLineSwitches
	LastTrustedBlock string     // -trust
	GocoinHomeDir string        // -d
	OnlyStoreBlocks bool        // -b
	MaxNetworkConns uint        // -n
	GCPerc int                  // -g
	SeedNode string             // -s
	MemForBlocks uint           // -m (in megabytes)
	Testnet bool                // -t
)


func GlobalExit() bool {
	return atomic.LoadUint32(&globalexit)!=0
}

func Exit() {
	atomic.StoreUint32(&globalexit, 1)
}


func parse_command_line() {
	GocoinHomeDir = sys.BitcoinHome() + "gocoin" + string(os.PathSeparator)

	flag.BoolVar(&OnlyStoreBlocks, "b", false, "Only store blocks, without parsing them into UTXO database")
	flag.BoolVar(&Testnet, "t", false, "Use Testnet3")
	flag.StringVar(&GocoinHomeDir, "d", GocoinHomeDir, "Specify the home directory")
	flag.StringVar(&LastTrustedBlock, "trust", "auto", "Specify the highest trusted block hash (use \"all\" for all)")
	flag.StringVar(&SeedNode, "s", "", "Specify IP of the node to fetch headers from")
	flag.UintVar(&MaxNetworkConns, "n", 10, "Set maximum number of network connections for chain download")
	flag.IntVar(&GCPerc, "g", 0, "Set waste percentage treshold for Go's garbage collector")

	flag.UintVar(&MemForBlocks, "m", 64, "Set memory buffer for cached block data (value in megabytes)")

	var help bool
	flag.BoolVar(&help, "h", false, "Show this help")
	flag.Parse()
	if help {
		flag.PrintDefaults()
		os.Exit(0)
	}

	MemForBlocks <<= 20 // Convert megabytes to bytes
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
					chain.AbortNow = true
				case <-__exit:
					return
			}
		}
	}()
	TheBlockChain = chain.NewChain(GocoinHomeDir, GenesisBlock, false)
	__exit <- true
	return
}



func setup_runtime_vars() {
	runtime.GOMAXPROCS(runtime.NumCPU()) // It seems that Go does not do it by default
	if GCPerc>0 {
		debug.SetGCPercent(GCPerc)
	}
	qdb.SetDefragPercent(300)
	//qdb.SetMaxPending(1000, 10000)
}


func main() {
	fmt.Println("Gocoin blockchain downloader version", lib.Version)
	parse_command_line()
	setup_runtime_vars()

	if len(GocoinHomeDir)>0 && GocoinHomeDir[len(GocoinHomeDir)-1]!=os.PathSeparator {
		GocoinHomeDir += string(os.PathSeparator)
	}
	if Testnet {
		GocoinHomeDir += "tstnet" + string(os.PathSeparator)
		Magic = [4]byte{0x0B,0x11,0x09,0x07}
		GenesisBlock = btc.NewUint256FromString("000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943")
		fmt.Println("Using testnet3")
	} else {
		GocoinHomeDir += "btcnet" + string(os.PathSeparator)
	}
	fmt.Println("GocoinHomeDir:", GocoinHomeDir)

	sys.LockDatabaseDir(GocoinHomeDir)
	defer sys.UnlockDatabaseDir()

	peersdb.Testnet = Testnet
	peersdb.InitPeers(GocoinHomeDir)

	StartTime = time.Now()
	if open_blockchain() {
		fmt.Printf("Blockchain opening aborted\n")
		return
	}
	fmt.Println("Blockchain open in", time.Now().Sub(StartTime))

	go do_usif()

	download_headers()
	if GlobalExit() {
		TheBlockChain.Close()
		return
	}

	var HighestTrustedBlock *btc.Uint256
	if LastTrustedBlock=="all" {
		HighestTrustedBlock = TheBlockChain.BlockTreeEnd.BlockHash
		fmt.Println("Assume all blocks trusted")
	} else if LastTrustedBlock=="auto" {
		if LastBlockHeight>6 {
			use := LastBlockHeight-6
			ha := BlocksToGet[use]
			HighestTrustedBlock = btc.NewUint256(ha[:])
			fmt.Println("Assume last trusted block as", HighestTrustedBlock.String(), "at", use)
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
				break
			}
		}
	} else {
		fmt.Println("WARNING: The trusted block not found (it will be very slow).")
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

	peersdb.ClosePeerDB()

	StartTime = time.Now()
	fmt.Print("All blocks done - defrag unspent")
	qdb.SetDefragPercent(100)
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
