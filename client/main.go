package main

import (
	"os"
	"fmt"
	"flag"
	"time"
	"sync"
	"runtime"
	"github.com/piotrnar/gocoin/btc"
	_ "github.com/piotrnar/gocoin/btc/qdb"
)

const (
	PendingFifoLen = 2000
	MaxCachedBlocks = 600
)

var (
	testnet *bool = flag.Bool("t", false, "Use Testnet3")
	rescan *bool = flag.Bool("r", false, "Rebuild the unspent DB (fixes 'Unknown input TxID' errors)")
	proxy *string = flag.String("c", "", "Connect to this host and nowhere else")
	server *bool = flag.Bool("l", true, "Enable TCP server (allow incomming connections)")
	datadir *string = flag.String("d", "", "Specify Gocoin's database root folder")
	nosync *bool = flag.Bool("nosync", false, "Init blockchain with syncing disabled (dangerous!)")
	maxul = flag.Uint("ul", 0, "Upload limit in KB/s (0 for no limit)")
	maxdl = flag.Uint("dl", 0, "Download limit in KB/s (0 for no limit)")

	minerId *string = flag.String("miner", mid_am, "Monitor new blocks with the string in their coinbase TX")

	GenesisBlock *btc.Uint256
	Magic [4]byte
	BlockChain *btc.Chain
	AddrVersion byte

	exit_now bool

	dbg int64
	beep bool

	LastBlock *btc.BlockTreeNode
	LastBlockReceived time.Time = time.Now().Add(-10*time.Minute) // Let's just assume this on init

	mutex, counter_mutex sync.Mutex
	uicmddone chan bool = make(chan bool, 1)
	netBlocks chan *blockRcvd = make(chan *blockRcvd, 300)
	uiChannel chan oneUiReq = make(chan oneUiReq, 1)

	pendingBlocks map[[btc.Uint256IdxLen]byte] *btc.Uint256 = make(map[[btc.Uint256IdxLen]byte] *btc.Uint256, 600)
	pendingFifo chan [btc.Uint256IdxLen]byte = make(chan [btc.Uint256IdxLen]byte, PendingFifoLen)

	cachedBlocks map[[btc.Uint256IdxLen]byte] oneCachedBlock = make(map[[btc.Uint256IdxLen]byte] oneCachedBlock, MaxCachedBlocks)
	receivedBlocks map[[btc.Uint256IdxLen]byte] int64 = make(map[[btc.Uint256IdxLen]byte] int64, 300e3)

	Counter map[string] uint64 = make(map[string]uint64)

	busy string
)


type blockRcvd struct {
	conn *oneConnection
	bl *btc.Block
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


func AcceptBlock(bl *btc.Block) (e error) {
	sta := time.Now()
	e = BlockChain.AcceptBlock(bl)
	sto := time.Now()
	if e == nil {
		tim := sto.Sub(sta)
		if tim > 3*time.Second {
			fmt.Println("AcceptBlock", LastBlock.Height, "took", tim)
			ui_show_prompt()
		}
		if BalanceChanged {
			fmt.Println("\007Your balance has just changed")
			DumpBalance(nil)
			ui_show_prompt()
			BalanceChanged = false
		}
		if mined_by_us(bl.Raw) {
			fmt.Println("\007Mined by ASICMINER:", bl.Hash)
			ui_show_prompt()
		}
		if LastBlock == BlockChain.BlockTreeEnd {
			// last block has not changes - it must have been an orphaned block
			fmt.Println("\007Orphaned block:", LastBlock.Height, bl.Hash.String())
			ui_show_prompt()
		} else {
			LastBlock = BlockChain.BlockTreeEnd
		}
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
			e := AcceptBlock(v.Block)
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


func netBlockReceived(conn *oneConnection, b []byte) {
	bl, e := btc.NewBlock(b)
	if e != nil {
		conn.DoS()
		println("NewBlock:", e.Error())
		return
	}

	idx := bl.Hash.BIdx()
	delete(conn.GetBlocksInProgress, idx)
	mutex.Lock()
	if _, got := receivedBlocks[idx]; got {
		if _, ok := pendingBlocks[idx]; ok {
			panic("wtf?")
		} else {
			CountSafe("SameBlockReceived")
		}
		mutex.Unlock()
		return
	}
	receivedBlocks[idx] = time.Now().UnixNano()
	delete(pendingBlocks, idx)
	mutex.Unlock()

	netBlocks <- &blockRcvd{conn:conn, bl:bl}
}


func blockDataNeeded() ([]byte) {
	for len(pendingFifo)>0 && len(netBlocks)<200 {
		idx := <- pendingFifo
		mutex.Lock()
		if _, hadit := receivedBlocks[idx]; !hadit {
			if pbl, hasit := pendingBlocks[idx]; hasit {
				mutex.Unlock()
				pendingFifo <- idx // put it back to the channel
				return pbl.Hash[:]
			} else {
				println("some block not peending anymore")
			}
		} else {
			delete(pendingBlocks, idx)
		}
		mutex.Unlock()
	}
	return nil
}


func blockWanted(h []byte) (yes bool) {
	ha := btc.NewUint256(h)
	idx := ha.BIdx()
	mutex.Lock()
	if _, ok := receivedBlocks[idx]; !ok {
		yes = true
	} else {
		CountSafe("Block not wanted")
	}
	mutex.Unlock()
	return
}


func InvsNotify(h []byte) (need bool) {
	ha := btc.NewUint256(h)
	idx := ha.BIdx()
	mutex.Lock()
	if _, ok := pendingBlocks[idx]; ok {
		CountSafe("InvWasPending")
	} else if _, ok := receivedBlocks[idx]; ok {
		CountSafe("InvWasReceived")
	} else if len(pendingFifo)<PendingFifoLen {
		if dbg>0 {
			fmt.Println("blinv", btc.NewUint256(h).String())
		}
		CountSafe("InvWanted")
		pendingBlocks[idx] = ha
		pendingFifo <- idx
		need = true
	} else {
		CountSafe("InvFIFOfull")
	}
	mutex.Unlock()
	return
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


func GetBlockData(h []byte) []byte {
	bl, _, e  := BlockChain.Blocks.BlockGet(btc.NewUint256(h))
	if e == nil {
		return bl
	}
	println("BlockChain.Blocks.BlockGet failed")
	return nil
}


func main() {
	var sta int64
	var retryCachedBlocks bool

	if btc.EC_Verify==nil {
		fmt.Println("WARNING: EC_Verify acceleration disabled. Enable OpenSSL wrapper if possible.")
		fmt.Println("         Look for the instruction in README.md or in client/speedup folder.")
	}

	fmt.Println("Gocoin client version", btc.SourcesTag)
	fmt.Println("Send 0.01 BTC to 1WEyRRbAgPTpAPUgCSxLrJjXgiTU86WKt if you support this project")
	runtime.GOMAXPROCS(runtime.NumCPU()) // It seems that Go does not do it by default
	if flag.Lookup("h") != nil {
		flag.PrintDefaults()
		os.Exit(0)
	}
	flag.Parse()

	UploadLimit = *maxul << 10
	DownloadLimit = *maxdl << 10

	host_init()

	// load default wallet and its balance
	LoadWallet(GocoinHomeDir+"wallet.txt")
	if MyWallet!=nil {
		MyBalance = BlockChain.GetAllUnspent(MyWallet.addrs, true)
		BalanceInvalid = false
		DumpBalance(nil)
	}

	initPeers(GocoinHomeDir)

	LastBlock = BlockChain.BlockTreeEnd

	sta = time.Now().Unix()
	for k, _ := range BlockChain.BlockIndex {
		receivedBlocks[k] = sta
	}

	go network_process()

	go do_userif()

	var newbl *blockRcvd
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
			case newbl = <-netBlocks:
				break

			case cmd := <-uiChannel:
				Busy("UI command")
				CountSafe("UI messages")
				cmd.handler(cmd.param)
				uicmddone <- true
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

		bl := newbl.bl

		Busy("CheckBlock "+bl.Hash.String())
		e, dos, maybelater := BlockChain.CheckBlock(bl)
		if e != nil {
			if maybelater {
				addBlockToCache(bl, newbl.conn)
			} else {
				println(dos, e.Error())
				if dos {
					newbl.conn.DoS()
				}
			}
		} else {
			Busy("AcceptBlock "+bl.Hash.String())
			e = AcceptBlock(bl)
			if e == nil {
				LastBlockReceived = time.Now()
				// block accepted, so route this inv to peers
				Busy("NetSendInv")
				NetSendInv(2, bl.Hash.Hash[:], newbl.conn)
				if beep {
					fmt.Println("\007Received block", BlockChain.BlockTreeEnd.Height)
					ui_show_prompt()
				}
				retryCachedBlocks = retry_cached_blocks()
			} else {
				println("AcceptBlock:", e.Error())
				newbl.conn.DoS()
			}
		}
	}
	println("Closing blockchain")
	BlockChain.Close()
	peerDB.Close()
}
