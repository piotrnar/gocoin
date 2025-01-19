package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"runtime/debug"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/piotrnar/gocoin"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/network/peersdb"
	"github.com/piotrnar/gocoin/client/rpcapi"
	"github.com/piotrnar/gocoin/client/usif"
	"github.com/piotrnar/gocoin/client/usif/textui"
	"github.com/piotrnar/gocoin/client/usif/webui"
	"github.com/piotrnar/gocoin/client/wallet"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/lib/others/qdb"
	"github.com/piotrnar/gocoin/lib/others/sys"
)

var (
	retryCachedBlocks bool
	SaveBlockChain    *time.Timer = time.NewTimer(24 * time.Hour)

	NetBlocksSize sys.SyncInt

	exitat *uint = flag.Uint("exitat", 0, "Auto exit node after comitting block with the given height")
)

const (
	SaveBlockChainAfter       = 2 * time.Second
	SaveBlockChainAfterNoSync = 10 * time.Minute
)

func reset_save_timer() {
	SaveBlockChain.Stop()
	for len(SaveBlockChain.C) > 0 {
		<-SaveBlockChain.C
	}
	if common.BlockChainSynchronized {
		SaveBlockChain.Reset(SaveBlockChainAfter)
	} else {
		SaveBlockChain.Reset(SaveBlockChainAfterNoSync)
	}
}

func blockMined(bl *btc.Block) {
	network.BlockMined(bl)
	if int(bl.LastKnownHeight)-int(bl.Height) < 144 { // do not run it when syncing chain
		usif.ProcessBlockFees(bl.Height, bl)
	}
}

func blockUndone(bl *btc.Block) {
	network.BlockUndone(bl)
}

func exit_now() {
	al, sy := sys.MemUsed()
	cb, _ := common.MemUsed()
	fmt.Printf("Sync to %d took %s  -  %.0f min.  Mem: %d %d %d MB  - cachempty: %d\n",
		common.Last.Block.Height, time.Since(common.StartTime).String(),
		float64(time.Since(common.StartTime))/float64(time.Minute), al>>20, sy>>20, cb>>20,
		network.Fetch.CacheEmpty)
	fmt.Printf("Wasted %dMB from %d blocks.  Max cache used: %d / %dMB\n",
		network.Fetch.BlockBytesWasted>>20, network.Fetch.BlockSameRcvd,
		network.MaxCachedBlocksSize.Get()>>20, common.SyncMaxCacheBytes.Get()>>20)
	common.PrintBWStats()
	fmt.Print("Reached given block ", *exitat, ". Now exiting....\n\n\n\n")
	os.Exit(0)
}

func LocalAcceptBlock(newbl *network.BlockRcvd) (e error) {
	bl := newbl.Block
	if common.FLAG.TrustAll || newbl.BlockTreeNode.Trusted.Get() {
		bl.Trusted.Set()
	}

	common.BlockChain.Unspent.AbortWriting() // abort saving of UTXO.db
	common.BlockChain.Blocks.BlockAdd(newbl.BlockTreeNode.Height, bl)
	newbl.TmQueue = time.Now()

	if newbl.DoInvs {
		common.Busy()
		network.NetRouteInv(network.MSG_BLOCK, bl.Hash, newbl.Conn)
	}

	network.MutexRcv.Lock()
	bl.LastKnownHeight = network.LastCommitedHeader.Height
	network.MutexRcv.Unlock()
	e = common.BlockChain.CommitBlock(bl, newbl.BlockTreeNode)
	if bl.LastKnownHeight-bl.Height > common.GetUint32(&common.CFG.Memory.MaxCachedBlks) {
		bl.Txs = nil // we won't be needing bl.Txs anymore, so might as well mark the memory as unused
	}

	if e == nil {
		// new block accepted
		newbl.TmAccepted = time.Now()

		newbl.NonWitnessSize = bl.NoWitnessSize
		newbl.TheWeight = bl.BlockWeight
		newbl.ThePaidVSize = bl.PaidTxsVSize
		newbl.TheOrdCnt = bl.OrbTxCnt
		newbl.TheOrdSize = bl.OrbTxSize
		newbl.TheOrdWeight = bl.OrbTxWeight

		common.RecalcAverageBlockSize()

		common.Last.Mutex.Lock()
		common.Last.Time = time.Now()
		common.Last.Block = common.BlockChain.LastBlock()
		common.UpdateScriptFlags(bl.VerifyFlags)

		if common.Last.ParseTill != nil && (common.Last.Block.Height%100e3) == 0 {
			fmt.Println("Parsing to", common.Last.Block.Height, "took", time.Since(newbl.TmStart).String())
		}

		if common.Last.ParseTill != nil && common.Last.Block == common.Last.ParseTill {
			fmt.Println("Initial parsing finished in", time.Since(newbl.TmStart).String())
			common.Last.ParseTill = nil
		}
		if common.Last.ParseTill == nil && !common.BlockChainSynchronized &&
			((common.Last.Block.Height%50e3) == 0 || common.Last.Block.Height == network.LastCommitedHeader.Height) {
			al, sy := sys.MemUsed()
			cb, _ := common.MemUsed()
			fmt.Printf("Sync to %d took %s  -  %.1f min.  Mem: %d %d %d MB  - cachempty: %d\n",
				common.Last.Block.Height, time.Since(common.StartTime).String(),
				float64(time.Since(common.StartTime))/float64(time.Minute), al>>20, sy>>20, cb>>20,
				network.Fetch.CacheEmpty)
			if common.Last.Block.Height <= 200e3 {
				// Cache underflow counter is not reliable at the beginning of chain sync,s o reset it here
				network.Fetch.CacheEmpty = 0
			}
		}
		if *exitat != 0 && uint(common.Last.Block.Height) == *exitat {
			exit_now()
		}
		common.Last.Mutex.Unlock()
	} else {
		//fmt.Println("Warning: AcceptBlock failed. If the block was valid, you may need to rebuild the unspent DB (-r)")
		new_end := common.BlockChain.LastBlock()
		common.Last.Mutex.Lock()
		common.Last.Block = new_end
		common.UpdateScriptFlags(bl.VerifyFlags)
		common.Last.Mutex.Unlock()
		// update network.LastCommitedHeader
		network.MutexRcv.Lock()
		prev_last_header := network.LastCommitedHeader
		network.DiscardBlock(newbl.BlockTreeNode) // this function can also modify network.LastCommitedHeader
		if common.Last.Block.Height > network.LastCommitedHeader.Height {
			network.LastCommitedHeader, _ = common.Last.Block.FindFarthestNode()
		}
		need_more_headers := prev_last_header != network.LastCommitedHeader
		network.MutexRcv.Unlock()
		if need_more_headers {
			//fmt.Println("LastCommitedHeader moved to", network.LastCommitedHeader.Height)
			network.GetMoreHeaders()
		}
	}
	reset_save_timer()
	return
}

func retry_cached_blocks() bool {
	var idx int
	common.CountSafe("RedoCachedBlks")
	for idx < len(network.CachedBlocks) {
		newbl := network.CachedBlocks[idx]
		if CheckParentDiscarded(newbl.BlockTreeNode) {
			common.CountSafe("DiscardCachedBlock")
			if newbl.Block == nil {
				os.Remove(common.TempBlocksDir() + newbl.BlockTreeNode.BlockHash.String())
			}
			network.CachedBlocksDel(idx)
			return len(network.CachedBlocks) > 0
		}
		if common.BlockChain.HasAllParents(newbl.BlockTreeNode) {
			common.Busy()

			if newbl.Block == nil {
				tmpfn := common.TempBlocksDir() + newbl.BlockTreeNode.BlockHash.String()
				dat, e := ioutil.ReadFile(tmpfn)
				os.Remove(tmpfn)
				if e != nil {
					panic(e.Error())
				}
				if newbl.Block, e = btc.NewBlock(dat); e != nil {
					panic(e.Error())
				}
				if e = newbl.Block.BuildTxList(); e != nil {
					panic(e.Error())
				}
				newbl.Block.BlockExtraInfo = *newbl.BlockExtraInfo
			}

			e := LocalAcceptBlock(newbl)
			if e != nil {
				fmt.Println("AcceptBlock2", newbl.BlockTreeNode.BlockHash.String(), "-", e.Error())
				newbl.Conn.Misbehave("LocalAcceptBl2", 250)
			}
			if usif.Exit_now.Get() {
				return false
			}
			// remove it from cache
			network.CachedBlocksDel(idx)
			return len(network.CachedBlocks) > 0
		} else {
			idx++
		}
	}
	return false
}

// CheckParentDiscarded returns true if the block's parent is on the DiscardedBlocks list.
// Add it to DiscardedBlocks, if returning true.
func CheckParentDiscarded(n *chain.BlockTreeNode) bool {
	network.MutexRcv.Lock()
	defer network.MutexRcv.Unlock()
	if network.DiscardedBlocks[n.Parent.BlockHash.BIdx()] {
		network.DiscardedBlocks[n.BlockHash.BIdx()] = true
		return true
	}
	return false
}

// HandleNetBlock is called from the blockchain thread.
func HandleNetBlock(newbl *network.BlockRcvd) {
	if common.Last.ParseTill != nil {
		NetBlocksSize.Add(-len(newbl.Block.Raw))
	}

	defer func() {
		common.CountSafe("MainNetBlock")
		if common.GetUint32(&common.WalletOnIn) > 0 {
			common.SetUint32(&common.WalletOnIn, 5) // snooze the timer to 5 seconds from now
		}
	}()

	if CheckParentDiscarded(newbl.BlockTreeNode) {
		common.CountSafe("DiscardFreshBlockA")
		if newbl.Block == nil {
			os.Remove(common.TempBlocksDir() + newbl.BlockTreeNode.BlockHash.String())
		}
		retryCachedBlocks = len(network.CachedBlocks) > 0
		return
	}

	if !common.BlockChain.HasAllParents(newbl.BlockTreeNode) {
		// it's not linking - keep it for later
		network.CachedBlocksAdd(newbl)
		common.CountSafe("BlockPostone")
		return
	}

	if newbl.Block == nil {
		tmpfn := common.TempBlocksDir() + newbl.BlockTreeNode.BlockHash.String()
		dat, e := ioutil.ReadFile(tmpfn)
		os.Remove(tmpfn)
		if e != nil {
			panic(e.Error())
		}
		if newbl.Block, e = btc.NewBlock(dat); e != nil {
			panic(e.Error())
		}
		if e = newbl.Block.BuildTxList(); e != nil {
			panic(e.Error())
		}
		newbl.Block.BlockExtraInfo = *newbl.BlockExtraInfo
	}

	common.Busy()
	if e := LocalAcceptBlock(newbl); e != nil {
		common.CountSafe("DiscardFreshBlockB")
		fmt.Println("AcceptBlock1", newbl.Block.Hash.String(), "-", e.Error())
		newbl.Conn.Misbehave("LocalAcceptBl1", 250)
	}
	retryCachedBlocks = retry_cached_blocks()
	if !retryCachedBlocks && network.BlocksToGetCnt() != 0 {
		now := time.Now()
		if network.Fetch.LastCacheEmpty.IsZero() || now.Sub(network.Fetch.LastCacheEmpty) >= time.Second {
			network.Fetch.CacheEmpty++
			network.Fetch.LastCacheEmpty = now
		}
	}
}

func HandleRpcBlock(msg *rpcapi.BlockSubmited) {
	common.CountSafe("RPCNewBlock")

	network.MutexRcv.Lock()
	rb := network.ReceivedBlocks[msg.Block.Hash.BIdx()]
	network.MutexRcv.Unlock()
	if rb == nil {
		panic("Block " + msg.Block.Hash.String() + " not in ReceivedBlocks map")
	}

	common.BlockChain.Unspent.AbortWriting()
	rb.TmQueue = time.Now()

	_, _, e := common.BlockChain.CheckBlock(msg.Block)
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

	network.NetRouteInv(network.MSG_BLOCK, msg.Block.Hash, nil)
	common.RecalcAverageBlockSize()

	common.CountSafe("RPCBlockOK")
	fmt.Println("New mined block", msg.Block.Height, "accepted OK in", rb.TmAccepted.Sub(rb.TmQueue).String())

	common.Last.Mutex.Lock()
	common.Last.Time = time.Now()
	common.Last.Block = common.BlockChain.LastBlock()
	common.UpdateScriptFlags(msg.VerifyFlags)
	last_block := common.Last.Block
	common.Last.Mutex.Unlock()

	network.MutexRcv.Lock()
	if last_block.Height > network.LastCommitedHeader.Height {
		network.LastCommitedHeader = last_block
	}
	network.MutexRcv.Unlock()

	msg.Done.Done()
}

func do_the_blocks(end *chain.BlockTreeNode) {
	sta := time.Now()
	last := common.BlockChain.LastBlock()
	for last != end {
		nxt := last.FindPathTo(end)
		if nxt == nil {
			break
		}

		if nxt.BlockSize == 0 {
			fmt.Println("BlockSize is zero - corrupt database")
			break
		}

		pre := time.Now()
		crec, trusted, _ := common.BlockChain.Blocks.BlockGetInternal(nxt.BlockHash, true)
		if crec == nil || crec.Data == nil {
			panic(fmt.Sprint("No data for block #", nxt.Height, " ", nxt.BlockHash.String()))
		}

		bl, er := btc.NewBlock(crec.Data)
		if er != nil {
			fmt.Println("btc.NewBlock() error - corrupt database")
			break
		}
		bl.Height = nxt.Height

		// Recover the flags to be used when verifying scripts for non-trusted blocks (stored orphaned blocks)
		common.BlockChain.ApplyBlockFlags(bl)

		er = bl.BuildTxList()
		if er != nil {
			fmt.Println("bl.BuildTxList() error - corrupt database")
			break
		}

		bl.Trusted.Store(trusted)

		tdl := time.Now()

		rb := &network.OneReceivedBlock{TmStart: sta, TmPreproc: pre, TmDownload: tdl}
		network.MutexRcv.Lock()
		network.ReceivedBlocks[bl.Hash.BIdx()] = rb
		network.MutexRcv.Unlock()

		network.NetBlocks <- &network.BlockRcvd{Conn: nil, Block: bl, BlockTreeNode: nxt,
			OneReceivedBlock: rb, BlockExtraInfo: nil}

		NetBlocksSize.Add(len(bl.Raw))
		for NetBlocksSize.Get() > 64*1024*1024 {
			time.Sleep(100 * time.Millisecond)
		}

		last = nxt

	}
	//fmt.Println("all blocks queued", len(network.NetBlocks))
}

func fetch_balances_now() {
	var prev_progress uint32
	var abort bool
	const info = "\rFetching all balances (Ctrl+C to skip) - "
	__exit := make(chan bool)
	var done sync.WaitGroup
	done.Add(1)
	go func() {
		for {
			select {
			case s := <-common.KillChan:
				fmt.Println(s)
				abort = true
			case <-__exit:
				done.Done()
				return
			}
		}
	}()
	wallet.FetchingBalanceTick = func() bool {
		if abort {
			return true
		}
		if new_progress := common.WalletProgress / 10; new_progress != prev_progress {
			prev_progress = new_progress
			fmt.Print(info, prev_progress, "% complete ... ")
		}
		return false
	}
	sta := time.Now()
	fmt.Print(info)
	wallet.LoadBalancesFromUtxo()
	fmt.Print("\r                                                                 \r")
	__exit <- true
	done.Wait()
	if !abort {
		fmt.Println("All balances fetched in", time.Since(sta))
		common.SetBool(&common.WalletON, true)
		common.SetUint32(&common.WalletOnIn, 0)
	}
}

func main() {
	var ptr *byte
	if unsafe.Sizeof(ptr) < 8 {
		fmt.Println("WARNING: Gocoin client shall be build for 64-bit arch. It will likely crash now.")
	}

	fmt.Println("Gocoin client version", gocoin.Version)

	// Disable Ctrl+C
	signal.Notify(common.KillChan, os.Interrupt, syscall.SIGTERM)
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

	if common.FLAG.SaveConfig {
		common.SaveConfig()
		fmt.Println("Configuration file saved")
		os.Exit(0)
	}

	if common.FLAG.VolatileUTXO {
		fmt.Println("WARNING! Using UTXO database in a volatile mode. Make sure to close the client properly (do not kill it!)")
	}

	if common.FLAG.TrustAll {
		fmt.Println("WARNING! Assuming all scripts inside new blocks to PASS. Verify the last block's hash when finished.")
	}

	host_init() // This will create the DB lock file and keep it open

	os.RemoveAll(common.TempBlocksDir())
	common.MkTempBlocksDir()

	if common.FLAG.UndoBlocks > 0 {
		usif.Exit_now.Set()
	}

	if common.FLAG.Rescan && common.FLAG.VolatileUTXO {

		fmt.Println("UTXO database rebuilt complete in the volatile mode, so flush DB to disk and exit...")

	} else if !usif.Exit_now.Get() {

		common.RecalcAverageBlockSize()

		peersTick := time.Tick(peersdb.ExpirePeersPeriod)
		netTick := time.Tick(time.Second)

		reset_save_timer() // we wil do one save try after loading, in case if ther was a rescan

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

		if common.Last.ParseTill != nil {
			network.LastCommitedHeader = common.Last.ParseTill
			fmt.Println("Hold on network for now as we have",
				common.Last.ParseTill.Height-common.Last.Block.Height, "new blocks on disk.")
			go do_the_blocks(common.Last.ParseTill)
		} else {
			network.LastCommitedHeader = common.Last.Block
		}

		if common.CFG.TXPool.SaveOnDisk {
			network.MempoolLoad()
		}

		usif.LoadBlockFees()

		if !common.FLAG.NoWallet {
			sta := time.Now()
			if er := wallet.LoadBalances(); er == nil {
				common.SetBool(&common.WalletON, true)
				common.SetUint32(&common.WalletOnIn, 0)
				fmt.Println("AllBalances loaded from", wallet.LAST_SAVED_FNAME, "in", time.Since(sta).String())
			} else {
				fmt.Println("wallet.LoadBalances:", er.Error())
				if common.CFG.AllBalances.InstantWallet {
					fetch_balances_now()
				}
			}
			if !common.GetBool(&common.WalletON) {
				// snooze the timer to 10 seconds after startup_ticks goes down
				common.SetUint32(&common.WalletOnIn, 10)
			}
		}

		wallet.FetchingBalanceTick = func() bool {
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
				if common.Last.ParseTill != nil {
					break
				}
				network.NetworkTick()

			case on := <-wallet.OnOff:
				if !on {
					return true
				}

			default:
			}
			return usif.Exit_now.Get()
		}

		if common.CFG.WebUI.Interface != "" {
			go webui.ServerThread()
		}

		if common.CFG.RPC.Enabled {
			go rpcapi.StartServer(common.RPCPort())
		}

		if common.CFG.TextUI_Enabled {
			go textui.MainThread()
		}

		startup_ticks := 5 // give 5 seconds for finding out missing blocks
		for !usif.Exit_now.Get() {
			common.Busy()

			common.CountSafe("MainThreadLoops")
			for retryCachedBlocks {
				retryCachedBlocks = retry_cached_blocks()
				if !retryCachedBlocks && network.BlocksToGetCnt() != 0 {
					now := time.Now()
					if network.Fetch.LastCacheEmpty.IsZero() || now.Sub(network.Fetch.LastCacheEmpty) >= time.Second {
						network.Fetch.CacheEmpty++
						network.Fetch.LastCacheEmpty = now
					}
				}
				// We have done one per loop - now do something else if pending...
				if len(network.NetBlocks) > 0 || len(usif.UiChannel) > 0 {
					break
				}
			}

			// first check for priority messages; kill signal or a new block
			select {
			case <-common.KillChan:
				common.Busy()
				usif.Exit_now.Set()
				continue

			case newbl := <-network.NetBlocks:
				common.Busy()
				HandleNetBlock(newbl)

			case rpcbl := <-rpcapi.RpcBlocks:
				common.Busy()
				HandleRpcBlock(rpcbl)

			default: // timeout immediatelly if no priority message
			}

			common.Busy()

			select {
			case <-common.KillChan:
				common.Busy()
				usif.Exit_now.Set()
				continue

			case newbl := <-network.NetBlocks:
				common.Busy()
				HandleNetBlock(newbl)

			case rpcbl := <-rpcapi.RpcBlocks:
				common.Busy()
				HandleRpcBlock(rpcbl)

			case rec := <-usif.LocksChan:
				common.Busy()
				common.CountSafe("MainLocks")
				rec.In.Done()
				rec.Out.Wait()

			case <-SaveBlockChain.C:
				common.Busy()
				common.CountSafe("SaveBlockChain")
				if common.BlockChain.Idle() {
					common.CountSafe("ChainIdleUsed")
				}

			case newtx := <-network.NetTxs:
				common.Busy()
				common.CountSafe("MainNetTx")
				network.HandleNetTx(newtx, false)

			case <-netTick:
				common.Busy()
				common.CountSafe("MainNetTick")
				if common.Last.ParseTill != nil {
					break
				}
				network.NetworkTick()

				if common.BlockChainSynchronized {
					if common.WalletPendingTick() {
						wallet.OnOff <- true
					}
					break // BlockChainSynchronized so never mind checking it
				}

				// Now check if the chain is synchronized...
				if (network.HeadersReceived.Get() > int(common.GetUint32(&common.CFG.Net.MaxOutCons)/2) ||
					peersdb.ConnectOnly != "" && network.HeadersReceived.Get() >= 1) &&
					network.BlocksToGetCnt() == 0 && len(network.NetBlocks) == 0 &&
					len(network.CachedBlocks) == 0 {
					// only when we have no pending blocks and rteceived header messages, startup_ticks can go down..
					if startup_ticks > 0 {
						startup_ticks--
						break
					}
					common.SetBool(&common.BlockChainSynchronized, true)
					if *exitat == 99999999 {
						exit_now()
					}
					reset_save_timer()
				} else {
					startup_ticks = 5 // snooze by 5 seconds each time we're in here
				}

			case cmd := <-usif.UiChannel:
				common.Busy()
				common.CountSafe("MainUICmd")
				cmd.Handler(cmd.Param)
				cmd.Done.Done()

			case <-peersTick:
				common.Busy()
				peersdb.ExpirePeers()
				usif.ExpireBlockFees()

			case on := <-wallet.OnOff:
				common.Busy()
				if on {
					if common.BlockChainSynchronized {
						usif.FetchingBalances.Set()
						wallet.LoadBalancesFromUtxo()
						usif.FetchingBalances.Clr()
					} else {
						fmt.Println("Cannot enable wallet functionality with blockchain sync in progress")
					}
				} else {
					wallet.Disable()
					common.SetUint32(&common.WalletOnIn, 0)
				}
			}
		}

		common.BlockChain.Unspent.HurryUp()
		wallet.UpdateMapSizes()
		fmt.Println("Shutting down PID", os.Getpid())
		network.NetCloseAll()
	}

	sta := time.Now()
	common.CloseBlockChain()
	fmt.Println("Blockchain closed in", time.Since(sta).String())
	if common.FLAG.UndoBlocks == 0 {
		network.MempoolSave(false)
	}
	peersdb.ClosePeerDB()
	usif.SaveBlockFees()
	sta = time.Now()
	if er := wallet.SaveBalances(); er != nil {
		fmt.Println("SaveBalances:", er.Error())
	} else {
		fmt.Println(wallet.LAST_SAVED_FNAME, "saved in", time.Since(sta).String())
	}
	sys.UnlockDatabaseDir()
	os.RemoveAll(common.TempBlocksDir())
}
