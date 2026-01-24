package textui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/piotrnar/gocoin"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/peersdb"
	"github.com/piotrnar/gocoin/client/txpool"
	"github.com/piotrnar/gocoin/client/usif"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/sys"
)

type oneUiCmd struct {
	handler func(pars string)
	help    string
	cmds    []string
	sync    bool
}

var (
	uiCmds      []*oneUiCmd
	show_prompt bool = true
)

// newUi adds a new UI commend handler.
func newUi(cmds string, sync bool, hn func(string), help string) {
	cs := strings.Split(cmds, " ")
	if len(cs[0]) > 0 {
		var c = new(oneUiCmd)
		c.cmds = append(c.cmds, cs...)
		c.sync = sync
		c.help = help
		c.handler = hn
		if len(uiCmds) > 0 {
			var i int
			for i = 0; i < len(uiCmds); i++ {
				if uiCmds[i].cmds[0] > c.cmds[0] {
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
		if l == "y" {
			return true
		} else if l == "n" {
			return false
		}
	}
}

func ShowPrompt() {
	fmt.Print("> ")
}

func MainThread() {
	time.Sleep(1e9) // hold on for 1 sencond before showing the show_prompt
	for !usif.Exit_now.Get() {
		if show_prompt {
			ShowPrompt()
		}
		show_prompt = true
		li := strings.Trim(readline(), " \n\t\r")
		if len(li) > 0 {
			cmdpar := strings.SplitN(li, " ", 2)
			cmd := cmdpar[0]
			param := ""
			if len(cmdpar) == 2 {
				param = cmdpar[1]
			}
			found := false
			for i := range uiCmds {
				for j := range uiCmds[i].cmds {
					if cmd == uiCmds[i].cmds[j] {
						found = true
						if uiCmds[i].sync {
							usif.ExecUiReq(&usif.OneUiReq{Param: param, Handler: uiCmds[i].handler})
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
	fmt.Println("main.go last seen in line:", common.BusyIn())

	network.MutexRcv.Lock()
	discarded := len(network.DiscardedBlocks)
	cached := network.CachedBlocksLen()
	b2g_len := len(network.BlocksToGet)
	b2g_idx_len := len(network.IndexToBlocksToGet)
	network.MutexRcv.Unlock()

	fmt.Printf("Gocoin: %s,  Synced: %t (%d),   PID: %d,   Uptime %s\n", gocoin.Version,
		common.Get(&common.BlockChainSynchronized), network.HeadersReceived.Get(), os.Getpid(),
		time.Since(common.StartTime).String())
	// Memory used
	al, sy := sys.MemUsed()
	cb, ca, ms := common.MemUsed()
	fmt.Printf("HeapUsed: %d MB,  SysUsed: %d MB,  UTXO-X-mem: %dMB in %d/%d\n",
		al>>20, sy>>20, cb>>20, ca, ms)
	fmt.Printf("Peers: %d,  ECDSAs: %d %d %d,  AvgFee: %.1f SPB,  Saving: %t\n",
		peersdb.PeerDB.Count(),
		btc.EcdsaVerifyCnt(), btc.SchnorrVerifyCnt(), btc.CheckPay2ContractCnt(),
		usif.GetAverageFee(), common.BlockChain.Unspent.WritingInProgress.Get())

	network.MutexRcv.Lock()
	fmt.Println("LastHeder:", network.LastCommitedHeader.BlockHash.String(), "@",
		network.LastCommitedHeader.Height)
	network.MutexRcv.Unlock()

	common.Last.Mutex.Lock()
	fmt.Println("LastBlock:", common.Last.Block.BlockHash.String(), "@", common.Last.Block.Height)
	fmt.Printf("  %s (~%s),  Diff: %.0f,  %s ago\n",
		time.Unix(int64(common.Last.Block.Timestamp()), 0).Format("2006/01/02 15:04:05"),
		time.Unix(int64(common.Last.Block.GetMedianTimePast()), 0).Format("15:04:05"),
		btc.GetDifficulty(common.Last.Block.Bits()), time.Since(common.Last.Time).String())
	common.Last.Mutex.Unlock()

	network.Mutex_net.Lock()
	fmt.Printf("Blocks Queued: %d,  Cached: %d,  Discarded: %d,  To Get: %d/%d,  UTXO.db on disk: %d\n",
		len(network.NetBlocks), cached, discarded, b2g_len, b2g_idx_len,
		atomic.LoadUint32(&common.BlockChain.Unspent.CurrentHeightOnDisk))
	network.Mutex_net.Unlock()

	var gs debug.GCStats
	debug.ReadGCStats(&gs)
	usif.BlockFeesMutex.Lock()
	fmt.Println("Go version:", runtime.Version(), "  LastGC:", time.Since(gs.LastGC).String(),
		"  NumGC:", gs.NumGC, "  PauseTotal:", gs.PauseTotal.String())
	usif.BlockFeesMutex.Unlock()
}

func show_counters(par string) {
	common.CounterMutex.Lock()
	ck := make([]string, 0)
	for k := range common.Counter {
		if par == "" || strings.HasPrefix(k, par) {
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
		} else if li != "" {
			li += ",   "
		}
		li += s
	}
	if li != "" {
		fmt.Println(li)
	}
	common.CounterMutex.Unlock()
}

func show_help(par string) {
	fmt.Println("The following", len(uiCmds), "commands are supported:")
	for i := range uiCmds {
		fmt.Print("   ")
		for j := range uiCmds[i].cmds {
			if j > 0 {
				fmt.Print(", ")
			}
			fmt.Print(uiCmds[i].cmds[j])
		}
		fmt.Println(" -", uiCmds[i].help)
	}
	fmt.Println("All the commands are case sensitive.")
}

func show_mem(p string) {
	if p == "free" {
		fmt.Println("Freeing the mem...")
		sys.FreeMem()
		show_mem("")
		return
	}
	if p == "gc" {
		fmt.Println("Running GC...")
		runtime.GC()
		fmt.Println("Done.")
		return
	}

	if strings.HasSuffix(p, "MB") {
		i, e := strconv.ParseUint(p[:len(p)-2], 10, 32)
		if e != nil {
			println(e.Error())
			return
		}
		fmt.Println("CFG.Memory.MemoryLimitMB =", i)
		fmt.Println("Execute 'sc' to save the config.")
		common.LockCfg()
		common.CFG.Memory.MemoryLimitMB = uint32(i)
		common.UpdateMemoryLimit()
		common.UnlockCfg()
		return
	}

	i, e := strconv.ParseInt(p, 10, 64)
	if e == nil {
		common.LockCfg()
		common.CFG.Memory.GCPercTrshold = int(i)
		common.UnlockCfg()
		debug.SetGCPercent(int(i))
		fmt.Println("CFG.Memory.GCPercTrshold = ", i)
		fmt.Println("Execute 'sc' to save the config.")
		return
	}

	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	fmt.Println("Alloc         :", ms.Alloc)
	fmt.Println("TotalAlloc    :", ms.TotalAlloc)
	fmt.Println("Sys           :", ms.Sys)
	fmt.Println("Lookups       :", ms.Lookups)
	fmt.Println("Mallocs       :", ms.Mallocs)
	fmt.Println("Frees         :", ms.Frees)
	fmt.Println("HeapAlloc     :", ms.HeapAlloc)
	fmt.Println("HeapSys       :", ms.HeapSys)
	fmt.Println("HeapIdle      :", ms.HeapIdle)
	fmt.Println("HeapInuse     :", ms.HeapInuse)
	fmt.Println("HeapReleased  :", ms.HeapReleased)
	fmt.Println("HeapObjects   :", ms.HeapObjects)
	fmt.Println("StackInuse    :", ms.StackInuse)
	fmt.Println("StackSys      :", ms.StackSys)
	fmt.Println("MSpanInuse    :", ms.MSpanInuse)
	fmt.Println("MSpanSys      :", ms.MSpanSys)
	fmt.Println("MCacheInuse   :", ms.MCacheInuse)
	fmt.Println("MCacheSys     :", ms.MCacheSys)
	fmt.Println("BuckHashSys   :", ms.BuckHashSys)
	fmt.Println("GCSys         :", ms.GCSys)
	fmt.Println("OtherSys      :", ms.OtherSys)
	fmt.Println("LastGC        :", time.Unix(0, int64(ms.LastGC)).Format("15:04:05"))
	fmt.Println("PauseTotal    :", time.Duration(ms.PauseTotalNs).String())
	fmt.Println("NumGC         :", ms.NumGC)
	fmt.Println("NumForcedGC   :", ms.NumForcedGC)
	fmt.Println("GCCPUFraction :", ms.GCCPUFraction)
	fmt.Println("EnableGC      :", ms.EnableGC)
	fmt.Println("DebugGC       :", ms.DebugGC)

	if p == "" {
		return
	}

	if p == "bs" {
		var prvSize uint32
		for _, bs := range ms.BySize {
			fmt.Printf("  Alloc_Size:%6d -%6d  %10d allocs  %10d frees\n", prvSize, bs.Size, bs.Mallocs, bs.Frees)
			prvSize = bs.Size
		}
		return
	}
}

func dump_block(s string) {
	h := btc.NewUint256FromString(s)
	if h == nil {
		println("Specify block's hash")
		return
	}
	crec, _, er := common.BlockChain.Blocks.BlockGetExt(btc.NewUint256(h.Hash[:]))
	if er != nil {
		println("BlockGetExt:", er.Error())
		return
	}

	os.WriteFile(h.String()+".bin", crec.Data, 0700)
	fmt.Println("Block saved")
}

func ui_quit(par string) {
	if par != "" && par[0] == 'r' {
		usif.Restart.Set()
	}
	usif.Exit_now.Set()
}

func blchain_stats(par string) {
	fmt.Println(common.BlockChain.Stats())
}

func blchain_utxodb(par string) {
	fmt.Println(common.BlockChain.Unspent.UTXOStats())
}

func set_ulmax(par string) {
	v, e := strconv.ParseUint(par, 10, 64)
	if e == nil {
		common.SetUploadLimit(v << 10)
	}
	if common.UploadLimit() != 0 {
		fmt.Printf("Current upload limit is %d KB/s\n", common.UploadLimit()>>10)
	} else {
		fmt.Println("The upload speed is not limited")
	}
}

func set_dlmax(par string) {
	v, e := strconv.ParseUint(par, 10, 64)
	if e == nil {
		common.SetDownloadLimit(v << 10)
	}
	if common.DownloadLimit() != 0 {
		fmt.Printf("Current download limit is %d KB/s\n", common.DownloadLimit()>>10)
	} else {
		fmt.Println("The download speed is not limited")
	}
}

func set_config(s string) {
	common.LockCfg()
	defer common.UnlockCfg()
	if s != "" {
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
	dat, _ := json.MarshalIndent(&common.CFG, "", "    ")
	fmt.Println(string(dat))
}

func load_config(s string) {
	d, e := os.ReadFile(common.ConfigFile)
	if e != nil {
		println(e.Error())
		return
	}
	common.LockCfg()
	defer common.UnlockCfg()
	e = json.Unmarshal(d, &common.CFG)
	if e != nil {
		println(e.Error())
		return
	}
	common.Reset()
	fmt.Println("Config reloaded")
}

func save_config(s string) {
	common.LockCfg()
	if common.SaveConfig() {
		fmt.Println("Current settings saved to", common.ConfigFile)
	}
	common.UnlockCfg()
}

func send_inv(par string) {
	cs := strings.Split(par, " ")
	if len(cs) != 2 {
		println("Specify hash and type")
		return
	}
	ha := btc.NewUint256FromString(cs[1])
	if ha == nil {
		println("Incorrect hash")
		return
	}
	v, e := strconv.ParseInt(cs[0], 10, 32)
	if e != nil {
		println("Incorrect type:", e.Error())
		return
	}
	network.NetRouteInv(uint32(v), ha, nil)
	fmt.Println("Inv sent to all peers")
}

func switch_trust(par string) {
	if par == "0" {
		common.FLAG.TrustAll = false
	} else if par == "1" {
		common.FLAG.TrustAll = true
	}
	fmt.Println("Assume blocks trusted:", common.FLAG.TrustAll)
}

func save_utxo(par string) {
	//common.BlockChain.Unspent.HurryUp()
	common.BlockChain.Unspent.Save()
}

func show_pending(par string) {
	network.MutexRcv.Lock()
	cnt := len(network.BlocksToGet)
	var sofar int
	fmt.Println("Number of blocks to get:", cnt)
	bh := network.LowestIndexToBlocksToGet
	for sofar < cnt {
		if b2gs := network.IndexToBlocksToGet[bh]; len(b2gs) > 0 {
			for _, bha := range b2gs {
				sofar++
				b2g := network.BlocksToGet[bha]
				fmt.Println(sofar, bh, b2g.Height, b2g.BlockHash.String())
				fmt.Printf("  Announced %s,  inpr:%d  invs:%t\n", time.Since(b2g.Started).String(), b2g.InProgress, b2g.SendInvs)
				if len(b2g.OnlyFetchFrom) > 0 {
					fmt.Print("  Only fetch from ", len(b2g.OnlyFetchFrom), " peers:")
					for _, fid := range b2g.OnlyFetchFrom {
						fmt.Print(" ", fid)
					}
					fmt.Println()
				} else {
					fmt.Println("  Fetch from anywhere")

				}
			}
		}
		bh++
	}
	network.MutexRcv.Unlock()
}

func show_cached(par string) {
	network.MutexRcv.Lock()
	cnt := len(network.CachedBlocksIdx)
	var sofar int
	fmt.Println("CachedBlocksIdx length:", cnt)
	bh := network.CachedMinHeight
	for sofar < cnt {
		if cblks, ok := network.CachedBlocksIdx[bh]; ok && len(cblks) > 0 {
			for _, cbl := range cblks {
				fmt.Print(sofar, " ", bh, " ")
				if cbl != nil {
					if cbl.Block != nil {
						fmt.Print(cbl.Block.Height, " ")
					} else {
						fmt.Print("nil.Block ")
					}
					if cbl.BlockHash != nil {
						fmt.Print(cbl.BlockHash.String(), " ")
					} else {
						fmt.Print("nil.Hash ")
					}
					if cbl.OneReceivedBlock != nil {
						fmt.Println(time.Since(cbl.OneReceivedBlock.TmStart).String(), "ago")
					} else {
						fmt.Println("nil.Rcvd")
					}
				} else {
					fmt.Println("NIL")
				}
				parent_hash := cbl.Parent.BlockHash
				_, parenttoget := network.BlocksToGet[parent_hash.BIdx()]
				_, parentrcvd := network.ReceivedBlocks[parent_hash.BIdx()]
				fmt.Println("   linking to:", parent_hash.String(), "   toget:", parenttoget, "   got:", parentrcvd)
			}
			sofar++
		}
		bh++
	}
	network.MutexRcv.Unlock()
}

func purge_utxo(par string) {
	common.BlockChain.Unspent.PurgeUnspendable(true)
	if !common.CFG.Memory.PurgeUnspendableUTXO {
		fmt.Println("Save your config file (cs) to have all the futher records purged automatically.")
		common.CFG.Memory.PurgeUnspendableUTXO = true
	}
}

func undo_block(par string) {
	println("Undoing block...")
	if par == "slow" {
		println("Slow mode ON")
	} else {
		txpool.BlockCommitInProgress(true)
	}
	common.BlockChain.UndoLastBlock()
	txpool.BlockCommitInProgress(false)
	println("Block un-DONE")
	common.Last.Mutex.Lock()
	common.Last.Block = common.BlockChain.LastBlock()
	common.UpdateScriptFlags(0)
	common.Last.Mutex.Unlock()
}

func redo_block(par string) {
	network.MutexRcv.Lock()
	end := network.LastCommitedHeader
	network.MutexRcv.Unlock()
	last := common.BlockChain.LastBlock()
	if last == end {
		println("You already are at the last known block - nothing to redo")
		return
	}

	sta := time.Now()
	nxt := last.FindPathTo(end)
	if nxt == nil {
		println("FindPathTo failed")
		return
	}

	if nxt.BlockSize == 0 {
		println("BlockSize is zero - block not downloaded yet or corrupt database")
		return
	}

	pre := time.Now()
	crec, _, _ := common.BlockChain.Blocks.BlockGetInternal(nxt.BlockHash, true)

	bl, er := btc.NewBlock(crec.Data)
	if er != nil {
		println("btc.NewBlock() error - corrupt database")
		return
	}
	bl.Height = nxt.Height

	// Recover the flags to be used when verifying scripts for non-trusted blocks (stored orphaned blocks)
	common.BlockChain.ApplyBlockFlags(bl)

	er = bl.BuildTxList()
	if er != nil {
		println("bl.BuildTxList() error - corrupt database")
		return
	}

	bl.Trusted.Clr() // assume not trusted

	tdl := time.Now()
	rb := &network.OneReceivedBlock{TmStart: sta, TmPreproc: pre, TmDownload: tdl}
	network.MutexRcv.Lock()
	network.ReceivedBlocks[bl.Hash.BIdx()] = rb
	network.MutexRcv.Unlock()

	fmt.Println("Putting block", bl.Height, "into net queue...")
	network.NetBlocks <- &network.BlockRcvd{Conn: nil, Block: bl, BlockTreeNode: nxt,
		OneReceivedBlock: rb, BlockExtraInfo: nil}
}

func kill_node(par string) {
	if par != "" && par[0] == 'r' {
		os.Exit(66)
	}
	os.Exit(1)
}

func webui_stats(par string) {
	fmt.Print(usif.GetWebUIStats())
}

func init() {
	newUi("bchain b", true, blchain_stats, "Display blockchain statistics")
	newUi("cach", true, show_cached, "Show cached blocks [del2height]")
	newUi("configload lc", false, load_config, "Re-load settings from the config file")
	newUi("configsave sc", false, save_config, "Save current settings to the config file")
	newUi("configset cfg", false, set_config, "Set a specific config value: use JSON, omit top {}")
	newUi("counters c", false, show_counters, "Show internal debug counters [prefix]")
	newUi("help h ?", false, show_help, "Shows this help")
	newUi("info i", false, show_info, "Shows general info about the node")
	newUi("inv", false, send_inv, "Send inv message to all the peers - specify type & hash")
	newUi("kill", false, kill_node, "Kill the node. WARNING: not safe - use 'quit' instead")
	newUi("mem", false, show_mem, "Show memory stats and... [bs|free|gc|<new_gc_perc>|<new_limit>MB]")
	newUi("pend", true, show_pending, "Show pending blocks")
	newUi("purge", true, purge_utxo, "Purge all unspendable outputs from UTXO database")
	newUi("quit q", false, ui_quit, "Quit the node: [restart]")
	newUi("redo", true, redo_block, "Redo one block")
	newUi("savebl bl", false, dump_block, "Saves a block to disk: <hash>")
	newUi("saveutxo s", true, save_utxo, "Save UTXO database now")
	newUi("trust", true, switch_trust, "Assume all downloaded blocks trusted: 0|1")
	newUi("undo", true, undo_block, "Undo one block")
	newUi("utxo u", true, blchain_utxodb, "Display UTXO-db statistics [mem]")
	newUi("web", true, webui_stats, "Show WebUI access statistics")
}
