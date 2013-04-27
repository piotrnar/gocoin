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
	"github.com/piotrnar/gocoin/btc"
	_ "github.com/piotrnar/gocoin/btc/qdb"
)

const (
	InvsAskDuration = 2*time.Second
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

	dbg uint64

	LastBlock *btc.BlockTreeNode
	
	mutex sync.Mutex

	pendingBlocks map[[btc.Uint256IdxLen]byte] *btc.Uint256 = make(map[[btc.Uint256IdxLen]byte] *btc.Uint256, 600)
	pendingFifo chan [btc.Uint256IdxLen]byte = make(chan [btc.Uint256IdxLen]byte, 1000)
	
	cachedBlocks map[[btc.Uint256IdxLen]byte] *btc.Block = make(map[[btc.Uint256IdxLen]byte] *btc.Block)
	receivedBlocks map[[btc.Uint256IdxLen]byte] int64 = make(map[[btc.Uint256IdxLen]byte] int64, 300e3)

	MyWallet *oneWallet

	nextInvAsk time.Time = time.Now()

	InvsIgnored, BlockDups, InvsAsked uint32
)


func init_blockchain() {
	fmt.Println("Opening blockchain...")
	sta := time.Now().UnixNano()
	BlockChain = btc.NewChain(dbdir, GenesisBlock, *rescan)
	sto := time.Now().UnixNano()
	fmt.Printf("Blockchain open in %.3f seconds\n", float64(sto-sta)/1e9)
}


func do_userif() {
	for {
		time.Sleep(250e6)
		fmt.Print("> ")
		li, _, _ := bufio.NewReader(os.Stdin).ReadLine()
		if len(li) > 0 {
			c := new(command)
			c.src = "ui"
			c.str = string(li[:])
			cmdChannel <- c
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


func show_stats() {
	mutex.Lock()
	fmt.Printf("cachedBlocks:%d  pendingBlocks:%d/%d  receivedBlocks:%d\n", 
		len(cachedBlocks), len(pendingBlocks), len(pendingFifo), len(receivedBlocks))
	fmt.Printf("InvsIgn:%d  BlockDups:%d  InvsAsked:%d\n", 
		InvsIgnored, BlockDups, InvsAsked)
	mutex.Unlock()
	fmt.Println(BlockChain.Stats())
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
	unsp := BlockChain.GetAllUnspent(MyWallet.addrs)
	var sum uint64
	for i := range unsp {
		fmt.Printf("%7d %s @ %s\n", 1+BlockChain.BlockTreeEnd.Height-unsp[i].MinedAt,
			unsp[i].String(), MyWallet.addrs[unsp[i].AskIndex].String())
		sum += unsp[i].Value
	}
	fmt.Printf("%.8f BTC @ all %d addresses\n", float64(sum)/1e8, len(MyWallet.addrs))
}


func retry_cached_blocks() bool {
	if len(cachedBlocks)==0 {
		return false
	}
	if len(cmdChannel) > 0 {
		return true
	}
	for k, v := range cachedBlocks {
		e := BlockChain.AcceptBlock(v)
		if e == nil {
			//println("*** Old block accepted", BlockChain.BlockTreeEnd.Height)
			delete(cachedBlocks, k)
			LastBlock = BlockChain.BlockTreeEnd
			return len(cachedBlocks)>0
		} else if e.Error()!=btc.ErrParentNotFound {
			println(e.Error())
			show_stats()
			show_cached()
			os.Exit(0)
			delete(cachedBlocks, k)
			return len(cachedBlocks)>0
		}
	}
	return false
}



func blocksNeeded() (res []byte) {
	mutex.Lock()
	if time.Now().After(nextInvAsk) {
		if len(pendingBlocks)==0 {
			InvsAsked++
			res = LastBlock.BlockHash.Hash[:]
		}
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


func InvsNotify(h []byte) {
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
	}
	mutex.Unlock()
}


func show_help() {
	fmt.Println("There are different commands...")
	fmt.Println("bal, unspent <address>, info, mem, prof, invs, cach, pers, net, quit")
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
	println("receivedBlocks:", len(receivedBlocks))
	
	go network_process()
	go do_userif()

	if LastBlock.Height > 10000 {
		sta = 0
	}
	for {
		if retryCachedBlocks {
			retryCachedBlocks = retry_cached_blocks()
		}

		msg, ok := <- cmdChannel
		if !ok {
			if !retryCachedBlocks {
				BlockChain.Idle()
			}
			continue
		}
		//println("got msg", msg.src)
		if msg.src=="ui" {
			if strings.HasPrefix(msg.str, "unspent") {
				list_unspent(strings.Trim(msg.str[7:], " "))
			} else if strings.HasPrefix(msg.str, "u ") {
				list_unspent(strings.Trim(msg.str[2:], " "))
			} else if strings.HasPrefix(msg.str, "dbg ") {
				dbg, _ = strconv.ParseUint(msg.str[4:], 10, 64)
				println("dbg:", dbg)
			} else {
				sta := time.Now().UnixNano()
				switch msg.str {
					case "i": 
						show_stats()

					case "info": 
						show_stats()
					
					case "bal": 
						show_balance()

					case "cach": 
						show_cached()
					
					case "invs": 
						show_invs()
					
					case "net":
						net_stats()

					case "prof": 
						btc.ShowProfileData()
					
					case "save": 
						fmt.Println("Saving coinbase...")
						BlockChain.Save()
					
					case "quit": 
						goto exit
					
					case "q": 
						os.Exit(0)
					
					case "mem":
						var ms runtime.MemStats
						runtime.ReadMemStats(&ms)
						fmt.Println("HeapAlloc", ms.HeapAlloc>>20, "MB")
					
					case "pers":
						show_addresses()
					
					case "?":
						show_help()
					
					case "h":
						show_help()
					
					default:
						println("unknown command")
				}
				sto := time.Now().UnixNano()
				fmt.Printf("Ready in %.3fs\n", float64(sto-sta)/1e9)
			}
		} else if msg.src=="net" {
			switch msg.str {
				case "bl":
					bl, e := btc.NewBlock(msg.dat[:])
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
							break
						}
						receivedBlocks[idx] = time.Now().UnixNano()
						delete(pendingBlocks, idx)
						mutex.Unlock()
						e = bl.CheckBlock()
						if e == nil {
							e = BlockChain.AcceptBlock(bl)
							if e == nil {
								//println(BlockChain.BlockTreeEnd.Height, "\007")
								retryCachedBlocks = retry_cached_blocks()
								mutex.Lock()
								LastBlock = BlockChain.BlockTreeEnd
								mutex.Unlock()
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
					} else {
						println("NewBlock:", e.Error())
					}
			}
		}

		if sta!=0 && LastBlock.Height>=10000 {
			sto := time.Now().Unix()
			println("Block", LastBlock.Height, "reached after", sto-sta, "seconds")
			sta = 0
			//goto exit
		}
	}
exit:
	println("Closing blockchain")
	BlockChain.Close()
	peerDB.Close()
}
