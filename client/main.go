package main

import (
	"os"
	"fmt"
	"time"
	"sync"
	"runtime"
	"os/signal"
	"github.com/piotrnar/gocoin/btc"
	_ "github.com/piotrnar/gocoin/btc/qdb"
)

const (
	PendingFifoLen = 2000
	MaxCachedBlocks = 600
)

type onePendingBlock struct {
	hash *btc.Uint256
	noticed time.Time
	single bool
}

var (
	GenesisBlock *btc.Uint256
	Magic [4]byte
	BlockChain *btc.Chain
	AddrVersion byte

	exit_now bool

	dbg int64
	beep bool

	LastBlock *btc.BlockTreeNode
	LastBlockReceived time.Time

	mutex, counter_mutex sync.Mutex
	netBlocks chan *blockRcvd = make(chan *blockRcvd, 300)
	netTxs chan *txRcvd = make(chan *txRcvd, 300)
	uiChannel chan *oneUiReq = make(chan *oneUiReq, 1)

	pendingBlocks map[[btc.Uint256IdxLen]byte] *onePendingBlock =
		make(map[[btc.Uint256IdxLen]byte] *onePendingBlock, 600)
	pendingFifo chan [btc.Uint256IdxLen]byte = make(chan [btc.Uint256IdxLen]byte, PendingFifoLen)

	retryCachedBlocks bool
	cachedBlocks map[[btc.Uint256IdxLen]byte] oneCachedBlock = make(map[[btc.Uint256IdxLen]byte] oneCachedBlock, MaxCachedBlocks)
	receivedBlocks map[[btc.Uint256IdxLen]byte] *onePendingBlock =
		make(map[[btc.Uint256IdxLen]byte] *onePendingBlock, 300e3)

	Counter map[string] uint64 = make(map[string]uint64)

	busy string
)


type blockRcvd struct {
	conn *oneConnection
	bl *btc.Block
}

type txRcvd struct {
	conn *oneConnection
	tx *btc.Tx
	raw []byte
}

type oneCachedBlock struct {
	time.Time
	*btc.Block
	conn *oneConnection
}

func Busy(b string) {
	mutex.Lock()
	busy = b
	mutex.Unlock()
}

func CountSafe(k string) {
	counter_mutex.Lock()
	Counter[k]++
	counter_mutex.Unlock()
}

func CountSafeAdd(k string, val uint64) {
	counter_mutex.Lock()
	Counter[k] += val
	counter_mutex.Unlock()
}


func list_unspent(addr string) {
	fmt.Println("Checking unspent coins for addr", addr)
	var a[1] *btc.BtcAddr
	var e error
	a[0], e = btc.NewAddrFromString(addr)
	if e != nil {
		println(e.Error())
		return
	}
	unsp := BlockChain.GetAllUnspent(a[:], false)
	var sum uint64
	for i := range unsp {
		fmt.Println(unsp[i].String())
		sum += unsp[i].Value
	}
	fmt.Printf("Total %.8f unspent BTC at address %s\n", float64(sum)/1e8, a[0].String());
}


func addBlockToCache(bl *btc.Block, conn *oneConnection) {
	// we use cachedBlocks only from one therad so no need for a mutex
	if len(cachedBlocks)==MaxCachedBlocks {
		// Remove the oldest one
		oldest := time.Now()
		var todel [btc.Uint256IdxLen]byte
		for k, v := range cachedBlocks {
			if v.Time.Before(oldest) {
				oldest = v.Time
				todel = k
			}
		}
		delete(cachedBlocks, todel)
		CountSafe("CacheBlocksExpired")
	}
	cachedBlocks[bl.Hash.BIdx()] = oneCachedBlock{Time:time.Now(), Block:bl, conn:conn}
}


func LocalAcceptBlock(bl *btc.Block, from *oneConnection) (e error) {
	mutex.Lock()
	rb := receivedBlocks[bl.Hash.BIdx()]
	mutex.Unlock()
	sta := time.Now()
	e = BlockChain.AcceptBlock(bl)
	sto := time.Now()
	if e == nil {
		tim := sto.Sub(sta)
		if tim > 3*time.Second {
			fmt.Println("AcceptBlock", LastBlock.Height, "took", tim)
			ui_show_prompt()
		}

		for i:=1; i<len(bl.Txs); i++ {
			TxMined(bl.Txs[i].Hash.Hash)
		}

		if int64(bl.BlockTime) > time.Now().Add(-10*time.Minute).Unix() {
			if measure_block_timing {
				println("New Block", bl.Hash.String(), "handled in",
					sta.Sub(rb.noticed).String(), "->", sto.Sub(rb.noticed).String())
				ui_show_prompt()
			}

			// Freshly mined block - do the inv and beeps...
			Busy("NetRouteInv")
			NetRouteInv(2, bl.Hash, from)

			if beep {
				fmt.Println("\007Received block", BlockChain.BlockTreeEnd.Height)
				ui_show_prompt()
			}

			if mined_by_us(bl.Raw) {
				fmt.Println("\007Mined by '"+CFG.MinerID+"':", bl.Hash)
				ui_show_prompt()
			}

			if LastBlock == BlockChain.BlockTreeEnd {
				// Last block has not changed, so it must have been an orphaned block
				bln := BlockChain.BlockIndex[bl.Hash.BIdx()]
				commonNode := LastBlock.FirstCommonParent(bln)
				forkDepth := bln.Height - commonNode.Height
				fmt.Println("Orphaned block:", bln.Height, bl.Hash.String())
				if forkDepth > 1 {
					fmt.Println("\007\007\007WARNING: the fork is", forkDepth, "blocks deep")
				}
				ui_show_prompt()
			}

			if BalanceChanged {
				fmt.Println("\007Your balance has just changed")
				fmt.Print(DumpBalance(nil, false))
				ui_show_prompt()
			}
		}

		LastBlockReceived = time.Now()
		LastBlock = BlockChain.BlockTreeEnd
		BalanceChanged = false

	} else {
		println("Warning: AcceptBlock failed. If the block was valid, you may need to rebuild the unspent DB (-r)")
	}
	return
}


func retry_cached_blocks() bool {
	if len(cachedBlocks)==0 {
		return false
	}
	accepted_cnt := 0
	for k, v := range cachedBlocks {
		Busy("Cache.CheckBlock "+v.Block.Hash.String())
		e, dos, maybelater := BlockChain.CheckBlock(v.Block)
		if e == nil {
			Busy("Cache.AcceptBlock "+v.Block.Hash.String())
			e := LocalAcceptBlock(v.Block, v.conn)
			if e == nil {
				//println("*** Old block accepted", BlockChain.BlockTreeEnd.Height)
				CountSafe("BlocksFromCache")
				delete(cachedBlocks, k)
				accepted_cnt++
				break // One at a time should be enough
			} else {
				println("retry AcceptBlock:", e.Error())
				CountSafe("CachedBlocksDOS")
				v.conn.DoS()
				delete(cachedBlocks, k)
			}
		} else {
			if !maybelater {
				println("retry CheckBlock:", e.Error())
				CountSafe("BadCachedBlocks")
				if dos {
					v.conn.DoS()
					CountSafe("CachedBlocksDoS")
				}
				delete(cachedBlocks, k)
			}
		}
	}
	return accepted_cnt>0 && len(cachedBlocks)>0
}


func ui_quit(par string) {
	exit_now = true
}

func blchain_stats(par string) {
	fmt.Println(BlockChain.Stats())
}


func save_bchain(par string) {
	BlockChain.Save()
}


func switch_sync(par string) {
	offit := (par=="0" || par=="false" || par=="off")

	// Actions when syncing is enabled:
	if !BlockChain.DoNotSync {
		if offit {
			BlockChain.DoNotSync = true
			fmt.Println("Sync has been disabled. Do not forget to switch it back on, to have DB changes on disk.")
		} else {
			fmt.Println("Sync is enabled. Use 'sync 0' to switch it off.")
		}
		return
	}

	// Actions when syncing is disabled:
	if offit {
		fmt.Println("Sync is already disabled. Request ignored.")
	} else {
		fmt.Println("Switching sync back on & saving all the changes...")
		BlockChain.Sync()
		fmt.Println("Sync is back on now.")
	}
}


func init() {
	newUi("bchain b", true, blchain_stats, "Display blockchain statistics")
	newUi("quit q", true, ui_quit, "Exit nicely, saving all files. Otherwise use Ctrl+C")
	newUi("unspent u", true, list_unspent, "Shows unpent outputs for a given address")
	newUi("sync", true, switch_sync, "Control sync of the database to disk")
}


func main() {
	if btc.EC_Verify==nil {
		fmt.Println("WARNING: EC_Verify acceleration disabled. Enable EC_Verify wrapper if possible.")
		fmt.Println("         Look for the instruction in README.md or in client/speedup folder.")
	}

	fmt.Println("Gocoin client version", btc.SourcesTag)
	runtime.GOMAXPROCS(runtime.NumCPU()) // It seems that Go does not do it by default

	UploadLimit = CFG.MaxUpKBps << 10
	DownloadLimit = CFG.MaxDownKBps << 10

	// Disable Ctrl+C
	killchan := make(chan os.Signal, 1)
	signal.Notify(killchan, os.Interrupt, os.Kill)

	host_init() // This will create the DB lock file and keep it open

	// Clean up the DB lock file on exit
	defer UnlockDatabaseDir()

	// load default wallet and its balance
	LoadWallet(GocoinHomeDir+"wallet.txt")
	if MyWallet!=nil {
		MyBalance = BlockChain.GetAllUnspent(MyWallet.addrs, true)
		BalanceInvalid = false
		fmt.Print(DumpBalance(nil, false))
	}

	initPeers(GocoinHomeDir)
	go txPoolManager()

	LastBlock = BlockChain.BlockTreeEnd
	LastBlockReceived = time.Unix(int64(LastBlock.Timestamp), 0)

	for k, v := range BlockChain.BlockIndex {
		receivedBlocks[k] = &onePendingBlock{hash:v.BlockHash, noticed:time.Unix(int64(v.Timestamp), 0)}
	}

	go network_process()
	go do_userif()
	if CFG.WebUI!="" {
		go webserver()
	}

	for !exit_now {
		CountSafe("MainThreadLoops")
		for retryCachedBlocks {
			retryCachedBlocks = retry_cached_blocks()
			// We have done one per loop - now do something else if pending...
			if len(netBlocks)>0 || len(uiChannel)>0 {
				break
			}
		}

		Busy("")

		select {
			case s := <-killchan:
				fmt.Println("Got signal:", s)
				exit_now = true
				continue

			case newbl := <-netBlocks:
				HandleNetBlock(newbl)

			case newtx := <-netTxs:
				HandleNetTx(newtx)

			case cmd := <-uiChannel:
				Busy("UI command")
				CountSafe("UI messages")
				cmd.handler(cmd.param)
				cmd.done.Done()
				continue

			case <-time.After(time.Second):
				CountSafe("MainThreadTouts")
				if !retryCachedBlocks {
					Busy("BlockChain.Idle()")
					BlockChain.Idle()
				}
				continue
		}

		CountSafe("NetMessagesGot")

	}
	println("Closing blockchain")
	BlockChain.Close()
	peerDB.Close()
}
