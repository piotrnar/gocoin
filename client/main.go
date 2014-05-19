package main

import (
	"os"
	"fmt"
	"time"
	"unsafe"
	"runtime"
	"io/ioutil"
	"os/signal"
	"runtime/debug"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/qdb"
	"github.com/piotrnar/gocoin/others/ver"
	"github.com/piotrnar/gocoin/others/sys"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/wallet"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/usif"
	"github.com/piotrnar/gocoin/client/usif/textui"
	"github.com/piotrnar/gocoin/client/usif/webui"
)


var killchan chan os.Signal = make(chan os.Signal)
var retryCachedBlocks bool


/* this would print text messages for transactions that are being processed
func contains_message(tx *btc.Tx) []byte {
	for i := range tx.TxOut {
		if len(tx.TxOut[i].Pk_script)>=2 && tx.TxOut[i].Pk_script[0]==0x6a {
			s, e := btc.ReadString(bytes.NewBuffer(tx.TxOut[i].Pk_script[1:]))
			if e==nil {
				return []byte(s)
			}
		}
	}
	return nil
}
*/


func LocalAcceptBlock(bl *btc.Block, from *network.OneConnection) (e error) {
	sta := time.Now()
	e = common.BlockChain.AcceptBlock(bl)
	if e == nil {
		wallet.DoPendingStealths()
		network.MutexRcv.Lock()
		network.ReceivedBlocks[bl.Hash.BIdx()].TmAccept = time.Now().Sub(sta)
		network.MutexRcv.Unlock()

		for i:=1; i<len(bl.Txs); i++ {
			network.TxMined(bl.Txs[i])
			/*
			if msg:=contains_message(bl.Txs[i]); msg!=nil {
				for xx:=range msg {
					if msg[xx]<' ' || msg[xx]>127 {
						msg[xx] = '.'
					}
				}
				fmt.Println("TX", bl.Txs[i].Hash.String(), "says:", "'" + string(msg) + "'")
				textui.ShowPrompt()
			}
			*/
		}

		if int64(bl.BlockTime()) > time.Now().Add(-10*time.Minute).Unix() {
			// Freshly mined block - do the inv and beeps...
			common.Busy("NetRouteInv")
			network.NetRouteInv(2, bl.Hash, from)

			if common.CFG.Beeps.NewBlock {
				fmt.Println("\007Received block", common.BlockChain.BlockTreeEnd.Height)
				textui.ShowPrompt()
			}

			if common.MinedByUs(bl.Raw) {
				fmt.Println("\007Mined by '"+common.CFG.Beeps.MinerID+"':", bl.Hash)
				textui.ShowPrompt()
			}

			if common.CFG.Beeps.ActiveFork && common.Last.Block == common.BlockChain.BlockTreeEnd {
				// Last block has not changed, so it must have been an orphaned block
				bln := common.BlockChain.BlockIndex[bl.Hash.BIdx()]
				commonNode := common.Last.Block.FirstCommonParent(bln)
				forkDepth := bln.Height - commonNode.Height
				fmt.Println("Orphaned block:", bln.Height, bl.Hash.String())
				if forkDepth > 1 {
					fmt.Println("\007\007\007WARNING: the fork is", forkDepth, "blocks deep")
				}
				textui.ShowPrompt()
			}

			if wallet.BalanceChanged && common.CFG.Beeps.NewBalance{
				fmt.Print("\007")
			}
		}

		common.Last.Mutex.Lock()
		common.Last.Time = time.Now()
		common.Last.Block = common.BlockChain.BlockTreeEnd
		common.Last.Mutex.Unlock()

		if common.CFG.Memory.NoCacheBefore < 0 { // Remove old records from unspent database
			if target := int(common.Last.Block.Height) + common.CFG.Memory.NoCacheBefore + 1; target > 0 {
				common.BlockChain.Unspent.BrowseUTXO(true, func(db *qdb.DB, k qdb.KeyType, rec *btc.OneWalkRecord) uint32 {
					if int(rec.BlockHeight()) <= target && !rec.IsStealthIdx() {
						common.CountSafe("BlockUTXOAged")
						return btc.WALK_NOMORE
					}
					return 0
				})
			}
		}

		if wallet.BalanceChanged {
			wallet.BalanceChanged = false
			fmt.Println("Your balance has just changed")
			fmt.Print(wallet.DumpBalance(wallet.MyBalance, nil, false, true))
			textui.ShowPrompt()
		}
	} else {
		fmt.Println("Warning: AcceptBlock failed. If the block was valid, you may need to rebuild the unspent DB (-r)")
	}
	return
}


func retry_cached_blocks() bool {
	if len(network.CachedBlocks)==0 {
		return false
	}
	accepted_cnt := 0
	for k, v := range network.CachedBlocks {
		common.Busy("Cache.CheckBlock "+v.Block.Hash.String())
		e, dos, maybelater := common.BlockChain.CheckBlock(v.Block)
		if e == nil {
			common.Busy("Cache.AcceptBlock "+v.Block.Hash.String())
			e := LocalAcceptBlock(v.Block, v.Conn)
			if e == nil {
				//fmt.Println("*** Old block accepted", common.BlockChain.BlockTreeEnd.Height)
				common.CountSafe("BlocksFromCache")
				delete(network.CachedBlocks, k)
				accepted_cnt++
				break // One at a time should be enough
			} else {
				fmt.Println("retry AcceptBlock:", e.Error())
				v.Conn.DoS("BadCachedBlock1")
				delete(network.CachedBlocks, k)
			}
		} else {
			if !maybelater {
				fmt.Println("retry CheckBlock:", e.Error())
				common.CountSafe("BadCachedBlocks")
				if dos {
					v.Conn.DoS("BadCachedBlock2")
				}
				delete(network.CachedBlocks, k)
			}
		}
	}
	return accepted_cnt>0 && len(network.CachedBlocks)>0
}


// Called from the blockchain thread
func HandleNetBlock(newbl *network.BlockRcvd) {
	common.CountSafe("HandleNetBlock")
	bl := newbl.Block
	common.Busy("CheckBlock "+bl.Hash.String())
	e, dos, maybelater := common.BlockChain.CheckBlock(bl)
	if e != nil {
		if maybelater {
			network.AddBlockToCache(bl, newbl.Conn)
		} else {
			fmt.Println(dos, e.Error())
			if dos {
				newbl.Conn.DoS("CheckBlock")
			}
		}
	} else {
		common.Busy("LocalAcceptBlock "+bl.Hash.String())
		e = LocalAcceptBlock(bl, newbl.Conn)
		if e == nil {
			retryCachedBlocks = retry_cached_blocks()
		} else {
			fmt.Println("AcceptBlock:", e.Error())
			newbl.Conn.DoS("LocalAcceptBl")
		}
	}
}


func defrag_db() {
	qdb.SetDefragPercent(1)
	fmt.Print("Defragmenting UTXO database")
	os.Stdout.Sync()
	for {
		if !common.BlockChain.Unspent.Idle() {
			break
		}
		fmt.Print(".")
		os.Stdout.Sync()
	}
	fmt.Println("done")
	os.Stdout.Sync()

	fmt.Println("Creating empty database in", common.GocoinHomeDir+"defrag", "...")
	os.RemoveAll(common.GocoinHomeDir+"defrag")
	defragdb := btc.NewBlockDB(common.GocoinHomeDir+"defrag")
	fmt.Println("Defragmenting the database...")
	blk := common.BlockChain.BlockTreeRoot
	for {
		blk = blk.FindPathTo(common.BlockChain.BlockTreeEnd)
		if blk==nil {
			fmt.Println("Database defragmenting finished successfully")
			fmt.Println("To use the new DB, move the two new files to a parent directory and restart the client")
			break
		}
		if (blk.Height&0xff)==0 {
			fmt.Printf("%d / %d blocks written (%d%%)\r", blk.Height, common.BlockChain.BlockTreeEnd.Height,
				100 * blk.Height / common.BlockChain.BlockTreeEnd.Height)
		}
		bl, trusted, er := common.BlockChain.Blocks.BlockGet(blk.BlockHash)
		if er != nil {
			fmt.Println("FATAL ERROR during BlockGet:", er.Error())
			break
		}
		nbl, er := btc.NewBlock(bl)
		if er != nil {
			fmt.Println("FATAL ERROR during NewBlock:", er.Error())
			break
		}
		nbl.Trusted = trusted
		defragdb.BlockAdd(blk.Height, nbl)
	}
	defragdb.Sync()
	defragdb.Close()
}


func main() {
	var ptr *byte
	if unsafe.Sizeof(ptr) < 8 {
		fmt.Println("WARNING: Gocoin client shall be build for 64-bit arch. It will likely crash now.")
	}

	fmt.Println("Gocoin client version", ver.SourcesTag)
	runtime.GOMAXPROCS(runtime.NumCPU()) // It seems that Go does not do it by default

	// Disable Ctrl+C
	signal.Notify(killchan, os.Interrupt, os.Kill)
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
			network.ClosePeerDB()
			sys.UnlockDatabaseDir()
			os.Exit(1)
		}
	}()

	host_init() // This will create the DB lock file and keep it open

	default_wallet_fn := common.GocoinHomeDir+"wallet"+string(os.PathSeparator)+wallet.DefaultFileName
	fi, _ := os.Stat(default_wallet_fn)
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

	// ... and now load the dafault wallet
	wallet.LoadWallet(default_wallet_fn)
	if wallet.MyWallet!=nil {
		wallet.UpdateBalance()
		fmt.Print(wallet.DumpBalance(wallet.MyBalance, nil, false, true))
	}

	peersTick := time.Tick(5*time.Minute)
	txPoolTick := time.Tick(time.Minute)
	netTick := time.Tick(time.Second)

	network.InitPeers(common.GocoinHomeDir)

	common.Last.Block = common.BlockChain.BlockTreeEnd
	common.Last.Time = time.Unix(int64(common.Last.Block.Timestamp()), 0)
	if common.Last.Time.After(time.Now()) {
		common.Last.Time = time.Now()
	}

	for k, v := range common.BlockChain.BlockIndex {
		network.ReceivedBlocks[k] = &network.OneReceivedBlock{Time: time.Unix(int64(v.Timestamp()), 0)}
	}

	if common.CFG.TextUI.Enabled {
		go textui.MainThread()
	}

	if common.CFG.WebUI.Interface!="" {
		fmt.Println("Starting WebUI at", common.CFG.WebUI.Interface, "...")
		go webui.ServerThread(common.CFG.WebUI.Interface)
	}

	for !usif.Exit_now {
		common.CountSafe("MainThreadLoops")
		for retryCachedBlocks {
			retryCachedBlocks = retry_cached_blocks()
			// We have done one per loop - now do something else if pending...
			if len(network.NetBlocks)>0 || len(usif.UiChannel)>0 {
				break
			}
		}

		common.Busy("")

		select {
			case s := <-killchan:
				fmt.Println("Got signal:", s)
				usif.Exit_now = true
				continue

			case newbl := <-network.NetBlocks:
				common.CountSafe("MainNetBlock")
				HandleNetBlock(newbl)

			case newtx := <-network.NetTxs:
				common.CountSafe("MainNetTx")
				network.HandleNetTx(newtx, false)

			case newal := <-network.NetAlerts:
				common.CountSafe("MainNetAlert")
				fmt.Println("\007" + newal)
				textui.ShowPrompt()

			case <-netTick:
				common.CountSafe("MainNetTick")
				network.NetworkTick()

			case cmd := <-usif.UiChannel:
				common.CountSafe("MainUICmd")
				common.Busy("UI command")
				cmd.Handler(cmd.Param)
				cmd.Done.Done()
				continue

			case <-peersTick:
				network.ExpirePeers()

			case <-txPoolTick:
				network.ExpireTxs()

			case <-time.After(time.Second/2):
				common.CountSafe("MainThreadTouts")
				if !retryCachedBlocks {
					common.Busy("common.BlockChain.Idle()")
					if common.BlockChain.Idle() {
						common.CountSafe("ChainIdleUsed")
					}
				}
				continue
		}
	}

	network.NetCloseAll()
	network.ClosePeerDB()

	if usif.DefragBlocksDB {
		defrag_db()
	}

	common.CloseBlockChain()
	sys.UnlockDatabaseDir()
}
