package main

import (
	"os"
	"fmt"
	"time"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/utxo"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/wallet"
	"github.com/piotrnar/gocoin/lib/others/sys"
)


func host_init() {
	BtcRootDir := sys.BitcoinHome()
	common.GocoinHomeDir = common.CFG.Datadir+string(os.PathSeparator)

	common.Testnet = common.CFG.Testnet // So chaging this value would will only affect the behaviour after restart
	if common.CFG.Testnet { // testnet3
		common.GenesisBlock = btc.NewUint256FromString("000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943")
		common.Magic = [4]byte{0x0B,0x11,0x09,0x07}
		common.GocoinHomeDir += common.DataSubdir() + string(os.PathSeparator)
		BtcRootDir += "testnet3"+string(os.PathSeparator)
		common.MaxPeersNeeded = 2000
	} else {
		common.GenesisBlock = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
		common.Magic = [4]byte{0xF9,0xBE,0xB4,0xD9}
		common.GocoinHomeDir += common.DataSubdir() + string(os.PathSeparator)
		common.MaxPeersNeeded = 5000
	}

	// Lock the folder
	os.MkdirAll(common.GocoinHomeDir, 0770)
	sys.LockDatabaseDir(common.GocoinHomeDir)

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

	if check_if_convert_needed(common.GocoinHomeDir) {
		fmt.Println("You will no longer need the folder", common.GocoinHomeDir+"unspent4", "(delete it to recover space)")
		fmt.Println("Start the client again to continue.")
		sys.UnlockDatabaseDir()
		os.Exit(0)
	}
	if chain.AbortNow {
		sys.UnlockDatabaseDir()
		os.Exit(1)
	}

	ext := &chain.NewChanOpts{
		UTXOVolatileMode : common.FLAG.VolatileUTXO,
		UndoBlocks : common.FLAG.UndoBlocks,
		SetBlocksDBCacheSize:true, BlocksDBCacheSize:int(common.CFG.Memory.MaxCachedBlks)}

	sta := time.Now()
	common.BlockChain = chain.NewChainExt(common.GocoinHomeDir, common.GenesisBlock, common.FLAG.Rescan, ext)
	if chain.AbortNow {
		fmt.Printf("Blockchain opening aborted after %s seconds\n", time.Now().Sub(sta).String())
		common.BlockChain.Close()
		sys.UnlockDatabaseDir()
		os.Exit(1)
	}

	common.Last.Block = common.BlockChain.BlockTreeEnd
	common.Last.Time = time.Unix(int64(common.Last.Block.Timestamp()), 0)
	if common.Last.Time.After(time.Now()) {
		common.Last.Time = time.Now()
	}

	if common.CFG.Memory.FreeAtStart {
		fmt.Print("Freeing memory... ")
		sys.FreeMem()
		fmt.Print("\r                  \r")
	}
	sto := time.Now()

	al, sy := sys.MemUsed()
	fmt.Printf("Blockchain open in %s.  %d + %d MB of RAM used (%d)\n",
		sto.Sub(sta).String(), al>>20, utxo.ExtraMemoryConsumed>>20, sy>>20)

	if common.FLAG.UndoBlocks==0 && !common.FLAG.NoWallet {
		// Init Wallet
		common.BlockChain.Unspent.CB.NotifyTxAdd = wallet.TxNotifyAdd
		common.BlockChain.Unspent.CB.NotifyTxDel = wallet.TxNotifyDel
		// LoadWalk = wallet.NewUTXO
		sta = time.Now()
		wallet.FetchInitialBalance(&chain.AbortNow)
		if chain.AbortNow {
			fmt.Printf("Loading balances aborted after %s seconds\n", time.Now().Sub(sta).String())
			common.BlockChain.Close()
			sys.UnlockDatabaseDir()
			os.Exit(1)
		}
		if common.CFG.Memory.FreeAtStart {
			fmt.Print("Freeing memory... ")
			sys.FreeMem()
			fmt.Print("\r                  \r")
		}
		sto = time.Now()
		al, sy = sys.MemUsed()
		fmt.Printf("Balances loaded in %s seconds.  %d + %d MB of RAM used (%d)\n",
			sto.Sub(sta).String(), al>>20, utxo.ExtraMemoryConsumed>>20, sy>>20)
	}

	common.StartTime = time.Now()
	__exit <- true
	_ = <- __done

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
