package main

import (
	"fmt"
	"github.com/piotrnar/gocoin"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/rpcapi"
	"github.com/piotrnar/gocoin/client/usif"
	"github.com/piotrnar/gocoin/client/usif/textui"
	"github.com/piotrnar/gocoin/client/usif/webui"
	"github.com/piotrnar/gocoin/client/wallet"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/lib/others/peersdb"
	"github.com/piotrnar/gocoin/lib/others/qdb"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"io/ioutil"
	"os"
	"os/signal"
	"runtime"
	"runtime/debug"
	"time"
	"unsafe"
)

var (
	retryCachedBlocks bool
	SaveBlockChain    *time.Timer = time.NewTimer(24 * time.Hour)
)

const (
	SaveBlockChainAfter = 2 * time.Second
)

func reset_save_timer() {
	SaveBlockChain.Stop()
	for len(SaveBlockChain.C) > 0 {
		<-SaveBlockChain.C
	}
	SaveBlockChain.Reset(SaveBlockChainAfter)
}

func blockMined(bl *btc.Block) {
	network.BlockMined(bl)
	if int(bl.LastKnownHeight)-int(bl.Height) < 144 { // do not run it when syncing chain
		usif.ProcessBlockFees(bl.Height, bl)
	}
}

func LocalAcceptBlock(newbl *network.BlockRcvd) (e error) {
	bl := newbl.Block
	if common.FLAG.TrustAll {
		bl.Trusted = true
	}

	common.BlockChain.Unspent.AbortWriting() // abort saving of UTXO.db
	common.BlockChain.Blocks.BlockAdd(newbl.BlockTreeNode.Height, bl)
	newbl.TmQueue = time.Now()

	if newbl.DoInvs {
		common.Busy("NetRouteInv")
		network.NetRouteInv(2, bl.Hash, newbl.Conn)
	}

	network.MutexRcv.Lock()
	bl.LastKnownHeight = network.LastCommitedHeader.Height
	network.MutexRcv.Unlock()
	e = common.BlockChain.CommitBlock(bl, newbl.BlockTreeNode)

	if e == nil {
		// new block accepted
		newbl.TmAccepted = time.Now()

		newbl.NonWitnessSize = bl.NoWitnessSize

		common.RecalcAverageBlockSize()

		common.Last.Mutex.Lock()
		common.Last.Time = time.Now()
		common.Last.Block = common.BlockChain.LastBlock()
		common.Last.Mutex.Unlock()

		reset_save_timer()
	} else {
		//fmt.Println("Warning: AcceptBlock failed. If the block was valid, you may need to rebuild the unspent DB (-r)")
		new_end := common.BlockChain.LastBlock()
		common.Last.Mutex.Lock()
		common.Last.Block = new_end
		common.Last.Mutex.Unlock()
		// update network.LastCommitedHeader
		network.MutexRcv.Lock()
		if network.LastCommitedHeader != new_end {
			network.LastCommitedHeader = new_end
			//println("LastCommitedHeader moved to", network.LastCommitedHeader.Height)
		}
		network.DiscardedBlocks[newbl.Hash.BIdx()] = true
		network.MutexRcv.Unlock()
	}
	return
}

func retry_cached_blocks() bool {
	var idx int
	common.CountSafe("RedoCachedBlks")
	for idx < len(network.CachedBlocks) {
		newbl := network.CachedBlocks[idx]
		if CheckParentDiscarded(newbl.BlockTreeNode) {
			common.CountSafe("DiscardCachedBlock")
			network.CachedBlocks = append(network.CachedBlocks[:idx], network.CachedBlocks[idx+1:]...)
			network.CachedBlocksLen.Store(len(network.CachedBlocks))
			return len(network.CachedBlocks) > 0
		}
		if common.BlockChain.HasAllParents(newbl.BlockTreeNode) {
			common.Busy("Cache.LocalAcceptBlock " + newbl.Block.Hash.String())
			e := LocalAcceptBlock(newbl)
			if e != nil {
				fmt.Println("AcceptBlock2", newbl.Block.Hash.String(), "-", e.Error())
				newbl.Conn.Misbehave("LocalAcceptBl2", 250)
			}
			if usif.Exit_now.Get() {
				return false
			}
			// remove it from cache
			network.CachedBlocks = append(network.CachedBlocks[:idx], network.CachedBlocks[idx+1:]...)
			network.CachedBlocksLen.Store(len(network.CachedBlocks))
			return len(network.CachedBlocks) > 0
		} else {
			idx++
		}
	}
	return false
}

// Return true iof the block's parent is on the DiscardedBlocks list
// Add it to DiscardedBlocks, if returning true
func CheckParentDiscarded(n *chain.BlockTreeNode) bool {
	network.MutexRcv.Lock()
	defer network.MutexRcv.Unlock()
	if network.DiscardedBlocks[n.Parent.BlockHash.BIdx()] {
		network.DiscardedBlocks[n.BlockHash.BIdx()] = true
		return true
	}
	return false
}

// Called from the blockchain thread
func HandleNetBlock(newbl *network.BlockRcvd) {
	var tmpfn string

	if newbl.Block == nil {
		tmpfn = common.TempBlocksDir() + newbl.BlockTreeNode.BlockHash.String()
	}
	defer func() {
		if tmpfn != "" {
			os.Remove(tmpfn)
		}
	}()

	if CheckParentDiscarded(newbl.BlockTreeNode) {
		common.CountSafe("DiscardFreshBlockA")
		retryCachedBlocks = len(network.CachedBlocks) > 0
		return
	}

	if !common.BlockChain.HasAllParents(newbl.BlockTreeNode) {
		// it's not linking - keep it for later
		network.CachedBlocks = append(network.CachedBlocks, newbl)
		network.CachedBlocksLen.Store(len(network.CachedBlocks))
		common.CountSafe("BlockPostone")
		return
	}

	if newbl.Block == nil {
		dat, e := ioutil.ReadFile(tmpfn)
		if e != nil {
			panic(e.Error())
		}
		if newbl.Block, e = btc.NewBlock(dat); e != nil {
			panic(e.Error())
		}
		if e = newbl.Block.BuildTxList(); e != nil {
			panic(e.Error())
		}
	}

	common.Busy("LocalAcceptBlock " + newbl.Hash.String())
	if e := LocalAcceptBlock(newbl); e != nil {
		common.CountSafe("DiscardFreshBlockB")
		fmt.Println("AcceptBlock1", newbl.Block.Hash.String(), "-", e.Error())
		newbl.Conn.Misbehave("LocalAcceptBl1", 250)
	} else {
		//println("block", newbl.Block.Height, "accepted")
		retryCachedBlocks = retry_cached_blocks()
	}
}

func HandleRpcBlock(msg *rpcapi.BlockSubmited) {
	network.MutexRcv.Lock()
	rb := network.ReceivedBlocks[msg.Block.Hash.BIdx()]
	network.MutexRcv.Unlock()
	if rb == nil {
		panic("Block " + msg.Block.Hash.String() + " not in ReceivedBlocks map")
	}

	common.BlockChain.Unspent.AbortWriting()
	rb.TmQueue = time.Now()

	e, _, _ := common.BlockChain.CheckBlock(msg.Block)
	if e == nil {
		e = common.BlockChain.AcceptBlock(msg.Block)
		rb.TmAccepted = time.Now()
	}
	if e != nil {
		common.CountSafe("RPCBlockError")
		msg.Error = e.Error()
		msg.Done.Done()
		return
	}

	common.RecalcAverageBlockSize()

	common.CountSafe("RPCBlockOK")
	println("New mined block", msg.Block.Height, "accepted OK in", rb.TmAccepted.Sub(rb.TmQueue).String())

	common.Last.Mutex.Lock()
	common.Last.Time = time.Now()
	common.Last.Block = common.BlockChain.LastBlock()
	common.Last.Mutex.Unlock()

	msg.Done.Done()
}

func main() {
	var ptr *byte
	if unsafe.Sizeof(ptr) < 8 {
		fmt.Println("WARNING: Gocoin client shall be build for 64-bit arch. It will likely crash now.")
	}

	fmt.Println("Gocoin client version", gocoin.Version)
	runtime.GOMAXPROCS(runtime.NumCPU()) // It seems that Go does not do it by default

	// Disable Ctrl+C
	signal.Notify(common.KillChan, os.Interrupt, os.Kill)
	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(error)
			if !ok {
				err = fmt.Errorf("pkg: %v", r)
			}
			fmt.Println("main panic recovered:", err.Error())
			fmt.Println(string(debug.Stack()))
			network.NetCloseAll()
			common.CloseBlockChain()
			peersdb.ClosePeerDB()
			sys.UnlockDatabaseDir()
			os.Exit(1)
		}
	}()

	common.InitConfig()

	if common.FLAG.VolatileUTXO {
		fmt.Println("WARNING! Using UTXO database in a volatile mode. Make sure to close the client properly (do not kill it!)")
	}

	if common.FLAG.TrustAll {
		fmt.Println("WARNING! Assuming all scripts inside new blocks to PASS. Verify the last block's hash when finished.")
	}

	host_init() // This will create the DB lock file and keep it open

	os.RemoveAll(common.TempBlocksDir())

	if common.FLAG.UndoBlocks > 0 {
		usif.Exit_now.Set()
	}

	if common.FLAG.Rescan && common.FLAG.VolatileUTXO {

		fmt.Println("UTXO database rebuilt complete in the volatile mode, so flush DB to disk and exit...")

	} else if !usif.Exit_now.Get() {

		common.RecalcAverageBlockSize()

		peersTick := time.Tick(5 * time.Minute)
		netTick := time.Tick(time.Second)

		peersdb.Testnet = common.Testnet
		peersdb.ConnectOnly = common.CFG.ConnectOnly
		peersdb.Services = common.Services
		peersdb.InitPeers(common.GocoinHomeDir)
		if common.FLAG.UnbanAllPeers {
			var keys []qdb.KeyType
			var vals [][]byte
			peersdb.PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
				peer := peersdb.NewPeer(v)
				if peer.Banned != 0 {
					fmt.Println("Unban", peer.NetAddr.String())
					peer.Banned = 0
					keys = append(keys, k)
					vals = append(vals, peer.Bytes())
				}
				return 0
			})
			for i := range keys {
				peersdb.PeerDB.Put(keys[i], vals[i])
			}

			fmt.Println(len(keys), "peers un-baned")
		}

		for k, v := range common.BlockChain.BlockIndex {
			network.ReceivedBlocks[k] = &network.OneReceivedBlock{TmStart: time.Unix(int64(v.Timestamp()), 0)}
		}
		network.LastCommitedHeader = common.Last.Block

		if common.CFG.TXPool.SaveOnDisk {
			network.MempoolLoad2()
		}

		if common.CFG.TextUI_Enabled {
			go textui.MainThread()
		}

		if common.CFG.WebUI.Interface != "" {
			fmt.Println("Starting WebUI at", common.CFG.WebUI.Interface)
			go webui.ServerThread(common.CFG.WebUI.Interface)
		}

		if common.CFG.RPC.Enabled {
			go rpcapi.StartServer(common.RPCPort())
		}

		usif.LoadBlockFees()

		wallet.FetchingBalanceTick = func() {
			select {
			case rec := <-usif.LocksChan:
				common.CountSafe("DoMainLocks")
				rec.In.Done()
				rec.Out.Wait()

			case newtx := <-network.NetTxs:
				common.CountSafe("DoMainNetTx")
				network.HandleNetTx(newtx, false)

			case <-netTick:
				common.CountSafe("DoMainNetTick")
				network.NetworkTick()

			default:
			}
		}

		for !usif.Exit_now.Get() {
			common.CountSafe("MainThreadLoops")
			for retryCachedBlocks {
				retryCachedBlocks = retry_cached_blocks()
				// We have done one per loop - now do something else if pending...
				if len(network.NetBlocks) > 0 || len(usif.UiChannel) > 0 {
					break
				}
			}

			common.Busy("")

			select {
			case s := <-common.KillChan:
				fmt.Println("Got signal:", s)
				usif.Exit_now.Set()
				continue

			case rpcbl := <-rpcapi.RpcBlocks:
				common.CountSafe("RPCNewBlock")
				common.Busy("HandleRpcBlock()")
				HandleRpcBlock(rpcbl)

			case rec := <-usif.LocksChan:
				common.CountSafe("MainLocks")
				common.Busy("LockedByRequest")
				rec.In.Done()
				rec.Out.Wait()
				continue

			case newbl := <-network.NetBlocks:
				common.CountSafe("MainNetBlock")
				common.Busy("HandleNetBlock()")
				HandleNetBlock(newbl)

			case <-SaveBlockChain.C:
				common.CountSafe("SaveBlockChain")
				common.Busy("BlockChain.Idle()")
				if common.BlockChain.Idle() {
					common.CountSafe("ChainIdleUsed")
				}

			case newtx := <-network.NetTxs:
				common.CountSafe("MainNetTx")
				common.Busy("network.HandleNetTx()")
				network.HandleNetTx(newtx, false)

			case <-netTick:
				common.CountSafe("MainNetTick")
				common.Busy("network.NetworkTick()")
				network.NetworkTick()

			case cmd := <-usif.UiChannel:
				common.CountSafe("MainUICmd")
				common.Busy("UI command")
				cmd.Handler(cmd.Param)
				cmd.Done.Done()
				continue

			case <-peersTick:
				common.Busy("peersdb.ExpirePeers()")
				peersdb.ExpirePeers()
				usif.ExpireBlockFees()

			case <-time.After(time.Second):
				common.CountSafe("MainThreadIdle")
				continue
			}
		}

		common.BlockChain.Unspent.HurryUp.Set()
		network.NetCloseAll()
	}

	sta := time.Now()
	common.CloseBlockChain()
	if common.FLAG.UndoBlocks == 0 {
		network.MempoolSave(false)
		wallet.Save()
	}
	fmt.Println("Blockchain closed in", time.Now().Sub(sta).String())
	peersdb.ClosePeerDB()
	usif.SaveBlockFees()
	sys.UnlockDatabaseDir()
	os.RemoveAll(common.TempBlocksDir())
}
