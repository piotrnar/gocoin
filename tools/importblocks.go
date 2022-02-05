// This tool can import blockchain database from satoshi client to gocoin
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/lib/others/blockdb"
	"github.com/piotrnar/gocoin/lib/others/sys"
)

const Trust = true // Set this to false if you want to re-check all scripts

var (
	Magic               [4]byte
	GocoinHomeDir       string
	BtcRootDir          string
	GenesisBlock        *btc.Uint256
	prev_EcdsaVerifyCnt uint64
)

func stat(totnsec, pernsec int64, totbytes, perbytes uint64, height uint32) {
	totmbs := float64(totbytes) / (1024 * 1024)
	perkbs := float64(perbytes) / (1024)
	var x string
	cn := btc.EcdsaVerifyCnt() - prev_EcdsaVerifyCnt
	if cn > 0 {
		x = fmt.Sprintf("|  %d -> %d us/ecdsa", cn, uint64(pernsec)/cn/1e3)
		prev_EcdsaVerifyCnt += cn
	}
	fmt.Printf("%.1fMB of data processed. We are at height %d. Processing speed %.3fMB/sec, recent: %.1fKB/s %s\n",
		totmbs, height, totmbs/(float64(totnsec)/1e9), perkbs/(float64(pernsec)/1e9), x)
}

func import_blockchain(dir string) {
	BlockDatabase := blockdb.NewBlockDB(dir, Magic)
	chain := chain.NewChainExt(GocoinHomeDir, GenesisBlock, false, nil, nil)

	var bl *btc.Block
	var er error
	var dat []byte
	var totbytes, perbytes uint64

	fmt.Println("Be patient while importing Satoshi's database... ")
	start := time.Now().UnixNano()
	prv := start
	for {
		now := time.Now().UnixNano()
		if now-prv >= 10e9 {
			stat(now-start, now-prv, totbytes, perbytes, chain.LastBlock().Height)
			prv = now // show progress each 10 seconds
			perbytes = 0
		}

		dat, er = BlockDatabase.FetchNextBlock()
		if dat == nil || er != nil {
			println("END of DB file")
			break
		}

		bl, er = btc.NewBlock(dat[:])
		if er != nil {
			println("Block inconsistent:", er.Error())
			break
		}

		bl.Trusted.Store(Trust)

		_, _, er = chain.CheckBlock(bl)

		if er != nil {
			if er.Error() != "Genesis" {
				println("CheckBlock failed:", er.Error())
				os.Exit(1) // Such a thing should not happen, so let's better abort here.
			}
			continue
		}

		er = chain.AcceptBlock(bl)
		if er != nil {
			println("AcceptBlock failed:", er.Error())
			os.Exit(1) // Such a thing should not happen, so let's better abort here.
		}

		totbytes += uint64(len(bl.Raw))
		perbytes += uint64(len(bl.Raw))
	}

	stop := time.Now().UnixNano()
	stat(stop-start, stop-prv, totbytes, perbytes, chain.LastBlock().Height)

	fmt.Println("Satoshi's database import finished in", (stop-start)/1e9, "seconds")

	fmt.Println("Now saving the new database...")
	chain.Close()
	fmt.Println("Database saved. No more imports should be needed.")
}

func RemoveLastSlash(p string) string {
	if len(p) > 0 && os.IsPathSeparator(p[len(p)-1]) {
		return p[:len(p)-1]
	}
	return p
}

func exists(fn string) bool {
	_, e := os.Lstat(fn)
	return e == nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Specify at least one parameter - a path to the blk0000?.dat files.")
		fmt.Println("By default it should be:", sys.BitcoinHome()+"blocks")
		fmt.Println()
		fmt.Println("If you specify a second parameter, that's where output data will be stored.")
		fmt.Println("Otherwise the output data will go to Gocoin's default data folder.")
		return
	}

	BtcRootDir = RemoveLastSlash(os.Args[1])
	fn := BtcRootDir + string(os.PathSeparator) + "blk00000.dat"
	fmt.Println("Looking for file", fn, "...")
	f, e := os.Open(fn)
	if e != nil {
		println(e.Error())
		os.Exit(1)
	}
	_, e = f.Read(Magic[:])
	f.Close()
	if e != nil {
		println(e.Error())
		os.Exit(1)
	}

	if len(os.Args) > 2 {
		GocoinHomeDir = RemoveLastSlash(os.Args[2]) + string(os.PathSeparator)
	} else {
		GocoinHomeDir = sys.BitcoinHome() + "gocoin" + string(os.PathSeparator)
	}

	if Magic == [4]byte{0x0B, 0x11, 0x09, 0x07} {
		// testnet3
		fmt.Println("There are Testnet3 blocks")
		GenesisBlock = btc.NewUint256FromString("000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943")
		GocoinHomeDir += "tstnet" + string(os.PathSeparator)
	} else if Magic == [4]byte{0xF9, 0xBE, 0xB4, 0xD9} {
		fmt.Println("There are valid Bitcoin blocks")
		GenesisBlock = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
		GocoinHomeDir += "btcnet" + string(os.PathSeparator)
	} else {
		println("blk00000.dat has an unexpected magic")
		os.Exit(1)
	}

	fmt.Println("Importing blockchain data into", GocoinHomeDir, "...")

	if exists(GocoinHomeDir+"blockchain.dat") ||
		exists(GocoinHomeDir+"blockchain.idx") ||
		exists(GocoinHomeDir+"unspent") {
		println("Destination folder contains some database files.")
		println("Either move them somewhere else or delete manually.")
		println("None of the following files/folders must exist before you proceed:")
		println(" *", GocoinHomeDir+"blockchain.dat")
		println(" *", GocoinHomeDir+"blockchain.idx")
		println(" *", GocoinHomeDir+"unspent")
		os.Exit(1)
	}

	import_blockchain(BtcRootDir)
}
