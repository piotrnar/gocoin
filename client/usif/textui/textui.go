package textui

import (
	"os"
	"fmt"
	"time"
	"sort"
	"bufio"
	"strings"
	"strconv"
	"runtime"
	"io/ioutil"
	"encoding/json"
	"runtime/debug"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/qdb"
	"github.com/piotrnar/gocoin/lib"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/client/usif"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/lib/others/peersdb"
)

type oneUiCmd struct {
	cmds []string // command name
	help string // a helf for this command
	sync bool  // shall be executed in the blochcina therad
	handler func(pars string)
}

var (
	uiCmds []*oneUiCmd
	show_prompt bool = true
)

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


func AskYesNo(msg string) bool {
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


func ShowPrompt() {
	fmt.Print("> ")
}


func MainThread() {
	time.Sleep(1e9) // hold on for 1 sencond before showing the show_prompt
	for {
		if show_prompt {
			ShowPrompt()
		}
		show_prompt = true
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
							usif.ExecUiReq(&usif.OneUiReq{Param:param, Handler:uiCmds[i].handler})
							show_prompt = false
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
	common.Busy_mutex.Lock()
	if common.BusyWith!="" {
		fmt.Println("Chain thread busy with:", common.BusyWith)
	} else {
		fmt.Println("Chain thread is idle")
	}
	common.Busy_mutex.Unlock()

	common.Last.Mutex.Lock()
	fmt.Println("Last Block:", common.Last.Block.BlockHash.String())
	fmt.Printf("Height: %d @ %s,  Diff: %.0f,  Got: %s ago\n",
		common.Last.Block.Height,
		time.Unix(int64(common.Last.Block.Timestamp()), 0).Format("2006/01/02 15:04:05"),
		btc.GetDifficulty(common.Last.Block.Bits()), time.Now().Sub(common.Last.Time).String())
	common.Last.Mutex.Unlock()

	network.Mutex_net.Lock()
	fmt.Printf("BlocksCached: %d,  NetQueueSize: %d,  NetConns: %d,  Peers: %d\n",
		len(network.CachedBlocks), len(network.NetBlocks), len(network.OpenCons), peersdb.PeerDB.Count())
	network.Mutex_net.Unlock()

	network.TxMutex.Lock()
	fmt.Printf("TransactionsToSend:%d,  TransactionsRejected:%d,  TransactionsPending:%d/%d\n",
		len(network.TransactionsToSend), len(network.TransactionsRejected),
		len(network.TransactionsPending), len(network.NetTxs))
	fmt.Printf("WaitingForInputs:%d,  SpentOutputs:%d,  Hashrate:%s\n",
		len(network.WaitingForInputs), len(network.SpentOutputs), usif.GetNetworkHashRate())
	network.TxMutex.Unlock()

	common.PrintStats()

	// Memory used
	al, sy := sys.MemUsed()
	fmt.Println("Go version:", runtime.Version())
	fmt.Println("Heap size:", al>>20, "MB    Sys mem used:", sy>>20, "MB",
		"   QDB Extra mem:", qdb.ExtraMemoryConsumed>>20, "MB in", qdb.ExtraMemoryAllocCnt, "parts")

	var gs debug.GCStats
	debug.ReadGCStats(&gs)
	fmt.Println("LastGC:", time.Now().Sub(gs.LastGC).String(),
		"   NumGC:", gs.NumGC,
		"   PauseTotal:", gs.PauseTotal.String())

	fmt.Println("Gocoin:", lib.Version,
		"  Threads:", sys.UseThreads,
		"  Uptime:", time.Now().Sub(common.StartTime).String(),
		"  ECDSA cnt:", btc.EcdsaVerifyCnt)
}


func show_counters(par string) {
	common.CounterMutex.Lock()
	ck := make([]string, 0)
	for k, _ := range common.Counter {
		if par=="" || strings.HasPrefix(k, par) {
			ck = append(ck, k)
		}
	}
	sort.Strings(ck)

	var li string
	for i := range ck {
		k := ck[i]
		v := common.Counter[k]
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
	common.CounterMutex.Unlock()
}


func ui_dbg(par string) {
	v, e := strconv.ParseInt(par, 10, 32)
	if e == nil {
		common.DebugLevel = v
	}
	fmt.Println("common.DebugLevel:", common.DebugLevel)
}


func show_cached(par string) {
	for _, v := range network.CachedBlocks {
		fmt.Printf(" * %s -> %s\n", v.Hash.String(), btc.NewUint256(v.ParentHash()).String())
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
	al, sy := sys.MemUsed()

	fmt.Println("Allocated:", al>>20, "MB")
	fmt.Println("SystemMem:", sy>>20, "MB")
	fmt.Println("QDB Extra:", qdb.ExtraMemoryConsumed>>20, "MB")

	if p=="" {
		return
	}
	if p=="free" {
		fmt.Println("Freeing the mem...")
		sys.FreeMem()
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
	bl, _, e := common.BlockChain.Blocks.BlockGet(h)
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
	usif.Exit_now = true
}


func blchain_stats(par string) {
	fmt.Println(common.BlockChain.Stats())
}


func qdb_stats(par string) {
	fmt.Print(qdb.GetStats())
}


func defrag_blocks(par string) {
	switch par {
		case "utxo": usif.DefragBlocksDB = 1
		case "blks": usif.DefragBlocksDB = 2
		case "all": usif.DefragBlocksDB = 3
		default:
			fmt.Println("Specify what to defragment: utxo, blks or all")
			return
	}
	usif.Exit_now = true
}


func set_ulmax(par string) {
	v, e := strconv.ParseUint(par, 10, 64)
	if e == nil {
		common.UploadLimit = uint(v<<10)
	}
	if common.UploadLimit!=0 {
		fmt.Printf("Current upload limit is %d KB/s\n", common.UploadLimit>>10)
	} else {
		fmt.Println("The upload speed is not limited")
	}
}


func set_dlmax(par string) {
	v, e := strconv.ParseUint(par, 10, 64)
	if e == nil {
		common.DownloadLimit = uint(v<<10)
	}
	if common.DownloadLimit!=0 {
		fmt.Printf("Current upload limit is %d KB/s\n", common.DownloadLimit>>10)
	} else {
		fmt.Println("The upload speed is not limited")
	}
}


func set_config(s string) {
	if s!="" {
		new := common.CFG
		e := json.Unmarshal([]byte("{"+s+"}"), &new)
		if e != nil {
			println(e.Error())
		} else {
			common.CFG = new
			common.Reset()
			fmt.Println("Config changed. Execute configsave, if you want to save it.")
		}
	}
	dat, _ := json.Marshal(&common.CFG)
	fmt.Println(string(dat))
}


func load_config(s string) {
	d, e := ioutil.ReadFile(common.ConfigFile)
	if e != nil {
		println(e.Error())
		return
	}
	e = json.Unmarshal(d, &common.CFG)
	if e != nil {
		println(e.Error())
		return
	}
	common.Reset()
	fmt.Println("Config reloaded")
}


func save_config(s string) {
	if common.SaveConfig() {
		fmt.Println("Current settings saved to", common.ConfigFile)
	}
}


func show_addresses(par string) {
	fmt.Println(peersdb.PeerDB.Count(), "peers in the database")
	if par=="list" {
		cnt :=  0
		peersdb.PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
			cnt++
			fmt.Printf("%4d) %s\n", cnt, peersdb.NewPeer(v).String())
			return 0
		})
	} else if par=="ban" {
		cnt :=  0
		peersdb.PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
			pr := peersdb.NewPeer(v)
			if pr.Banned != 0 {
				cnt++
				fmt.Printf("%4d) %s\n", cnt, pr.String())
			}
			return 0
		})
		if cnt==0 {
			fmt.Println("No banned peers in the DB")
		}
	} else if par != "" {
		limit, er := strconv.ParseUint(par, 10, 32)
		if er != nil {
			fmt.Println("Specify number of best peers to display")
			return
		}
		prs := peersdb.GetBestPeers(uint(limit), nil)
		for i := range prs {
			fmt.Printf("%4d) %s", i+1, prs[i].String())
			if network.ConnectionActive(prs[i]) {
				fmt.Print("  CONNECTED")
			}
			fmt.Print("\n")
		}
	} else {
		fmt.Println("Use 'peers list' to list them")
		fmt.Println("Use 'peers ban' to list the benned ones")
		fmt.Println("Use 'peers <number>' to show the most recent ones")
	}
}


func list_alerst(p string) {
	network.Alert_access.Lock()
	for _, v := range network.Alerts {
		fmt.Println("===", v.ID)
		fmt.Println(" Version:", v.Version)
		fmt.Println(" RelayUntil:", time.Unix(v.RelayUntil, 0).Format("2006-01-02 15:04:05"))
		fmt.Println(" Expiration:", time.Unix(v.Expiration, 0).Format("2006-01-02 15:04:05"))
		fmt.Println(" Cancel:", v.Cancel)
		fmt.Print(" SetCancel: [")
		for i := range v.SetCancel {
			fmt.Print(" ", v.SetCancel[i])
		}
		fmt.Println("]")
		fmt.Println(" MinVer:", v.MinVer)
		fmt.Println(" MaxVer:", v.MaxVer)
		fmt.Println(" SetSubVer:")
		for i := range v.SetSubVer {
			fmt.Println("    ", v.SetSubVer[i])
		}
		fmt.Println(" Priority:", v.Priority)
		fmt.Println(" Comment:", v.Comment)
		fmt.Println(" StatusBar:", v.StatusBar)
		fmt.Println(" Reserved:", v.Reserved)
	}
	network.Alert_access.Unlock()
}


func coins_age(s string) {
	common.BlockChain.Unspent.PrintCoinAge()
}

func init() {
	newUi("age", true, coins_age, "Show age of records in UTXO database")
	newUi("alerts a", false, list_alerst, "Show received alerts")
	newUi("bchain b", true, blchain_stats, "Display blockchain statistics")
	newUi("cache", false, show_cached, "Show blocks cached in memory")
	newUi("configload cl", false, load_config, "Re-load settings from the common file")
	newUi("configsave cs", false, save_config, "Save current settings to a common file")
	newUi("configset cfg", false, set_config, "Set a specific common value - use JSON, omit top {}")
	newUi("counters c", false, show_counters, "Show all kind of debug counters")
	newUi("dbg d", false, ui_dbg, "Control debugs (use numeric parameter)")
	newUi("defrag", true, defrag_blocks, "Defragment database files on disk (use with: utxo | blks | all)")
	newUi("dlimit dl", false, set_dlmax, "Set maximum download speed. The value is in KB/second - 0 for unlimited")
	newUi("help h ?", false, show_help, "Shows this help")
	newUi("info i", false, show_info, "Shows general info about the node")
	newUi("mem", false, show_mem, "Show detailed memory stats (optionally free, gc or a numeric param)")
	newUi("peers", false, show_addresses, "Dump pers database (specify number)")
	newUi("qdbstats qs", false, qdb_stats, "Show statistics of QDB engine")
	newUi("quit q", true, ui_quit, "Exit nicely, saving all files. Otherwise use Ctrl+C")
	newUi("savebl", false, dump_block, "Saves a block with a given hash to a binary file")
	newUi("ulimit ul", false, set_ulmax, "Set maximum upload speed. The value is in KB/second - 0 for unlimited")
}
