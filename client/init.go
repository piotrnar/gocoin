package main

import (
	"os"
	"fmt"
	"time"
	"io/ioutil"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/lib/qdb"
	"github.com/piotrnar/gocoin/lib/others/blockdb"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/wallet"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/usif/textui"
	"github.com/piotrnar/gocoin/lib/others/sys"
)


func host_init() {
	var e error
	BtcRootDir := sys.BitcoinHome()
	common.GocoinHomeDir = common.CFG.Datadir+string(os.PathSeparator)

	common.Testnet = common.CFG.Testnet // So chaging this value would will only affect the behaviour after restart
	if common.CFG.Testnet { // testnet3
		common.GenesisBlock = btc.NewUint256FromString("000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943")
		common.Magic = [4]byte{0x0B,0x11,0x09,0x07}
		common.GocoinHomeDir += common.DataSubdir() + string(os.PathSeparator)
		BtcRootDir += "testnet3"+string(os.PathSeparator)
		network.AlertPubKey, _ = hex.DecodeString("04302390343f91cc401d56d68b123028bf52e5fca1939df127f63c6467cdf9c8e2c14b61104cf817d0b780da337893ecc4aaff1309e536162dabbdb45200ca2b0a")
		common.MaxPeersNeeded = 100
	} else {
		common.GenesisBlock = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
		common.Magic = [4]byte{0xF9,0xBE,0xB4,0xD9}
		common.GocoinHomeDir += common.DataSubdir() + string(os.PathSeparator)
		network.AlertPubKey, _ = hex.DecodeString("04fc9702847840aaf195de8442ebecedf5b095cdbb9bc716bda9110971b28a49e0ead8564ff0db22209e0374782c093bb899692d524e9d6a6956e7c5ecbcd68284")
		common.MaxPeersNeeded = 1000
	}

	// Lock the folder
	os.MkdirAll(common.GocoinHomeDir, 0770)
	sys.LockDatabaseDir(common.GocoinHomeDir)

	fi, e := os.Stat(common.GocoinHomeDir+"blockchain.dat")
	if e!=nil {
		os.RemoveAll(common.GocoinHomeDir)
		fmt.Println("You seem to be running Gocoin for the fist time on this PC")
		fi, e = os.Stat(BtcRootDir+"blocks/blk00000.dat")
		if e==nil && fi.Size()>1024*1024 {
			fmt.Println("There is a database from Satoshi client on your disk...")
			if textui.AskYesNo("Do you want to import this database into Gocoin?") {
				import_blockchain(BtcRootDir+"blocks")
			}
		}
	}

	// Create default wallet file if does not exist
	println("wallet dir", common.CFG.Walletdir)
	os.MkdirAll(common.CFG.Walletdir+string(os.PathSeparator)+"stealth", 0770)
	default_wallet_fn := common.CFG.Walletdir + string(os.PathSeparator) + wallet.DefaultFileName
	println("default_wallet_fn", default_wallet_fn)
	fi, _ = os.Stat(default_wallet_fn)
	if fi==nil || fi.IsDir() {
		fmt.Println(default_wallet_fn, "not found")

		old_wallet_location := common.GocoinHomeDir+"wallet.txt"
		// If there is wallet.txt rename it to default.
		fi, _ := os.Stat(old_wallet_location)
		if fi!=nil && !fi.IsDir() {
			fmt.Println("Taking wallet.txt as", default_wallet_fn)
			os.Rename(old_wallet_location, default_wallet_fn)
		} else {
			fmt.Println("Creating empty default wallet at", default_wallet_fn)
			ioutil.WriteFile(default_wallet_fn, []byte(fmt.Sprintln("# Put your wallet's public addresses here")), 0660)
		}
	}

	// cache the current balance of all the addresses from the current wallet files
	wallet.LoadAllWallets()

	fmt.Println("Loading UTXO database while checking balance of", len(wallet.MyWallet.Addrs),
		"addresses... (press Ctrl-C to interrupt)")

	__exit := make(chan bool)
	__done := make(chan bool)
	go func() {
		for {
			select {
				case s := <-killchan:
					fmt.Println(s)
					chain.AbortNow = true
				case <-__exit:
					__done <- true
					return
			}
		}
	}()

	ext := &chain.NewChanOpts{NotifyTxAdd: wallet.TxNotifyAdd,
		NotifyTxDel: wallet.TxNotifyDel, LoadWalk: wallet.NewUTXO}

	sta := time.Now().UnixNano()
	common.BlockChain = chain.NewChainExt(common.GocoinHomeDir, common.GenesisBlock, common.FLAG.Rescan, ext)
	sto := time.Now().UnixNano()
	if chain.AbortNow {
		fmt.Printf("Blockchain opening aborted after %.3f seconds\n", float64(sto-sta)/1e9)
		common.BlockChain.Close()
		sys.UnlockDatabaseDir()
		os.Exit(1)
	}
	wallet.ChainInitDone()
	al, sy := sys.MemUsed()
	fmt.Printf("Blockchain open in %.3f seconds.  %d + %d MB of RAM used (%d)\n",
		float64(sto-sta)/1e9, al>>20, qdb.ExtraMemoryConsumed>>20, sy>>20)
	common.StartTime = time.Now()
	__exit <- true
	_ = <- __done


	// ... and now load the dafault wallet
	wallet.LoadWallet(default_wallet_fn)
	if wallet.MyWallet!=nil {
		wallet.UpdateBalance()
		fmt.Print(wallet.DumpBalance(wallet.MyBalance, nil, false, true))
	}
}


func stat(totnsec, pernsec int64, totbytes, perbytes uint64, height uint32) {
	totmbs := float64(totbytes) / (1024*1024)
	perkbs := float64(perbytes) / (1024)
	var x string
	if btc.EcdsaVerifyCnt > 0 {
		x = fmt.Sprintf("|  %d -> %d us/ecdsa", btc.EcdsaVerifyCnt, uint64(pernsec)/btc.EcdsaVerifyCnt/1e3)
		btc.EcdsaVerifyCnt = 0
	}
	fmt.Printf("%.1fMB of data processed. We are at height %d. Processing speed %.3fMB/sec, recent: %.1fKB/s %s\n",
		totmbs, height, totmbs/(float64(totnsec)/1e9), perkbs/(float64(pernsec)/1e9), x)
}


func import_blockchain(dir string) {
	trust := !textui.AskYesNo("Do you want to verify scripts while importing (will be slow)?")

	BlockDatabase := blockdb.NewBlockDB(dir, common.Magic)
	chain := chain.NewChain(common.GocoinHomeDir, common.GenesisBlock, false)

	var bl *btc.Block
	var er error
	var dat []byte
	var totbytes, perbytes uint64

	chain.DoNotSync = true

	fmt.Println("Be patient while importing Satoshi's database... ")
	start := time.Now().UnixNano()
	prv := start
	for {
		now := time.Now().UnixNano()
		if now-prv >= 10e9 {
			stat(now-start, now-prv, totbytes, perbytes, chain.BlockTreeEnd.Height)
			prv = now  // show progress each 10 seconds
			perbytes = 0
		}

		dat, er = BlockDatabase.FetchNextBlock()
		if dat==nil || er!=nil {
			println("END of DB file")
			break
		}

		bl, er = btc.NewBlock(dat[:])
		if er != nil {
			println("Block inconsistent:", er.Error())
			break
		}

		bl.Trusted = trust

		er, _, _ = chain.CheckBlock(bl)

		if er != nil {
			if er.Error()!="Genesis" {
				println("CheckBlock failed:", er.Error())
				//os.Exit(1) // Such a thing should not happen, so let's better abort here.
			}
			continue
		}

		er = chain.AcceptBlock(bl)
		if er != nil {
			println("AcceptBlock failed:", er.Error())
			//os.Exit(1) // Such a thing should not happen, so let's better abort here.
		}

		totbytes += uint64(len(bl.Raw))
		perbytes += uint64(len(bl.Raw))
	}

	stop := time.Now().UnixNano()
	stat(stop-start, stop-prv, totbytes, perbytes, chain.BlockTreeEnd.Height)

	fmt.Println("Satoshi's database import finished in", (stop-start)/1e9, "seconds")

	fmt.Println("Now saving the new database...")
	chain.Sync()
	chain.Save()
	chain.Close()
	fmt.Println("Database saved. No more imports should be needed.")
	fmt.Println("It is advised to close and restart the node now, to free some mem.")
}
