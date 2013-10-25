package main

import (
	"os"
	"fmt"
	"time"
	"runtime"
	"os/signal"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/client/dbase"
	"github.com/piotrnar/gocoin/client/config"
	"github.com/piotrnar/gocoin/client/wallet"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/textui"
	"github.com/piotrnar/gocoin/client/webui"
)

const (
	defragEvery = (5*time.Minute)
)

var (
	killchan chan os.Signal = make(chan os.Signal)
	retryCachedBlocks bool
)

func addBlockToCache(bl *btc.Block, conn *network.OneConnection) {
	// we use network.CachedBlocks only from one therad so no need for a mutex
	if len(network.CachedBlocks)==config.MaxCachedBlocks {
		// Remove the oldest one
		oldest := time.Now()
		var todel [btc.Uint256IdxLen]byte
		for k, v := range network.CachedBlocks {
			if v.Time.Before(oldest) {
				oldest = v.Time
				todel = k
			}
		}
		delete(network.CachedBlocks, todel)
		config.CountSafe("CacheBlocksExpired")
	}
	network.CachedBlocks[bl.Hash.BIdx()] = network.OneCachedBlock{Time:time.Now(), Block:bl, Conn:conn}
}


func LocalAcceptBlock(bl *btc.Block, from *network.OneConnection) (e error) {
	sta := time.Now()
	e = config.BlockChain.AcceptBlock(bl)
	if e == nil {
		network.MutexRcv.Lock()
		network.ReceivedBlocks[bl.Hash.BIdx()].TmAccept = time.Now().Sub(sta)
		network.MutexRcv.Unlock()

		for i:=1; i<len(bl.Txs); i++ {
			network.TxMined(bl.Txs[i].Hash)
		}

		if int64(bl.BlockTime) > time.Now().Add(-10*time.Minute).Unix() {
			// Freshly mined block - do the inv and beeps...
			config.Busy("NetRouteInv")
			network.NetRouteInv(2, bl.Hash, from)

			if config.CFG.Beeps.NewBlock {
				fmt.Println("\007Received block", config.BlockChain.BlockTreeEnd.Height)
				textui.ShowPrompt()
			}

			if config.MinedByUs(bl.Raw) {
				fmt.Println("\007Mined by '"+config.CFG.Beeps.MinerID+"':", bl.Hash)
				textui.ShowPrompt()
			}

			if config.CFG.Beeps.ActiveFork && config.Last.Block == config.BlockChain.BlockTreeEnd {
				// Last block has not changed, so it must have been an orphaned block
				bln := config.BlockChain.BlockIndex[bl.Hash.BIdx()]
				commonNode := config.Last.Block.FirstCommonParent(bln)
				forkDepth := bln.Height - commonNode.Height
				fmt.Println("Orphaned block:", bln.Height, bl.Hash.String())
				if forkDepth > 1 {
					fmt.Println("\007\007\007WARNING: the fork is", forkDepth, "blocks deep")
				}
				textui.ShowPrompt()
			}

			if wallet.BalanceChanged && config.CFG.Beeps.NewBalance{
				fmt.Print("\007")
			}
		}

		config.Last.Mutex.Lock()
		config.Last.Time = time.Now()
		config.Last.Block = config.BlockChain.BlockTreeEnd
		config.Last.Mutex.Unlock()

		if wallet.BalanceChanged {
			wallet.BalanceChanged = false
			fmt.Println("Your balance has just changed")
			fmt.Print(wallet.DumpBalance(nil, false))
			textui.ShowPrompt()
		}
	} else {
		println("Warning: AcceptBlock failed. If the block was valid, you may need to rebuild the unspent DB (-r)")
	}
	return
}


func retry_cached_blocks() bool {
	if len(network.CachedBlocks)==0 {
		return false
	}
	accepted_cnt := 0
	for k, v := range network.CachedBlocks {
		config.Busy("Cache.CheckBlock "+v.Block.Hash.String())
		e, dos, maybelater := config.BlockChain.CheckBlock(v.Block)
		if e == nil {
			config.Busy("Cache.AcceptBlock "+v.Block.Hash.String())
			e := LocalAcceptBlock(v.Block, v.Conn)
			if e == nil {
				//println("*** Old block accepted", config.BlockChain.BlockTreeEnd.Height)
				config.CountSafe("BlocksFromCache")
				delete(network.CachedBlocks, k)
				accepted_cnt++
				break // One at a time should be enough
			} else {
				println("retry AcceptBlock:", e.Error())
				config.CountSafe("CachedBlocksDOS")
				v.Conn.DoS()
				delete(network.CachedBlocks, k)
			}
		} else {
			if !maybelater {
				println("retry CheckBlock:", e.Error())
				config.CountSafe("BadCachedBlocks")
				if dos {
					v.Conn.DoS()
					config.CountSafe("CachedBlocksDoS")
				}
				delete(network.CachedBlocks, k)
			}
		}
	}
	return accepted_cnt>0 && len(network.CachedBlocks)>0
}


// Called from the blockchain thread
func HandleNetBlock(newbl *network.BlockRcvd) {
	config.CountSafe("HandleNetBlock")
	bl := newbl.Block
	config.Busy("CheckBlock "+bl.Hash.String())
	e, dos, maybelater := config.BlockChain.CheckBlock(bl)
	if e != nil {
		if maybelater {
			addBlockToCache(bl, newbl.Conn)
		} else {
			println(dos, e.Error())
			if dos {
				newbl.Conn.DoS()
			}
		}
	} else {
		config.Busy("LocalAcceptBlock "+bl.Hash.String())
		e = LocalAcceptBlock(bl, newbl.Conn)
		if e == nil {
			retryCachedBlocks = retry_cached_blocks()
		} else {
			println("AcceptBlock:", e.Error())
			newbl.Conn.DoS()
		}
	}
}


func main() {
	if btc.EC_Verify==nil {
		fmt.Println("WARNING: EC_Verify acceleration disabled. Enable EC_Verify wrapper if possible.")
		fmt.Println("         Look for the instruction in README.md or in client/speedup folder.")
	}

	fmt.Println("Gocoin client version", btc.SourcesTag)
	runtime.GOMAXPROCS(runtime.NumCPU()) // It seems that Go does not do it by default

	// Disable Ctrl+C
	signal.Notify(killchan, os.Interrupt, os.Kill)
	defer func() {
		if r := recover(); r != nil {
			err, ok := r.(error)
			if !ok {
				err = fmt.Errorf("pkg: %v", r)
			}
			println("main panic recovered:", err.Error())
			network.NetCloseAll()
			config.CloseBlockChain()
			network.ClosePeerDB()
			dbase.UnlockDatabaseDir()
			os.Exit(1)
		}
	}()

	host_init() // This will create the DB lock file and keep it open

	// Clean up the DB lock file on exit

	// load default wallet and its balance
	wallet.LoadWallet(config.GocoinHomeDir+"wallet"+string(os.PathSeparator)+"DEFAULT")
	if wallet.MyWallet==nil {
		wallet.LoadWallet(config.GocoinHomeDir+"wallet.txt")
	}
	if wallet.MyWallet!=nil {
		wallet.UpdateBalance()
		fmt.Print(wallet.DumpBalance(nil, false))
	}

	peersTick := time.Tick(defragEvery)
	txPoolTick := time.Tick(time.Minute)
	netTick := time.Tick(time.Second)

	network.InitPeers(config.GocoinHomeDir)

	config.Last.Block = config.BlockChain.BlockTreeEnd
	config.Last.Time = time.Unix(int64(config.Last.Block.Timestamp), 0)

	for k, v := range config.BlockChain.BlockIndex {
		network.ReceivedBlocks[k] = &network.OneReceivedBlock{Time: time.Unix(int64(v.Timestamp), 0)}
	}

	go textui.MainThread()
	if config.CFG.WebUI.Interface!="" {
		fmt.Println("Starting WebUI at", config.CFG.WebUI.Interface, "...")
		go webui.ServerThread(config.CFG.WebUI.Interface)
	}

	for !config.Exit_now {
		config.CountSafe("MainThreadLoops")
		for retryCachedBlocks {
			retryCachedBlocks = retry_cached_blocks()
			// We have done one per loop - now do something else if pending...
			if len(network.NetBlocks)>0 || len(textui.UiChannel)>0 {
				break
			}
		}

		config.Busy("")

		select {
			case s := <-killchan:
				fmt.Println("Got signal:", s)
				config.Exit_now = true
				continue

			case newbl := <-network.NetBlocks:
				HandleNetBlock(newbl)

			case newtx := <-network.NetTxs:
				network.HandleNetTx(newtx, false)

			case cmd := <-textui.UiChannel:
				config.Busy("UI command")
				cmd.Handler(cmd.Param)
				cmd.Done.Done()
				continue

			case <-peersTick:
				network.ExpirePeers()

			case <-txPoolTick:
				network.ExpireTxs()

			case <-netTick:
				network.NetworkTick()

			case <-time.After(time.Second/2):
				config.CountSafe("MainThreadTouts")
				if !retryCachedBlocks {
					config.Busy("config.BlockChain.Idle()")
					config.BlockChain.Idle()
				}
				continue
		}

		config.CountSafe("NetMessagesGot")
	}

	network.NetCloseAll()
	config.CloseBlockChain()
	network.ClosePeerDB()
	dbase.UnlockDatabaseDir()
}
