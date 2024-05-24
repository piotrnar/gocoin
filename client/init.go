package main

import (
	"crypto/rand"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/lib/others/sys"
)

func host_init() {
	common.GocoinHomeDir = common.CFG.Datadir + string(os.PathSeparator)

	common.Testnet = common.CFG.Testnet // So chaging this value would will only affect the behaviour after restart
	if common.CFG.Testnet {             // testnet4
		common.GenesisBlock = btc.NewUint256FromString("00000000da84f2bafbbc53dee25a72ae507ff4914b867c565be350b0da8bf043")
		common.Magic = [4]byte{0x1c, 0x16, 0x3f, 0x28}
		common.GocoinHomeDir += common.DataSubdir() + string(os.PathSeparator)
	} else {
		common.GenesisBlock = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
		common.Magic = [4]byte{0xF9, 0xBE, 0xB4, 0xD9}
		common.GocoinHomeDir += common.DataSubdir() + string(os.PathSeparator)
	}

	// Lock the folder
	os.MkdirAll(common.GocoinHomeDir, 0770)
	sys.LockDatabaseDir(common.GocoinHomeDir)

	common.SecretKey, _ = ioutil.ReadFile(common.GocoinHomeDir + "authkey")
	if len(common.SecretKey) != 32 {
		common.SecretKey = make([]byte, 32)
		rand.Read(common.SecretKey)
		ioutil.WriteFile(common.GocoinHomeDir+"authkey", common.SecretKey, 0600)
	}
	common.PublicKey = btc.Encodeb58(btc.PublicFromPrivate(common.SecretKey, true))
	fmt.Println("Public auth key:", "@"+common.PublicKey)

	__exit := make(chan bool)
	__done := make(chan bool)
	go func() {
		for {
			select {
			case s := <-common.KillChan:
				fmt.Println(s)
				chain.AbortNow = true
			case <-__exit:
				__done <- true
				return
			}
		}
	}()

	if chain.AbortNow {
		sys.UnlockDatabaseDir()
		os.Exit(1)
	}

	fmt.Print(string(common.LogBuffer.Bytes()))
	common.LogBuffer = nil

	if btc.EC_Verify == nil {
		fmt.Println("Using native secp256k1 lib for EC_Verify (consider installing a speedup)")
	}

	ext := &chain.NewChanOpts{
		UTXOVolatileMode: common.FLAG.VolatileUTXO,
		UndoBlocks:       common.FLAG.UndoBlocks,
		BlockMinedCB:     blockMined, BlockUndoneCB: blockUndone,
		DoNotRescan: true, CompressUTXO: common.CFG.UTXOSave.CompressRecords}

	if ext.UndoBlocks > 0 {
		ext.BlockUndoneCB = nil // Do not call the callback if undoing blocks as it will panic
	}

	sta := time.Now()
	common.BlockChain = chain.NewChainExt(common.GocoinHomeDir, common.GenesisBlock, common.FLAG.Rescan, ext,
		&chain.BlockDBOpts{
			MaxCachedBlocks: int(common.CFG.Memory.MaxCachedBlks),
			MaxDataFileSize: uint64(common.CFG.Memory.MaxDataFileMB) << 20,
			DataFilesKeep:   common.CFG.Memory.DataFilesKeep,
			DataFilesBackup: common.CFG.Memory.OldDataBackup,
			CompressOnDisk:  common.CFG.Memory.CompressBlockDB})
	if chain.AbortNow {
		fmt.Printf("Blockchain opening aborted after %s seconds\n", time.Now().Sub(sta).String())
		common.BlockChain.Close()
		sys.UnlockDatabaseDir()
		os.Exit(1)
	}

	if lb, _ := common.BlockChain.BlockTreeRoot.FindFarthestNode(); lb.Height > common.BlockChain.LastBlock().Height {
		common.Last.ParseTill = lb
	}

	common.Last.Block = common.BlockChain.LastBlock()
	common.Last.Time = time.Unix(int64(common.Last.Block.Timestamp()), 0)
	if common.Last.Time.After(time.Now()) {
		common.Last.Time = time.Now()
	}
	common.UpdateScriptFlags(0)

	common.LockCfg()
	common.ApplyLastTrustedBlock()
	common.UnlockCfg()

	if common.CFG.Memory.FreeAtStart {
		fmt.Print("Freeing memory... ")
		sys.FreeMem()
		fmt.Print("\r                  \r")
	}
	sto := time.Now()

	al, sy := sys.MemUsed()
	by, _ := common.MemUsed()
	fmt.Printf("Blockchain open in %s.  %d + %d MB of RAM used (%d)\n",
		sto.Sub(sta).String(), al>>20, by>>20, sy>>20)
	fmt.Println("Highest known block is", common.Last.Block.Height, "from",
		common.Last.Time.Format("2006-01-02 15:04:05"))

	common.StartTime = time.Now()
	__exit <- true
	_ = <-__done

}
