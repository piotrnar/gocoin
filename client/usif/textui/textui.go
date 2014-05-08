package textui

import (
	"os"
	"fmt"
	"time"
	"sort"
	"bytes"
	"bufio"
	"strings"
	"strconv"
	"runtime"
	"io/ioutil"
	"encoding/hex"
	"encoding/json"
	"runtime/debug"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/qdb"
	"github.com/piotrnar/gocoin/others/ver"
	"github.com/piotrnar/gocoin/others/utils"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/usif"
	"github.com/piotrnar/gocoin/client/wallet"
	"github.com/piotrnar/gocoin/client/network"
)

type oneUiCmd struct {
	cmds []string // command name
	help string // a helf for this command
	sync bool  // shall be executed in the blochcina therad
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
							common.Busy_mutex.Lock()
							if common.BusyWith!="" {
								print("now common.BusyWith with ", common.BusyWith)
							}
							common.Busy_mutex.Unlock()
							println("...")
							sta := time.Now().UnixNano()
							req := &usif.OneUiReq{Param:param, Handler:uiCmds[i].handler}
							req.Done.Add(1)
							usif.UiChannel <- req
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
		len(network.CachedBlocks), len(network.NetBlocks), len(network.OpenCons), network.PeerDB.Count())
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

	fmt.Println("Gocoin:", ver.SourcesTag,
		"  Threads:", btc.UseThreads,
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


func list_unspent(addr string) {
	fmt.Println("Checking unspent coins for addr", addr)
	var ad *btc.BtcAddr
	var e error
	ad, e = btc.NewAddrFromString(addr)
	if e != nil {
		println(e.Error())
		return
	}
	sa := ad.StealthAddr
	exp_scr := ad.OutScript()
	var walk btc.FunctionWalkUnspent
	var unsp btc.AllUnspentTx

	if sa==nil {
		walk = func(db *qdb.DB, k qdb.KeyType, rec *btc.OneWalkRecord) (uint32) {
			if bytes.Equal(rec.Script(), exp_scr) {
				unsp = append(unsp, rec.ToUnspent(ad))
			}
			return 0
		}
	} else {
		d := wallet.FindStealthSecret(sa)
		if d==nil {
			fmt.Println("No matching secret found your wallet/stealth folder")
			return
		}
		defer utils.ClearBuffer(d)

		walk = func(db *qdb.DB, k qdb.KeyType, rec *btc.OneWalkRecord) (uint32) {
			fl, uo := wallet.CheckStealthRec(db, k, rec, sa, d)
			if uo!=nil {
				unsp = append(unsp, uo)
			}
			return fl
		}
	}
	common.BlockChain.Unspent.BrowseUTXO(false, walk)

	sort.Sort(unsp)
	var sum uint64
	for i := range unsp {
		if len(unsp)<200 {
			fmt.Println(unsp[i].String())
		}
		sum += unsp[i].Value
	}
	fmt.Printf("Total %.8f unspent BTC in %d outputs at address %s\n",
		float64(sum)/1e8, len(unsp), ad.String());
}

func qdb_stats(par string) {
	fmt.Print(qdb.GetStats())
}


func defrag_blocks(par string) {
	usif.DefragBlocksDB = true
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


func load_wallet(fn string) {
	if fn=="." {
		fmt.Println("Default wallet from", common.GocoinHomeDir+"wallet/DEFAULT")
		wallet.LoadWallet(common.GocoinHomeDir+"wallet/DEFAULT")
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
	dat, _ := json.Marshal(&common.CFG)
	if dat != nil {
		ioutil.WriteFile(common.ConfigFile, dat, 0660)
		fmt.Println("Current settings saved to", common.ConfigFile)
	}
}


func show_balance(p string) {
	if p=="sum" {
		fmt.Print(wallet.DumpBalance(wallet.MyBalance, nil, false, true))
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


func show_balance_stats(p string) {
	println("CachedAddrs count:", len(wallet.CachedAddrs))
	println("CacheUnspentIdx count:", len(wallet.CacheUnspentIdx))
	println("CacheUnspent count:", len(wallet.CacheUnspent))
	if p!="" {
		wallet.LockBal()
		for i := range wallet.CacheUnspent {
			fmt.Printf("%5d) %35s - %d unspent output(s)\n", i, wallet.CacheUnspent[i].BtcAddr.String(),
				len(wallet.CacheUnspent[i].AllUnspentTx))
			/*for j := range wallet.CacheUnspent[i].AllUnspentTx {
				fmt.Printf(" %5d) %s\n", j, wallet.CacheUnspent[i].AllUnspentTx[j].String())
			}*/
		}
		wallet.UnlockBal()
	}
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


func do_scan_stealth(p string, ignore_prefix bool) {
	sa, _ := btc.NewStealthAddrFromString(p)
	if sa==nil {
		fmt.Println("Specify base58 encoded stealth address")
		return
	}
	if sa.Version!=btc.StealthAddressVersion(common.CFG.Testnet) {
		fmt.Println("Incorrect version of the stealth address")
		return
	}
	if len(sa.SpendKeys)!=1 {
		fmt.Println("Currently only single spend keys are supported. This address has", len(sa.SpendKeys))
		return
	}

	//fmt.Println("scankey", hex.EncodeToString(sa.ScanKey[:]))
	if ignore_prefix {
		sa.Prefix = []byte{0}
		fmt.Println("Ignoring Prefix inside the address")
	} else if len(sa.Prefix)==0 {
		fmt.Println("Prefix not present in the address")
	} else {
		fmt.Println("Prefix", sa.Prefix[0], hex.EncodeToString(sa.Prefix[1:]))
	}

	d := wallet.FindStealthSecret(sa)
	if d==nil {
		fmt.Println("No matching secret found your wallet/stealth folder")
		return
	}
	defer utils.ClearBuffer(d)

	var unsp btc.AllUnspentTx

	common.BlockChain.Unspent.BrowseUTXO(true, func(db *qdb.DB, k qdb.KeyType, rec *btc.OneWalkRecord) (uint32) {
		fl, uo := wallet.CheckStealthRec(db, k, rec, sa, d)
		if uo!=nil {
			unsp = append(unsp, uo)
		}
		return fl
	})

	sort.Sort(unsp)
	os.RemoveAll("balance")
	os.MkdirAll("balance/", 0770)
	utxt, _ := os.Create("balance/unspent.txt")
	fmt.Print(wallet.DumpBalance(unsp, utxt, true, false))
}


func scan_stealth(p string) {
	do_scan_stealth(p, false)
}

func scan_all_stealth(p string) {
	do_scan_stealth(p, true)
}


func init() {
	newUi("alerts a", false, list_alerst, "Show received alerts")
	newUi("balance bal", true, show_balance, "Show & save balance of currently loaded or a specified wallet")
	newUi("balstat", true, show_balance_stats, "Show balance cache statistics")
	newUi("bchain b", true, blchain_stats, "Display blockchain statistics")
	newUi("cache", false, show_cached, "Show blocks cached in memory")
	newUi("configload cl", false, load_config, "Re-load settings from the common file")
	newUi("configsave cs", false, save_config, "Save current settings to a common file")
	newUi("configset cfg", false, set_config, "Set a specific common value - use JSON, omit top {}")
	newUi("counters c", false, show_counters, "Show all kind of debug counters")
	newUi("dbg d", false, ui_dbg, "Control debugs (use numeric parameter)")
	newUi("defrag", true, defrag_blocks, "Defragment databases (UTXO + Blocks) on disk and exits")
	newUi("dlimit dl", false, set_dlmax, "Set maximum download speed. The value is in KB/second - 0 for unlimited")
	newUi("help h ?", false, show_help, "Shows this help")
	newUi("info i", false, show_info, "Shows general info about the node")
	newUi("mem", false, show_mem, "Show detailed memory stats (optionally free, gc or a numeric param)")
	newUi("peers", false, show_addresses, "Dump pers database (warning: may be long)")
	newUi("qdbstats qs", false, qdb_stats, "Show statistics of QDB engine")
	newUi("quit q", true, ui_quit, "Exit nicely, saving all files. Otherwise use Ctrl+C")
	newUi("savebl", false, dump_block, "Saves a block with a given hash to a binary file")
	newUi("scan", true, scan_stealth, "Get balance of a stealth address")
	newUi("scan0", true, scan_all_stealth, "Get balance of a stealth address. Ignore the prefix")
	newUi("ulimit ul", false, set_ulmax, "Set maximum upload speed. The value is in KB/second - 0 for unlimited")
	newUi("unspent u", true, list_unspent, "Shows unpent outputs for a given address")
	newUi("wallet wal", true, load_wallet, "Load wallet from given file (or re-load the last one) and display its addrs")
}
