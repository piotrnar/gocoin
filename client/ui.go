package main

import (
	"os"
	"fmt"
	"time"
	"sort"
	"bufio"
	"strings"
	"strconv"
	"runtime"
	"runtime/debug"
	"github.com/piotrnar/gocoin/btc"
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
	time.Sleep(5e8)
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
							uiChannel <- oneUiReq{param:param, handler:uiCmds[i].handler}
							go func() {
								_ = <- uicmddone
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
	// Memory used
	var ms runtime.MemStats
	var gs debug.GCStats
	runtime.ReadMemStats(&ms)
	fmt.Println("Go version:", runtime.Version(),
		"   Heap size:", ms.Alloc>>20, "MB",
		"   Sys mem used", ms.Sys>>20, "MB",
		"   NewBlockBeep:", beep)

	debug.ReadGCStats(&gs)
	fmt.Println("LastGC:", time.Now().Sub(gs.LastGC).String(),
		"   NumGC:", gs.NumGC,
		"   PauseTotal:", gs.PauseTotal.String())

	mutex.Lock()
	// Main thread activity:
	if busy!="" {
		fmt.Println("BlockChain thread is busy with", busy)
	} else {
		fmt.Println("BlockChain thread is currently idle")
	}
	mutex.Unlock()
	fmt.Println("Nonde's uptime:", time.Now().Sub(StartTime).String())
}


// The last block:
func show_last(par string) {
	mutex.Lock()
	fmt.Println("LastBlock:", LastBlock.BlockHash.String())
	fmt.Printf("  Height: %d @ %s,  Difficulty: %.1f\n", LastBlock.Height,
		time.Unix(int64(LastBlock.Timestamp), 0).Format("2006/01/02 15:04:05"),
		btc.GetDifficulty(LastBlock.Bits))
	fmt.Println("  got", time.Now().Sub(LastBlockReceived), "ago")
	mutex.Unlock()
}


func show_counters(par string) {
	mutex.Lock()
	fmt.Printf("BlocksCached: %d,   BlocksPending: %d/%d,   NetQueueSize: %d,   NetConns: %d\n",
		len(cachedBlocks), len(pendingBlocks), len(pendingFifo), len(netBlocks), len(openCons))
	mutex.Unlock()

	counter_mutex.Lock()
	ck := make([]string, len(Counter))
	i := 0
	for k, _ := range Counter {
		ck[i] = k
		i++
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


func ui_beep(par string) {
	if par=="1" || par=="on" || par=="true" {
		beep = true
	} else if par=="0" || par=="off" || par=="false" {
		beep = false
	}
	fmt.Println("beep:", beep)
}


func ui_dbg(par string) {
	v, e := strconv.ParseInt(par, 10, 32)
	if e == nil {
		dbg = v
	}
	fmt.Println("dbg:", dbg)
}


func show_invs(par string) {
	mutex.Lock()
	fmt.Println(len(pendingBlocks), "pending invs")
	for _, v := range pendingBlocks {
		fmt.Println(v.String())
	}
	mutex.Unlock()
}


func show_cached(par string) {
	for _, v := range cachedBlocks {
		fmt.Printf(" * %s -> %s\n", v.Hash.String(), btc.NewUint256(v.Parent).String())
	}
}


func show_help(par string) {
	fmt.Println("There following", len(uiCmds), "commands are supported:")
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
	if p=="free" {
		fmt.Println("Freeing the mem...")
		debug.FreeOSMemory()
		show_mem("")
	}
}


func dump_block(s string) {
	h := btc.NewUint256FromString(s)
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


func init() {
	newUi("help h ?", false, show_help, "Shows this help")
	newUi("info i", false, show_info, "Shows general info")
	newUi("last l", false, show_last, "Show last block info")
	newUi("counters c", false, show_counters, "Show counters")
	newUi("mem", false, show_mem, "Show detailed memory stats")
	newUi("beep", false, ui_beep, "Control beep when a new block is received (use param 0 or 1)")
	newUi("dbg d", false, ui_dbg, "Control debugs (use numeric parameter)")
	newUi("cach", false, show_cached, "Show blocks cached in memory")
	newUi("invs", false, show_invs, "Show pending block inv's (ones waiting for data)")
	newUi("savebl", false, dump_block, "Saves a block with a given hash to a binary file")
}
