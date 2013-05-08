package main

import (
	"fmt"
	"os"
	"flag"
	"time"
	"sync"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
	_ "github.com/piotrnar/gocoin/btc/qdb"
)

const (
	PendingFifoLen = 2000
)

var (
	verbose *bool = flag.Bool("v", false, "Verbose mode")
	testnet *bool = flag.Bool("t", false, "Use Testnet3")
	rescan *bool = flag.Bool("r", false, "Discard unspent outputs DB and rescan the blockchain")
	proxy *string = flag.String("c", "", "Connect to this host")
	server *bool = flag.Bool("l", false, "Enable TCP server (allow incomming connections)")
	datadir *string = flag.String("d", "", "Specify Gocoin's database root folder")
	nosync *bool = flag.Bool("nosync", false, "Init blockchain with syncing disabled (dangerous!)")
	maxul = flag.Uint("ul", 0, "Upload limit in KB/s (0 for no limit)")
	maxdl = flag.Uint("dl", 0, "Download limit in KB/s (0 for no limit)")

	GenesisBlock *btc.Uint256
	Magic [4]byte
	BlockChain *btc.Chain
	AddrVersion byte

	exit_now bool

	dbg uint64
	beep bool

	LastBlock *btc.BlockTreeNode
	LastBlockReceived int64 // time when the last block was received

	mutex sync.Mutex
	uicmddone chan bool = make(chan bool, 1)
	netBlocks chan *blockRcvd = make(chan *blockRcvd, 300)
	uiChannel chan oneUiReq = make(chan oneUiReq, 1)

	pendingBlocks map[[btc.Uint256IdxLen]byte] *btc.Uint256 = make(map[[btc.Uint256IdxLen]byte] *btc.Uint256, 600)
	pendingFifo chan [btc.Uint256IdxLen]byte = make(chan [btc.Uint256IdxLen]byte, PendingFifoLen)
	
	cachedBlocks map[[btc.Uint256IdxLen]byte] *btc.Block = make(map[[btc.Uint256IdxLen]byte] *btc.Block)
	receivedBlocks map[[btc.Uint256IdxLen]byte] int64 = make(map[[btc.Uint256IdxLen]byte] int64, 300e3)

	MyWallet *oneWallet

	InvsIgnored, BlockDups, BlocksNeeded, NetMsgsCnt, UiMsgsCnt, FifoFullCnt uint64
	TicksCnt uint64
	busy string

	TransactionsToSend map[[32]byte] []byte = make(map[[32]byte] []byte)
)


type blockRcvd struct {
	conn *oneConnection
	bl *btc.Block
}


func Busy(b string) {
	mutex.Lock()
	busy = b
	mutex.Unlock()
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


func show_balance(p string) {
	if MyWallet==nil {
		println("You have no wallet")
		return
	}
	if len(MyWallet.addrs)==0 {
		println("You have no addresses")
		return
	}
	os.RemoveAll("balance")
	os.MkdirAll("balance/", 0770)

	utxt, _ := os.Create("balance/unspent.txt")

	unsp := BlockChain.GetAllUnspent(MyWallet.addrs, true)
	var sum uint64
	for i := range unsp {
		sum += unsp[i].Value
		
		if len(unsp)<100 {
			fmt.Printf("%7d %s @ %s (%s)\n", 1+BlockChain.BlockTreeEnd.Height-unsp[i].MinedAt,
				unsp[i].String(), MyWallet.addrs[unsp[i].AskIndex].String(),
				MyWallet.label[unsp[i].AskIndex])
		}

		// update the balance/ folder
		if utxt != nil {
			po, e := BlockChain.Unspent.UnspentGet(&unsp[i].TxPrevOut)
			if e != nil {
				println("UnspentGet:", e.Error())
				fmt.Println("This should not happen - please, report a bug.")
				fmt.Println("You can probably fix it by launching the client with -rescan")
				os.Exit(1)
			}
			
			txid := btc.NewUint256(unsp[i].TxPrevOut.Hash[:])
			
			// Store the unspent line in balance/unspent.txt
			fmt.Fprintf(utxt, "%s # %.8f BTC / %d / %s (%s)\n", unsp[i].TxPrevOut.String(), 
				float64(unsp[i].Value)/1e8, unsp[i].MinedAt,
				MyWallet.addrs[unsp[i].AskIndex].String(), MyWallet.label[unsp[i].AskIndex])
				
			
			// store the entire transactiojn in balance/<txid>.tx
			fn := "balance/"+txid.String()[:64]+".tx"
			txf, _ := os.Open(fn)
			if txf != nil {
				// This file already exist - do no need to redo it
				txf.Close()
				continue
			}

			// Find the block with the indicated Height in the main tree
			BlockChain.BlockIndexAccess.Lock()
			n := BlockChain.BlockTreeEnd
			if n.Height < po.BlockHeight {
				println(n.Height, po.BlockHeight)
				BlockChain.BlockIndexAccess.Unlock()
				panic("This should not happen")
			}
			for n.Height > po.BlockHeight {
				n = n.Parent
			}
			BlockChain.BlockIndexAccess.Unlock()

			bd, _, e := BlockChain.Blocks.BlockGet(n.BlockHash)
			if e != nil {
				println("BlockGet", n.BlockHash.String(), po.BlockHeight, e.Error())
				fmt.Println("This should not happen - please, report a bug.")
				fmt.Println("You can probably fix it by launching the client with -rescan")
				os.Exit(1)
			}

			bl, e := btc.NewBlock(bd)
			if e != nil {
				println("NewBlock: ", e.Error())
				os.Exit(1)
			}

			e = bl.BuildTxList()
			if e != nil {
				println("BuildTxList:", e.Error())
				os.Exit(1)
			}

			// Find the transaction we need and store it in the file
			for i := range bl.Txs {
				if bl.Txs[i].Hash.Equal(txid) {
					txf, _ = os.Create(fn)
					if txf==nil {
						println("Cannot create ", fn)
						os.Exit(1)
					}
					txf.Write(bl.Txs[i].Serialize())
					txf.Close()
					break
				}
			}
		}
	}
	fmt.Printf("%.8f BTC in total, in %d unspent outputs\n", float64(sum)/1e8, len(unsp))
	if utxt != nil {
		fmt.Println("Your balance data has been saved to the 'balance/' folder.")
		fmt.Println("You nend to move this folder to your wallet PC, to spend the coins.")
		utxt.Close()
	}
}


func retry_cached_blocks() bool {
	if len(cachedBlocks)==0 {
		return false
	}
	if len(netBlocks) > 0 {
		return true
	}
	for k, v := range cachedBlocks {
		e, _, maybelater := BlockChain.CheckBlock(v)
		if e == nil {
			e := BlockChain.AcceptBlock(v)
			if e == nil {
				//println("*** Old block accepted", BlockChain.BlockTreeEnd.Height)
				delete(cachedBlocks, k)
				LastBlock = BlockChain.BlockTreeEnd
				LastBlockReceived = time.Now().Unix()
				return len(cachedBlocks)>0
			} else {
				println("retry AcceptBlock:", e.Error())
				delete(cachedBlocks, k)
				return len(cachedBlocks)>0
			}
		} else {
			if !maybelater {
				println("retry CheckBlock:", e.Error())
				delete(cachedBlocks, k)
				return len(cachedBlocks)>0
			}
		}
	}
	return false
}

/*
func findAllLeafes(n *btc.BlockTreeNode) {
	if len(n.Childs)==0 {
		println("Leaf:", n.BlockHash.String())
		return
	}
	for i := range n.Childs {
		findAllLeafes(n.Childs[i])
	}
}
*/

func netBlockReceived(conn *oneConnection, b []byte) {
	bl, e := btc.NewBlock(b)
	if e != nil {
		conn.DoS()
		println("NewBlock:", e.Error())
		return
	}

	mutex.Lock()
	idx := bl.Hash.BIdx()
	if _, got := receivedBlocks[idx]; got {
		if _, ok := pendingBlocks[idx]; ok {
			panic("wtf?")
		} else {
			BlockDups++
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
	}
	mutex.Unlock()
	if !yes {
		InvsIgnored++
	}
	return
}


func InvsNotify(h []byte) (need bool) {
	ha := btc.NewUint256(h)
	idx := ha.BIdx()
	mutex.Lock()
	if _, ok := pendingBlocks[idx]; ok {
		InvsIgnored++
	} else if _, ok := receivedBlocks[idx]; ok {
		InvsIgnored++
	} else if len(pendingFifo)<PendingFifoLen {
		pendingBlocks[idx] = ha
		pendingFifo <- idx
		need = true
	} else {
		FifoFullCnt++
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


func load_wallet(fn string) {
	if fn != "" {
		fmt.Println("Switching to wallet from file", fn)
		MyWallet = NewWallet(fn)
	} else {
		MyWallet = NewWallet(GocoinHomeDir+"wallet.txt")
	}
	if MyWallet == nil {
		fmt.Println("You have no wallet")
		return
	}
	fmt.Println("Dumping wallet:")
	for i := range MyWallet.addrs {
		fmt.Println(" ", MyWallet.addrs[i].String(), MyWallet.label[i])
	}
}

func load_tx(par string) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Something went wrong, but recovered in f", r)
		}
	}()
	
	f, e := os.Open(par)
	if e != nil {
		println(e.Error())
		return
	}
	n, _ := f.Seek(0, os.SEEK_END)
	f.Seek(0, os.SEEK_SET)
	buf := make([]byte, n)
	f.Read(buf)
	f.Close()

	txd, er := hex.DecodeString(string(buf))
	if er != nil {
		txd = buf
		fmt.Println("Seems like the transaction is in a binary format")
	} else {
		fmt.Println("Looks like the transaction file contains hex data")
	}

	// At this place we should have raw transaction in txd
	tx, le := btc.NewTx(txd)
	if le != len(txd) {
		fmt.Println("WARNING: Tx length mismatch", le, len(txd))
	}
	txid := btc.NewSha2Hash(txd)
	fmt.Println(len(tx.TxIn), "Inputs:")
	var totinp, totout uint64
	var missinginp bool
	for i := range tx.TxIn {
		fmt.Printf(" %3d %s", i, tx.TxIn[i].Input.String())
		po, _ := BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
		if po != nil {
			totinp += po.Value
			fmt.Printf(" %15.8f BTC @ %s\n", float64(po.Value)/1e8,
				btc.NewAddrFromPkScript(po.Pk_script, AddrVersion).String())
		} else {
			fmt.Println(" * no such unspent in the blockchain *")
			missinginp = true
		}
	}
	fmt.Println(len(tx.TxOut), "Outputs:")
	for i := range tx.TxOut {
		totout += tx.TxOut[i].Value
		fmt.Printf(" %15.8f BTC to %s\n", float64(tx.TxOut[i].Value)/1e8,
			btc.NewAddrFromPkScript(tx.TxOut[i].Pk_script, AddrVersion).String())
	}
	if missinginp {
		fmt.Println("WARNING: There are missing inputs, so you cannot calc input BTC amount")
	} else {
		fmt.Printf("%.8f BTC in -> %.8f BTC out, with %.8f BTC fee\n", float64(totinp)/1e8,
			float64(totout)/1e8, float64(totinp-totout)/1e8)
	}
	TransactionsToSend[txid.Hash] = txd
	fmt.Println("Transaction stored in the memory pool")
	fmt.Println("To brodcast it, execute: stx " + txid.String())
}


func send_tx(par string) {
	txid := btc.NewUint256FromString(par)
	if txid==nil {
		fmt.Println("You must specify a valid transaction ID for this command.")
		list_txs("")
		return
	}
	if _, ok := TransactionsToSend[txid.Hash]; !ok {
		fmt.Println("No such transaction ID in the memory pool.")
		list_txs("")
		return
	}
	cnt := NetSendInv(1, txid.Hash[:], nil)
	fmt.Println("Transaction", txid.String(), "broadcasted to", cnt, "node(s)")
	fmt.Println("If it does not appear in the chain, you may want to redo it.")
}


func list_txs(par string) {
	fmt.Println("Transactions in the memory pool:")
	cnt := 0
	for k, v := range TransactionsToSend {
		fmt.Println(cnt, btc.NewUint256(k[:]).String(), "-", len(v), "bytes")
	}
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
	newUi("balance bal", true, show_balance, "Show & save the balance of the currently loaded wallet")
	newUi("unspent u", true, list_unspent, "Shows unpent outputs for a given address")
	newUi("loadtx tx", true, load_tx, "Load transaction data from the given file, decode it and store in memory")
	newUi("sendtx stx", true, send_tx, "Broadcast transaction from memory pool, identified given <txid>")
	newUi("lstx", true, list_txs, "List all the transaction loaded into memory pool")
	newUi("wallet wal", true, load_wallet, "Load wallet from given file (or re-load the last one) and display its addrs")
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

	if flag.Lookup("h") != nil {
		flag.PrintDefaults()
		os.Exit(0)
	}
	flag.Parse()

	UploadLimit = *maxul << 10
	DownloadLimit = *maxdl << 10

	host_init()

	MyWallet = NewWallet(GocoinHomeDir+"wallet.txt")
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
		if retryCachedBlocks {
			Busy("retry_cached_blocks 1")
			retryCachedBlocks = retry_cached_blocks()
		}

		Busy("")
		select {
			case newbl = <-netBlocks:
				break
			
			case cmd := <-uiChannel:
				Busy("UI command")
				UiMsgsCnt++
				cmd.handler(cmd.param)
				uicmddone <- true
				continue
			
			case <-time.After(100*time.Millisecond):
				TicksCnt++
				if !retryCachedBlocks {
					Busy("BlockChain.Idle()")
					BlockChain.Idle()
				}
				continue
		}

		NetMsgsCnt++

		bl := newbl.bl

		Busy("CheckBlock "+bl.Hash.String())
		e, dos, maybelater := BlockChain.CheckBlock(bl)
		if e != nil {
			if maybelater {
				cachedBlocks[bl.Hash.BIdx()] = bl
			} else {
				println(dos, e.Error())
				if dos {
					newbl.conn.DoS()
				}
			}
		} else {
			Busy("AcceptBlock "+bl.Hash.String())
			e = BlockChain.AcceptBlock(bl)
			if e == nil {
				// block accepted, so route this inv to peers
				Busy("NetSendInv")
				NetSendInv(2, bl.Hash.Hash[:], newbl.conn)
				if beep {
					print("\007")
				}
				Busy("retry_cached_blocks 2")
				retryCachedBlocks = retry_cached_blocks()
				mutex.Lock()
				LastBlock = BlockChain.BlockTreeEnd
				LastBlockReceived = time.Now().Unix()
				mutex.Unlock()
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
