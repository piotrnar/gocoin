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
	"github.com/piotrnar/gocoin/others/utils"
)



var (
	Magic [4]byte = [4]byte{0xF9,0xBE,0xB4,0xD9}
	StartTime time.Time
	TheBlockChain *btc.Chain

	GenesisBlock *btc.Uint256 = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
	TrustUpTo uint32
	GlobalExit bool

	// CommandLineSwitches
	LastTrustedBlock string     // -trust
	GocoinHomeDir string        // -d
	OnlyStoreBlocks bool        // -b
	MaxNetworkConns uint        // -n
	GCPerc int                  // -g
	SeedNode string             // -s
	DoThePings bool             // -p
	MemForBlocks uint           // -m (in megabytes)
	Testnet bool                // -t
)


func parse_command_line() {
	GocoinHomeDir = utils.BitcoinHome() + "gocoin" + string(os.PathSeparator)

	flag.BoolVar(&OnlyStoreBlocks, "b", false, "Only store blocks, without parsing them into UTXO database")
	flag.BoolVar(&Testnet, "t", false, "Use Testnet3")
	flag.StringVar(&GocoinHomeDir, "d", GocoinHomeDir, "Specify the home directory")
	flag.StringVar(&LastTrustedBlock, "trust", "auto", "Specify the highest trusted block hash (use \"all\" for all)")
	flag.StringVar(&SeedNode, "s", "", "Specify IP of the node to fetch headers from")
	flag.UintVar(&MaxNetworkConns, "n", 20, "Set maximum number of network connections for chain download")
	flag.IntVar(&GCPerc, "g", 0, "Set waste percentage treshold for Go's garbage collector")
	flag.BoolVar(&DoThePings, "p", false, "Execute the pings procedure first to find the fastest peers")

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

	if !add_ip_str(SeedNode) {
		println("You need to specify IP address of a fast seed node.")
		println("For example run it like this: downloader -s 89.31.102.237")
		return
	}
	load_ips() // other seed nodes

	if len(GocoinHomeDir)>0 && GocoinHomeDir[len(GocoinHomeDir)-1]!=os.PathSeparator {
		GocoinHomeDir += string(os.PathSeparator)
	}
	if Testnet {
		GocoinHomeDir += "tstnet" + string(os.PathSeparator)
		Magic = [4]byte{0x0B,0x11,0x09,0x07}
		DefaultTcpPort = 18333
		GenesisBlock = btc.NewUint256FromString("000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943")
		fmt.Println("Using testnet3")
	} else {
		GocoinHomeDir += "btcnet" + string(os.PathSeparator)
	}
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

	fmt.Println("Downloading headers from the seed peer", SeedNode)
	download_headers()
	if GlobalExit {
		close_blockchain()
		return
	}

	if DoThePings {
		fmt.Println("Tuning to other peers and trying to find the fastest ones.")
		fmt.Println("Execute command 'g' to continue to block chain download.")
		fmt.Println("Otherwise it will auto-continue after 15 minutes.")
		usif_prompt()
		do_pings()
		fmt.Println("Pings done.")
		usif_prompt()
	}

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
