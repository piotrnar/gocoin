package main

import (
	"os"
	"fmt"
	"time"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/lib/qdb"
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

	ext := &chain.NewChanOpts{
		UTXOVolatileMode : common.FLAG.VolatileUTXO,
		UndoBlocks : common.FLAG.UndoBlocks,
		SetBlocksDBCacheSize:true, BlocksDBCacheSize:int(common.CFG.Memory.MaxCachedBlocks)}

	if !common.FLAG.NoWallet {
		ext.NotifyTxAdd = wallet.TxNotifyAdd
		ext.NotifyTxDel = wallet.TxNotifyDel
		ext.LoadWalk = wallet.NewUTXO
		fmt.Println("Loading UTXO-db and P2SH/P2KH outputs of", btc.UintToBtc(common.AllBalMinVal), "BTC or more")
	}

	sta := time.Now().UnixNano()
	common.BlockChain = chain.NewChainExt(common.GocoinHomeDir, common.GenesisBlock, common.FLAG.Rescan, ext)
	sto := time.Now().UnixNano()
	if chain.AbortNow {
		fmt.Printf("Blockchain opening aborted after %.3f seconds\n", float64(sto-sta)/1e9)
		common.BlockChain.Close()
		sys.UnlockDatabaseDir()
		os.Exit(1)
	}
	al, sy := sys.MemUsed()
	fmt.Printf("Blockchain open in %.3f seconds.  %d + %d MB of RAM used (%d)\n",
		float64(sto-sta)/1e9, al>>20, qdb.ExtraMemoryConsumed>>20, sy>>20)
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
