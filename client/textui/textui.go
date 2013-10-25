package textui

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
	"io/ioutil"
	"encoding/json"
	"runtime/debug"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/qdb"
	"github.com/piotrnar/gocoin/client/dbase"
	"github.com/piotrnar/gocoin/client/config"
	"github.com/piotrnar/gocoin/client/wallet"
	"github.com/piotrnar/gocoin/client/bwlimit"
	"github.com/piotrnar/gocoin/client/network"
)

type oneUiCmd struct {
	cmds []string // command name
	help string // a helf for this command
	sync bool  // shall be executed in the blochcina therad
	handler func(pars string)
}

type OneUiReq struct {
	Param string
	Handler func(pars string)
	Done sync.WaitGroup
}

var (
	UiChannel chan *OneUiReq = make(chan *OneUiReq, 1)
	uiCmds []*oneUiCmd
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
	var prompt bool = true
	time.Sleep(1e8)
	for {
		if prompt {
			ShowPrompt()
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
							config.Busy_mutex.Lock()
							if config.BusyWith!="" {
								print("now config.BusyWith with ", config.BusyWith)
							}
							config.Busy_mutex.Unlock()
							println("...")
							sta := time.Now().UnixNano()
							req := &OneUiReq{Param:param, Handler:uiCmds[i].handler}
							req.Done.Add(1)
							UiChannel <- req
							go func() {
								req.Done.Wait()
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
	config.Busy_mutex.Lock()
	if config.BusyWith!="" {
		fmt.Println("Chain thread config.BusyWith with:", config.BusyWith)
	} else {
		fmt.Println("Chain thread is idle")
	}
	config.Busy_mutex.Unlock()

	config.Last.Mutex.Lock()
	fmt.Println("Last Block:", config.Last.Block.BlockHash.String())
	fmt.Printf("Height: %d @ %s,  Diff: %.0f,  Got: %s ago\n",
		config.Last.Block.Height,
		time.Unix(int64(config.Last.Block.Timestamp), 0).Format("2006/01/02 15:04:05"),
		btc.GetDifficulty(config.Last.Block.Bits), time.Now().Sub(config.Last.Time).String())
	config.Last.Mutex.Unlock()

	network.Mutex_net.Lock()
	fmt.Printf("BlocksCached: %d,  NetQueueSize: %d,  NetConns: %d,  Peers: %d\n",
		len(network.CachedBlocks), len(network.NetBlocks), len(network.OpenCons), network.PeerDB.Count())
	network.Mutex_net.Unlock()

	network.TxMutex.Lock()
	fmt.Printf("TransactionsToSend:%d,  TransactionsRejected:%d,  TransactionsPending:%d/%d\n",
		len(network.TransactionsToSend), len(network.TransactionsRejected),
		len(network.TransactionsPending), len(network.NetTxs))
	fmt.Printf("WaitingForInputs:%d,  SpentOutputs:%d\n",
		len(network.WaitingForInputs), len(network.SpentOutputs))
	network.TxMutex.Unlock()

	bwlimit.PrintStats()

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
		"  Uptime:", time.Now().Sub(config.StartTime).String(),
		"  ECDSA cnt:", btc.EcdsaVerifyCnt)
}


func show_counters(par string) {
	config.CounterMutex.Lock()
	ck := make([]string, 0)
	for k, _ := range config.Counter {
		if par=="" || strings.HasPrefix(k, par) {
			ck = append(ck, k)
		}
	}
	sort.Strings(ck)

	var li string
	for i := range ck {
		k := ck[i]
		v := config.Counter[k]
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
	config.CounterMutex.Unlock()
}


func ui_dbg(par string) {
	v, e := strconv.ParseInt(par, 10, 32)
	if e == nil {
		config.DebugLevel = v
	}
	fmt.Println("config.DebugLevel:", config.DebugLevel)
}


func show_cached(par string) {
	for _, v := range network.CachedBlocks {
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
	bl, _, e := config.BlockChain.Blocks.BlockGet(h)
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
	config.Exit_now = true
}


func blchain_stats(par string) {
	fmt.Println(config.BlockChain.Stats())
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
	unsp := config.BlockChain.GetAllUnspent(a[:], false)
	sort.Sort(unsp)
	var sum uint64
	for i := range unsp {
		if len(unsp)<200 {
			fmt.Println(unsp[i].String())
		}
		sum += unsp[i].Value
	}
	fmt.Printf("Total %.8f unspent BTC in %d outputs at address %s\n",
		float64(sum)/1e8, len(unsp), a[0].String());
}

func qdb_stats(par string) {
	fmt.Print(qdb.GetStats())
}


func defrag_blocks(par string) {
	network.NetCloseAll()
	network.ClosePeerDB()

	println("Creating empty database in", config.GocoinHomeDir+"defrag", "...")
	os.RemoveAll(config.GocoinHomeDir+"defrag")
	defragdb := btc.NewBlockDB(config.GocoinHomeDir+"defrag")

	fmt.Println("Defragmenting the database...")

	blk := config.BlockChain.BlockTreeRoot
	for {
		blk = blk.FindPathTo(config.BlockChain.BlockTreeEnd)
		if blk==nil {
			fmt.Println("Database defragmenting finished successfully")
			fmt.Println("To use the new DB, move the two new files to a parent directory and restart the client")
			break
		}
		if (blk.Height&0xff)==0 {
			fmt.Printf("%d / %d blocks written (%d%%)\r", blk.Height, config.BlockChain.BlockTreeEnd.Height,
				100 * blk.Height / config.BlockChain.BlockTreeEnd.Height)
		}
		bl, trusted, er := config.BlockChain.Blocks.BlockGet(blk.BlockHash)
		if er != nil {
			println("FATAL ERROR during BlockGet:", er.Error())
			break
		}
		nbl, er := btc.NewBlock(bl)
		if er != nil {
			println("FATAL ERROR during NewBlock:", er.Error())
			break
		}
		nbl.Trusted = trusted
		defragdb.BlockAdd(blk.Height, nbl)
	}

	defragdb.Sync()
	defragdb.Close()

	config.CloseBlockChain()
	dbase.UnlockDatabaseDir()

	fmt.Println("The client will exit now")
	os.Exit(0)
}


func set_ulmax(par string) {
	v, e := strconv.ParseUint(par, 10, 64)
	if e == nil {
		bwlimit.UploadLimit = uint(v<<10)
	}
	if bwlimit.UploadLimit!=0 {
		fmt.Printf("Current upload limit is %d KB/s\n", bwlimit.UploadLimit>>10)
	} else {
		fmt.Println("The upload speed is not limited")
	}
}


func set_dlmax(par string) {
	v, e := strconv.ParseUint(par, 10, 64)
	if e == nil {
		bwlimit.DownloadLimit = uint(v<<10)
	}
	if bwlimit.DownloadLimit!=0 {
		fmt.Printf("Current upload limit is %d KB/s\n", bwlimit.DownloadLimit>>10)
	} else {
		fmt.Println("The upload speed is not limited")
	}
}


func load_wallet(fn string) {
	if fn=="." {
		fmt.Println("Default wallet from", config.GocoinHomeDir+"wallet/DEFAULT")
		wallet.LoadWallet(config.GocoinHomeDir+"wallet/DEFAULT")
	} else if fn != "" {
		fmt.Println("Switching to wallet from", fn)
		wallet.LoadWallet(fn)
	}

	if wallet.MyWallet==nil {
		fmt.Println("No wallet loaded")
		return
	}

	if fn == "-" {
		fmt.Println("Reloading wallet from", wallet.MyWallet.FileName)
		wallet.LoadWallet(wallet.MyWallet.FileName)
		fmt.Println("Dumping current wallet from", wallet.MyWallet.FileName)
	}

	for i := range wallet.MyWallet.Addrs {
		fmt.Println(" ", wallet.MyWallet.Addrs[i].StringLab())
	}
}


func set_config(s string) {
	if s!="" {
		new := config.CFG
		e := json.Unmarshal([]byte("{"+s+"}"), &new)
		if e != nil {
			println(e.Error())
		} else {
			config.CFG = new
			config.Reset()
			fmt.Println("Config changed. Execute configsave, if you want to save it.")
		}
	}
	dat, _ := json.Marshal(&config.CFG)
	fmt.Println(string(dat))
}


func load_config(s string) {
	d, e := ioutil.ReadFile(config.ConfigFile)
	if e != nil {
		println(e.Error())
		return
	}
	e = json.Unmarshal(d, &config.CFG)
	if e != nil {
		println(e.Error())
		return
	}
	config.Reset()
	fmt.Println("Config reloaded")
}


func save_config(s string) {
	dat, _ := json.Marshal(&config.CFG)
	if dat != nil {
		ioutil.WriteFile(config.ConfigFile, dat, 0660)
		fmt.Println("Current settings saved to", config.ConfigFile)
	}
}


func show_balance(p string) {
	if p=="sum" {
		fmt.Print(wallet.DumpBalance(nil, false))
		return
	}
	if p!="" {
		fmt.Println("Using wallet from file", p, "...")
		wallet.LoadWallet(p)
	}

	if wallet.MyWallet==nil {
		println("You have no loaded wallet")
		return
	}

	if len(wallet.MyWallet.Addrs)==0 {
		println("Your loaded wallet has no addresses")
		return
	}

	fmt.Print(wallet.UpdateBalanceFolder())
	fmt.Println("Your balance data has been saved to the 'balance/' folder.")
	fmt.Println("You nend to move this folder to your wallet PC, to spend the coins.")
}


func show_addresses(par string) {
	fmt.Println(network.PeerDB.Count(), "peers in the database")
	if par=="list" {
		cnt :=  0
		network.PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
			cnt++
			fmt.Printf("%4d) %s\n", cnt, network.NewPeer(v).String())
			return 0
		})
	} else if par=="ban" {
		cnt :=  0
		network.PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
			pr := network.NewPeer(v)
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
		prs := network.GetBestPeers(uint(limit), false)
		for i := range prs {
			fmt.Printf("%4d) %s", i+1, prs[i].String())
			if network.ConnectionActive(prs[i]) {
				fmt.Print("  CONNECTED")
			}
			fmt.Print("\n")
		}
	} else {
		fmt.Println("Use 'peers list' to list them")
	}
}


func list_alerst(p string) {
	network.Alert_access.Lock()
	for _, v := range network.Alerts {
		fmt.Println(v.Version, v.RelayUntil, v.Expiration, v.ID, v.Cancel,
			v.MinVer, v.MaxVer, v.Priority, v.Comment, v.StatusBar, v.Reserved)
	}
	network.Alert_access.Unlock()
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
	newUi("qdbstats qs", false, qdb_stats, "Show statistics of QDB engine")
	newUi("defrag", true, defrag_blocks, "Defragment blocks database and quit (purges orphaned blocks)")

	newUi("ulimit ul", false, set_ulmax, "Set maximum upload speed. The value is in KB/second - 0 for unlimited")
	newUi("dlimit dl", false, set_dlmax, "Set maximum download speed. The value is in KB/second - 0 for unlimited")

	newUi("wallet wal", true, load_wallet, "Load wallet from given file (or re-load the last one) and display its addrs")

	newUi("configsave cs", false, save_config, "Save current settings to a config file")
	newUi("configload cl", false, load_config, "Re-load settings from the config file")
	newUi("configset cfg", false, set_config, "Set a specific config value - use JSON, omit top {}")

	newUi("balance bal", true, show_balance, "Show & save balance of currently loaded or a specified wallet")

	newUi("pers", false, show_addresses, "Dump pers database (warning: may be long)")

	newUi("alerts a", false, list_alerst, "Show received alerts")
}
