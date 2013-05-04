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
	InvsAskDuration = 10*time.Second
)

var (
	//host *string = flag.String("c", "blockchain.info:8333", "Connect to specified host")
	//listen *bool = flag.Bool("l", false, "Listen insted of connecting")
	verbose *bool = flag.Bool("v", false, "Verbose mode")
	testnet *bool = flag.Bool("t", false, "Use Testnet3")
	rescan *bool = flag.Bool("rescan", false, "Rescan unspent outputs (not scripts)")
	proxy *string = flag.String("c", "", "Connect to this host")
	server *bool = flag.Bool("server", false, "Enable TCP server (allow incomming connections)")
	datadir *string = flag.String("datadir", "", "Specify Gocoin's database root folder")

	GenesisBlock *btc.Uint256
	Magic [4]byte
	BlockChain *btc.Chain
	AddrVersion byte

	exit_now bool

	dbg uint64
	beep bool

	LastBlock *btc.BlockTreeNode
	LastBlockReceived int64 // time when the last block was received
	lastInvAsked  *btc.BlockTreeNode

	mutex sync.Mutex
	uicmddone chan bool = make(chan bool, 1)
	netChannel chan *NetCommand = make(chan *NetCommand, 100)
	uiChannel chan oneUiReq = make(chan oneUiReq, 1)

	pendingBlocks map[[btc.Uint256IdxLen]byte] *btc.Uint256 = make(map[[btc.Uint256IdxLen]byte] *btc.Uint256, 600)
	pendingFifo chan [btc.Uint256IdxLen]byte = make(chan [btc.Uint256IdxLen]byte, 1000)
	
	cachedBlocks map[[btc.Uint256IdxLen]byte] *btc.Block = make(map[[btc.Uint256IdxLen]byte] *btc.Block)
	receivedBlocks map[[btc.Uint256IdxLen]byte] int64 = make(map[[btc.Uint256IdxLen]byte] int64, 300e3)

	MyWallet *oneWallet

	nextInvAsk time.Time = time.Now()

	InvsIgnored, BlockDups, InvsAsked, NetMsgsCnt, UiMsgsCnt uint64
	TicksCnt uint64
	busy string

	TransactionsToSend map[[32]byte] []byte = make(map[[32]byte] []byte)
)


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
	if len(netChannel) > 0 {
		return true
	}
	for k, v := range cachedBlocks {
		e := BlockChain.AcceptBlock(v)
		if e == nil {
			//println("*** Old block accepted", BlockChain.BlockTreeEnd.Height)
			delete(cachedBlocks, k)
			LastBlock = BlockChain.BlockTreeEnd
			LastBlockReceived = time.Now().Unix()
			return len(cachedBlocks)>0
		} else if e.Error()!=btc.ErrParentNotFound {
			panic(e.Error())
			delete(cachedBlocks, k)
			return len(cachedBlocks)>0
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

func blocksNeeded() (res []byte) {
	mutex.Lock()
	if lastInvAsked != LastBlock || time.Now().After(nextInvAsk) {
		lastInvAsked = LastBlock
		InvsAsked++
		BlockChain.BlockIndexAccess.Lock()
		var depth = 144 // by default let's ask up to 
		if LastBlockReceived != 0 {
			// Every minute from last block reception moves us 1-block up the chain
			depth = int((time.Now().Unix() - LastBlockReceived) / 60)
			if depth>400 {
				depth = 400
			}
		}
		// ask N-blocks up in the chain, allowing to "recover" from chain fork
		n := LastBlock
		for i:=0; i<depth && n.Parent != nil; i++ {
			n = n.Parent
		}
		BlockChain.BlockIndexAccess.Unlock()
		res = n.BlockHash.Hash[:]
		nextInvAsk = time.Now().Add(InvsAskDuration)
	}
	mutex.Unlock()

	/*
	if res != nil {
		println("Last:", btc.NewUint256(res).String())
		BlockChain.BlockIndexAccess.Lock()
		findAllLeafes(BlockChain.BlockTreeRoot)
		BlockChain.BlockIndexAccess.Unlock()
	}
	*/

	return
}


func blockDataNeeded() ([]byte) {
	for len(pendingFifo)>0 {
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
	} else {
		pendingBlocks[idx] = ha
		pendingFifo <- idx
		need = true
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
	txd, er := hex.DecodeString(par)
	if er != nil {
		println(er.Error())
	}
	tx, le := btc.NewTx(txd)
	if le != len(txd) {
		fmt.Println("WARNING: Tx length mismatch", le, len(txd))
	}
	txid := btc.NewSha2Hash(txd)
	fmt.Println(len(tx.TxIn), "inputs:")
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
	fmt.Println(len(tx.TxOut), "outputs:")
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
	fmt.Println("Transaction", txid.String(), "stored in the memory pool")
	fmt.Println("Execute 'txs" + txid.String() + "' if you want to send it")
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
	newUi("loadtx tx", true, load_tx, "Decode given hex-encoded tx and store it in memory pool")
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

	var netmsg *NetCommand
	for !exit_now {
		if retryCachedBlocks {
			Busy("retry_cached_blocks")
			retryCachedBlocks = retry_cached_blocks()
		}

		Busy("")
		select {
			case netmsg = <-netChannel:
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
		if netmsg.cmd=="bl" {
			Busy("NewBlock")
			bl, e := btc.NewBlock(netmsg.dat[:])
			if e == nil {
				idx := bl.Hash.BIdx()
				mutex.Lock()
				if _, got := receivedBlocks[idx]; got {
					if _, ok := pendingBlocks[idx]; ok {
						panic("wtf?")
					} else {
						BlockDups++
					}
					mutex.Unlock()
				} else {
					receivedBlocks[idx] = time.Now().UnixNano()
					delete(pendingBlocks, idx)
					mutex.Unlock()
					
					Busy("CheckBlock "+bl.Hash.String())
					e = bl.CheckBlock()
					if e == nil {
						Busy("AcceptBlock "+bl.Hash.String())
						e = BlockChain.AcceptBlock(bl)
						if e == nil {
							// block accepted, so route this inv to peers
							NetSendInv(2, bl.Hash.Hash[:], netmsg.conn)
							if beep {
								print("\007")
							}
							retryCachedBlocks = retry_cached_blocks()
							mutex.Lock()
							LastBlock = BlockChain.BlockTreeEnd
							LastBlockReceived = time.Now().Unix()
							mutex.Unlock()
						} else if e.Error()==btc.ErrParentNotFound {
							cachedBlocks[bl.Hash.BIdx()] = bl
							//println("Store block", bl.Hash.String(), "->", bl.GetParent().String(), "for later", len(blocksWithNoParent))
						} else {
							println("AcceptBlock:", e.Error())
						}
					} else {
						println("CheckBlock:", e.Error(), LastBlock.Height)
					}
				}
			} else {
				println("NewBlock:", e.Error())
			}
		}
	}
	println("Closing blockchain")
	BlockChain.Close()
	peerDB.Close()
}
