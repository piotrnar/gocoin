package main

import (
	"fmt"
	"os"
	"flag"
	"time"
	"bufio"
//	"bytes"
	"sync"
	"runtime"
	"strings"
	"strconv"
	"encoding/hex"
//	"encoding/binary"
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

	dbdir string
	
	GenesisBlock *btc.Uint256
	Magic [4]byte
	BlockChain *btc.Chain

	exit_now bool

	dbg uint64
	beep bool

	LastBlock *btc.BlockTreeNode
	lastInvAsked  *btc.BlockTreeNode
	disableSync time.Time

	mutex sync.Mutex
	uicmddone chan bool = make(chan bool, 1)
	netChannel chan *NetCommand = make(chan *NetCommand, 100)
	uiChannel chan string = make(chan string, 1)

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


func init_blockchain() {
	fmt.Println("Opening blockchain...")
	sta := time.Now().UnixNano()
	BlockChain = btc.NewChain(dbdir, GenesisBlock, *rescan)
	sto := time.Now().UnixNano()
	fmt.Printf("Blockchain open in %.3f seconds\n", float64(sto-sta)/1e9)
}

func Busy(b string) {
	mutex.Lock()
	busy = b
	mutex.Unlock()
}


func do_userif() {
	var prompt bool = true
	time.Sleep(1e9)
	for {
		if prompt {
			fmt.Print("> ")
		}
		li, _, _ := bufio.NewReader(os.Stdin).ReadLine()
		if len(li) > 0 {
			cmd := string(li[:])
			prompt = true
			switch cmd {
				case "i":
					show_info()
				
				case "mem":
					var ms runtime.MemStats
					runtime.ReadMemStats(&ms)
					fmt.Println("HeapAlloc", ms.HeapAlloc>>20, "MB")
				
				case "cach": 
					show_cached()
				
				case "invs": 
					show_invs()
				
				case "net":
					net_stats()

				case "beep":
					beep = !beep
					println("beep", beep)

				case "?":
					show_help()
				
				case "h":
					show_help()
				
				case "q":
					os.Exit(0)
				
				case "pers":
					show_addresses()
					
				default:
					prompt = false
					mutex.Lock()
					if busy!="" {
						print("now busy with ", busy)
					}
					mutex.Unlock()
					println("...")
					sta := time.Now().UnixNano()
					uiChannel <- cmd
					go func() {
						_ = <- uicmddone
						sto := time.Now().UnixNano()
						fmt.Printf("Ready in %.3fs\n", float64(sto-sta)/1e9)
						fmt.Print("> ")
					}()
			}
		}
	}
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
	unsp := BlockChain.GetAllUnspent(a[:])
	var sum uint64
	for i := range unsp {
		fmt.Println(unsp[i].String())
		sum += unsp[i].Value
	}
	fmt.Printf("Total %.8f unspent BTC at address %s\n", float64(sum)/1e8, a[0].String());
}


func show_info() {
	mutex.Lock()
	fmt.Printf("cachedBlocks:%d  pendingBlocks:%d/%d  receivedBlocks:%d\n", 
		len(cachedBlocks), len(pendingBlocks), len(pendingFifo), len(receivedBlocks))
	fmt.Printf("InvsIgn:%d  BlockDups:%d  InvsAsked:%d  NetMsgs:%d  UiMsgs:%d  Ticks:%d\n", 
		InvsIgnored, BlockDups, InvsAsked, NetMsgsCnt, UiMsgsCnt, TicksCnt)
	fmt.Println("LastBlock:", LastBlock.Height, LastBlock.BlockHash.String())
	if busy!="" {
		println("Currently busy with", busy)
	} else {
		println("Not busy")
	}
	mutex.Unlock()
}


func show_invs() {
	mutex.Lock()
	fmt.Println(len(pendingBlocks), "pending invs")
	for _, v := range pendingBlocks {
		fmt.Println(v.String())
	}
	mutex.Unlock()
}


func show_cached() {
	for _, v := range cachedBlocks {
		fmt.Printf(" * %s -> %s\n", v.Hash.String(), btc.NewUint256(v.Parent).String())
	}
}


func show_balance() {
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

	unsp := BlockChain.GetAllUnspent(MyWallet.addrs)
	var sum uint64
	for i := range unsp {
		if utxt != nil {
			txid := btc.NewUint256(unsp[i].TxPrevOut.Hash[:])
			fmt.Fprintln(utxt, unsp[i].TxPrevOut.String())
			po, e := BlockChain.Unspent.UnspentGet(&unsp[i].TxPrevOut)
			if e == nil {
				fn := "balance/"+txid.String()[:64]+".tx"
				txf, _ := os.Open(fn)
				if txf != nil {
					println(fn, "already done")
					txf.Close()
				} else {
					txf, _ = os.Create(fn)
					if txf != nil {
						n := BlockChain.BlockTreeEnd
						for n.Height > po.BlockHeight {
							n = n.Parent
						}
						bd, _, e := BlockChain.Blocks.BlockGet(n.BlockHash)
						if e == nil {
							bl, e := btc.NewBlock(bd)
							if e == nil {
								e = bl.BuildTxList()
								if e == nil {
									for i := range bl.Txs {
										if bl.Txs[i].Hash.Equal(txid) {
											txf.Write(bl.Txs[i].Serialize())
											break
										}
									}
								} else {
									println("BuildTxList: ", e.Error())
								}
							} else {
								println("NewBlock: ", e.Error())
							}
						} else {
							println("BlockGet: ", e.Error())
						}
						txf.Close()
					}
				}
			} else {
				println(e.Error())
			}
		}
		if len(unsp)<100 {
			fmt.Printf("%7d %s @ %s\n", 1+BlockChain.BlockTreeEnd.Height-unsp[i].MinedAt,
				unsp[i].String(), MyWallet.addrs[unsp[i].AskIndex].String())
		}
		sum += unsp[i].Value
	}
	fmt.Printf("%.8f BTC in total, in %d unspent outputs\n", float64(sum)/1e8, len(unsp))
	if utxt != nil {
		fmt.Println("Your balance data has been saved to the balance folder.")
		fmt.Println("You can move this folder now to your offline wallet PC.")
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
			snoozeDisableSync(5)
			return len(cachedBlocks)>0
		} else if e.Error()!=btc.ErrParentNotFound {
			panic(e.Error())
			delete(cachedBlocks, k)
			return len(cachedBlocks)>0
		}
	}
	return false
}

func snoozeDisableSync(sec int) {
	if BlockChain.DoNotSync {
		disableSync = time.Now().Add(time.Duration(sec)*time.Second)
	}
}

func blocksNeeded() (res []byte) {
	mutex.Lock()
	if lastInvAsked != LastBlock || time.Now().After(nextInvAsk) {
		lastInvAsked = LastBlock
		InvsAsked++
		res = LastBlock.BlockHash.Hash[:]
		nextInvAsk = time.Now().Add(InvsAskDuration)
	}
	mutex.Unlock()
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


func do_ui_request(str string) {
	Busy("User Interface: "+str)
	UiMsgsCnt++
	if strings.HasPrefix(str, "unspent") {
		list_unspent(strings.Trim(str[7:], " "))
	} else if strings.HasPrefix(str, "u ") {
		list_unspent(strings.Trim(str[2:], " "))
	} else if strings.HasPrefix(str, "tx ") {
		tx, er := hex.DecodeString(str[3:])
		if er != nil {
			println(er.Error())
		}
		println("broadcast tx len", len(tx))
		h := btc.Sha2Sum(tx)
		TransactionsToSend[h] = tx
		NetSendInv(1, h[:], nil)
	} else if strings.HasPrefix(str, "dbg ") {
		dbg, _ = strconv.ParseUint(str[4:], 10, 64)
		println("dbg:", dbg)
	} else if strings.HasPrefix(str, "wal ") {
		println("Switching to wallet from file", str[4:])
		MyWallet = NewWallet(str[4:])
	} else {
		switch str {
			case "b": 
				fmt.Println(BlockChain.Stats())

			case "bal": 
				show_balance()

			case "prof": 
				btc.ShowProfileData()
			
			case "save": 
				fmt.Println("Saving coinbase...")
				BlockChain.Save()
			
			case "quit": 
				exit_now = true
				return
			
			default:
				println("unknown command")
		}
	}
	uicmddone <- true
}


func GetBlockData(h []byte) []byte {
	bl, _, e  := BlockChain.Blocks.BlockGet(btc.NewUint256(h))
	if e == nil {
		return bl
	}
	println("BlockChain.Blocks.BlockGet failed")
	return nil
}


func show_help() {
	fmt.Println("There are different commands... some of them:")
	fmt.Println("i - show general info")
	fmt.Println("mem - show memory usage info")
	fmt.Println("b - show bockchain stats")
	fmt.Println("bal - show balance of your wallet")
	fmt.Println("u <address> - show unspent outputs of the given address")
	fmt.Println("wal <filename> - load a differetn wallet")
	fmt.Println("prof, invs, cach, pers, net, quit ...")
}


func main() {
	var sta int64
	var retryCachedBlocks bool

	if flag.Lookup("h") != nil {
		flag.PrintDefaults()
		os.Exit(0)
	}
	flag.Parse()

	if *testnet { // testnet3
		GenesisBlock = btc.NewUint256FromString("000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943")
		Magic = [4]byte{0x0B,0x11,0x09,0x07}
		dbdir = "/btc/database/tstnet/"
	} else {
		GenesisBlock = btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
		Magic = [4]byte{0xF9,0xBE,0xB4,0xD9}
		dbdir = "/btc/database/btcnet/"
	}
	
	init_blockchain()
	MyWallet = NewWallet(dbdir+"wallet.txt")
	initPeers(dbdir)

	LastBlock = BlockChain.BlockTreeEnd
	
	sta = time.Now().Unix()
	for k, _ := range BlockChain.BlockIndex {
		receivedBlocks[k] = sta
	}
	
	go network_process()
	go do_userif()

	var netmsg *NetCommand
	for !exit_now {
		//println(BlockChain.DoNotSync, retryCachedBlocks)
		if retryCachedBlocks {
			Busy("retry_cached_blocks")
			retryCachedBlocks = retry_cached_blocks()
		}

		Busy("")
		select {
			case netmsg = <-netChannel:
				break
			
			case cmd := <-uiChannel:
				do_ui_request(cmd)
				continue
			
			case <-time.After(100*time.Millisecond):
				TicksCnt++
				if !retryCachedBlocks {
					if BlockChain.DoNotSync && time.Now().After(disableSync) {
						sto := time.Now().Unix()
						println("Blocks stopped comming - enable disk sync")
						println("Block", LastBlock.Height, "reached after", sto-sta, "seconds")
						BlockChain.Sync()
					}

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
						if !BlockChain.DoNotSync && len(pendingBlocks)>50 {
							BlockChain.DoNotSync = true
							println("lots of pending blocks - switch syncing off for now...")
							snoozeDisableSync(5)
						}

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
							mutex.Unlock()
							snoozeDisableSync(5)
						} else if e.Error()==btc.ErrParentNotFound {
							cachedBlocks[bl.Hash.BIdx()] = bl
							//println("Store block", bl.Hash.String(), "->", bl.GetParent().String(), "for later", len(blocksWithNoParent))
						} else {
							println("AcceptBlock:", e.Error())
						}
					} else {
						println("CheckBlock:", e.Error(), LastBlock.Height)
						show_invs()
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
