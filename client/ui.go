package main

import (
	"os"
	"fmt"
	"time"
	"sort"
	"sync"
	"bufio"
	"strings"
	"strconv"
	"runtime"
	"runtime/debug"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/qdb"
)

type oneUiCmd struct {
	cmds []string // command name
	help string // a helf for this command
	sync bool  // shall be executed in the blochcina therad
	handler func(pars string)
}

type oneUiReq struct {
	param string
	handler func(pars string)
	done sync.WaitGroup
}

var uiCmds []*oneUiCmd


// add a new UI commend handler
func newUi(cmds string, sync bool, hn func(string), help string) {
	cs := strings.Split(cmds, " ")
	if len(cs[0])>0 {
		var c = new(oneUiCmd)
		for i := range cs {
			c.cmds = append(c.cmds, cs[i])
		}
		c.sync = sync
		c.help = help
		c.handler = hn
		if len(uiCmds)>0 {
			var i int
			for i = 0; i<len(uiCmds); i++ {
				if uiCmds[i].cmds[0]>c.cmds[0] {
					break // lets have them sorted
				}
			}
			tmp := make([]*oneUiCmd, len(uiCmds)+1)
			copy(tmp[:i], uiCmds[:i])
			tmp[i] = c
			copy(tmp[i+1:], uiCmds[i:])
			uiCmds = tmp
		} else {
			uiCmds = []*oneUiCmd{c}
		}
	} else {
		panic("empty command string")
	}
}

func readline() string {
	li, _, _ := bufio.NewReader(os.Stdin).ReadLine()
	return string(li)
}


func ask_yes_no(msg string) bool {
	for {
		fmt.Print(msg, " (y/n) : ")
		l := strings.ToLower(readline())
		if l=="y" {
			return true
		} else if l=="n" {
			return false
		}
	}
	return false
}


func ui_show_prompt() {
	fmt.Print("> ")
}


func do_userif() {
	var prompt bool = true
	time.Sleep(1e8)
	for {
		if prompt {
			ui_show_prompt()
		}
		prompt = true
		li := strings.Trim(readline(), " \n\t\r")
		if len(li) > 0 {
			cmdpar := strings.SplitN(li, " ", 2)
			cmd := cmdpar[0]
			param := ""
			if len(cmdpar)==2 {
				param = cmdpar[1]
			}
			found := false
			for i := range uiCmds {
				for j := range uiCmds[i].cmds {
					if cmd==uiCmds[i].cmds[j] {
						found = true
						if uiCmds[i].sync {
							mutex.Lock()
							if busy!="" {
								print("now busy with ", busy)
							}
							mutex.Unlock()
							println("...")
							sta := time.Now().UnixNano()
							req := &oneUiReq{param:param, handler:uiCmds[i].handler}
							req.done.Add(1)
							uiChannel <- req
							go func() {
								req.done.Wait()
								sto := time.Now().UnixNano()
								fmt.Printf("Ready in %.3fs\n", float64(sto-sta)/1e9)
								fmt.Print("> ")
							}()
							prompt = false
						} else {
							uiCmds[i].handler(param)
						}
					}
				}
			}
			if !found {
				fmt.Printf("Unknown command '%s'. Type 'help' for help.\n", cmd)
			}
		}
	}
}


func show_info(par string) {
	fmt.Println(busy)
	mutex.Lock()
	fmt.Println("LastBlock:", LastBlock.BlockHash.String())
	fmt.Printf("Height: %d @ %s,  Diff: %.0f,  Got: %s ago\n",
		LastBlock.Height,
		time.Unix(int64(LastBlock.Timestamp), 0).Format("2006/01/02 15:04:05"),
		btc.GetDifficulty(LastBlock.Bits), time.Now().Sub(LastBlockReceived).String())
	fmt.Printf("BlocksCached: %d,  NetQueueSize: %d,  NetConns: %d,  Peers: %d\n",
		len(cachedBlocks), len(netBlocks), len(openCons),
		peerDB.Count())
	mutex.Unlock()

	tx_mutex.Lock()
	fmt.Printf("TransactionsToSend:%d,  TransactionsRejected:%d,  TransactionsPending:%d/%d\n",
		len(TransactionsToSend), len(TransactionsRejected), len(TransactionsPending), len(netTxs))
	fmt.Printf("WaitingForInputs:%d,  SpentOutputs:%d\n",
		len(WaitingForInputs), len(SpentOutputs))
	tx_mutex.Unlock()

	bw_stats()

	// Memory used
	var ms runtime.MemStats
	var gs debug.GCStats
	runtime.ReadMemStats(&ms)
	fmt.Println("Go version:", runtime.Version(),
		"   Heap size:", ms.Alloc>>20, "MB",
		"   Sys mem used", ms.Sys>>20, "MB")

	debug.ReadGCStats(&gs)
	fmt.Println("LastGC:", time.Now().Sub(gs.LastGC).String(),
		"   NumGC:", gs.NumGC,
		"   PauseTotal:", gs.PauseTotal.String())

	fmt.Println("Gocoin:", btc.SourcesTag,
		"  Threads:", btc.UseThreads,
		"  Uptime:", time.Now().Sub(StartTime).String(),
		"  ECDSA cnt:", btc.EcdsaVerifyCnt)
}


func show_counters(par string) {
	counter_mutex.Lock()
	ck := make([]string, 0)
	for k, _ := range Counter {
		if par=="" || strings.HasPrefix(k, par) {
			ck = append(ck, k)
		}
	}
	sort.Strings(ck)

	var li string
	for i := range ck {
		k := ck[i]
		v := Counter[k]
		s := fmt.Sprint(k, ": ", v)
		if len(li)+len(s) >= 80 {
			fmt.Println(li)
			li = ""
		} else if li!="" {
			li += ",   "
		}
		li += s
	}
	if li != "" {
		fmt.Println(li)
	}
	counter_mutex.Unlock()
}


func ui_dbg(par string) {
	v, e := strconv.ParseInt(par, 10, 32)
	if e == nil {
		dbg = v
	}
	fmt.Println("dbg:", dbg)
}


func show_cached(par string) {
	for _, v := range cachedBlocks {
		fmt.Printf(" * %s -> %s\n", v.Hash.String(), btc.NewUint256(v.Parent).String())
	}
}


func show_help(par string) {
	fmt.Println("The following", len(uiCmds), "commands are supported:")
	for i := range uiCmds {
		fmt.Print("   ")
		for j := range uiCmds[i].cmds {
			if j>0 {
				fmt.Print(", ")
			}
			fmt.Print(uiCmds[i].cmds[j])
		}
		fmt.Println(" -", uiCmds[i].help)
	}
	fmt.Println("All the commands are case sensitive.")
}


func show_mem(p string) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	fmt.Println("Alloc       :", ms.Alloc)
	fmt.Println("TotalAlloc  :", ms.TotalAlloc)
	fmt.Println("Sys         :", ms.Sys)
	fmt.Println("Lookups     :", ms.Lookups)
	fmt.Println("Mallocs     :", ms.Mallocs)
	fmt.Println("Frees       :", ms.Frees)
	fmt.Println("HeapAlloc   :", ms.HeapAlloc)
	fmt.Println("HeapSys     :", ms.HeapSys)
	fmt.Println("HeapIdle    :", ms.HeapIdle)
	fmt.Println("HeapInuse   :", ms.HeapInuse)
	fmt.Println("HeapReleased:", ms.HeapReleased)
	fmt.Println("HeapObjects :", ms.HeapObjects)
	fmt.Println("StackInuse  :", ms.StackInuse)
	fmt.Println("StackSys    :", ms.StackSys)
	fmt.Println("MSpanInuse  :", ms.MSpanInuse)
	fmt.Println("MSpanSys    :", ms.MSpanSys)
	fmt.Println("MCacheInuse :", ms.MCacheInuse)
	fmt.Println("MCacheSys   :", ms.MCacheSys)
	fmt.Println("BuckHashSys :", ms.BuckHashSys)
	if p=="" {
		return
	}
	if p=="free" {
		fmt.Println("Freeing the mem...")
		debug.FreeOSMemory()
		show_mem("")
		return
	}
	if p=="gc" {
		fmt.Println("Running GC...")
		runtime.GC()
		fmt.Println("Done.")
		return
	}
	i, e := strconv.ParseInt(p, 10, 64)
	if e != nil {
		println(e.Error())
		return
	}
	debug.SetGCPercent(int(i))
	fmt.Println("GC treshold set to", i, "percent")
}


func dump_block(s string) {
	h := btc.NewUint256FromString(s)
	if h==nil {
		println("Specify block's hash")
		return
	}
	bl, _, e := BlockChain.Blocks.BlockGet(h)
	if e != nil {
		println(e.Error())
		return
	}
	fn := h.String()+".bin"
	f, e := os.Create(fn)
	if e != nil {
		println(e.Error())
		return
	}
	f.Write(bl)
	f.Close()
	fmt.Println("Block saved to file:", fn)
}


func ui_quit(par string) {
	exit_now = true
}

func blchain_stats(par string) {
	fmt.Println(BlockChain.Stats())
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

func qdb_stats(par string) {
	fmt.Print(qdb.GetStats())
}

func init() {
	newUi("help h ?", false, show_help, "Shows this help")
	newUi("info i", false, show_info, "Shows general info about the node")
	newUi("counters c", false, show_counters, "Show all kind of debug counters")
	newUi("mem", false, show_mem, "Show detailed memory stats (optionally free, gc or a numeric param)")
	newUi("dbg d", false, ui_dbg, "Control debugs (use numeric parameter)")
	newUi("cache", false, show_cached, "Show blocks cached in memory")
	newUi("savebl", false, dump_block, "Saves a block with a given hash to a binary file")
	newUi("bchain b", true, blchain_stats, "Display blockchain statistics")
	newUi("quit q", true, ui_quit, "Exit nicely, saving all files. Otherwise use Ctrl+C")
	newUi("unspent u", true, list_unspent, "Shows unpent outputs for a given address")
	newUi("sync", true, switch_sync, "Control sync of the database to disk")
	newUi("qdbstats qs", false, qdb_stats, "Show statistics of QDB engine")
}
