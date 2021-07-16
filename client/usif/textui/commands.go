package textui

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
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
	"github.com/piotrnar/gocoin/client/network/peersdb"
	"github.com/piotrnar/gocoin/client/usif"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/qdb"
	"github.com/piotrnar/gocoin/lib/others/sys"
)

type oneUiCmd struct {
	cmds    []string // command name
	help    string   // a helf for this command
	sync    bool     // shall be executed in the blochcina therad
	handler func(pars string)
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
		for i := range cs {
			c.cmds = append(c.cmds, cs[i])
		}
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
	return false
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
	cached := network.CachedBlocksLen.Get()
	b2g_len := len(network.BlocksToGet)
	b2g_idx_len := len(network.IndexToBlocksToGet)
	network.MutexRcv.Unlock()

	fmt.Printf("Gocoin: %s,  Synced: %t (%d),  Uptime %s,  Peers: %d,  ECDSAs: %d %d %d\n",
		gocoin.Version, common.GetBool(&common.BlockChainSynchronized), network.HeadersReceived.Get(),
		time.Now().Sub(common.StartTime).String(), peersdb.PeerDB.Count(),
		btc.EcdsaVerifyCnt(), btc.SchnorrVerifyCnt(), btc.CheckPay2ContractCnt())

	// Memory used
	al, sy := sys.MemUsed()
	fmt.Printf("Heap_used: %d MB,  System_used: %d MB,  UTXO-X-mem: %d MB in %d recs,  Saving: %t\n", al>>20, sy>>20,
		common.Memory.Bytes>>20, common.Memory.Allocs, common.BlockChain.Unspent.WritingInProgress.Get())

	network.MutexRcv.Lock()
	fmt.Println("Last Header:", network.LastCommitedHeader.BlockHash.String(), "@", network.LastCommitedHeader.Height)
	network.MutexRcv.Unlock()

	common.Last.Mutex.Lock()
	fmt.Println("Last Block :", common.Last.Block.BlockHash.String(), "@", common.Last.Block.Height)
	fmt.Printf(" Time: %s (~%s),  Diff: %.0f,  Rcvd: %s ago\n",
		time.Unix(int64(common.Last.Block.Timestamp()), 0).Format("2006/01/02 15:04:05"),
		time.Unix(int64(common.Last.Block.GetMedianTimePast()), 0).Format("15:04:05"),
		btc.GetDifficulty(common.Last.Block.Bits()), time.Now().Sub(common.Last.Time).String())
	common.Last.Mutex.Unlock()

	network.Mutex_net.Lock()
	fmt.Printf("Blocks Queued: %d,  Cached: %d,  Discarded: %d,  To Get: %d/%d,  UTXO.db on disk: %d\n",
		len(network.NetBlocks), cached, discarded, b2g_len, b2g_idx_len,
		atomic.LoadUint32(&common.BlockChain.Unspent.CurrentHeightOnDisk))
	network.Mutex_net.Unlock()

	network.TxMutex.Lock()
	var sw_cnt, sw_bts uint64
	for _, v := range network.TransactionsToSend {
		if v.SegWit != nil {
			sw_cnt++
			sw_bts += uint64(v.Size)
		}
	}
	fmt.Printf("Txs in mempool: %d (%sB),  Using SegWit: %d (%sB),  Rejected: %d (%sB)\n",
		len(network.TransactionsToSend), common.UintToString(network.TransactionsToSendSize),
		sw_cnt, common.UintToString(sw_bts),
		len(network.TransactionsRejected), common.UintToString(network.TransactionsRejectedSize))
	fmt.Printf(" Wait4Input: %d (%sB),  SpentOuts: %d,  AvgFee: %.1f SpB,  Pending:%d/%d\n",
		len(network.WaitingForInputs), common.UintToString(network.WaitingForInputsSize),
		len(network.SpentOutputs), common.GetAverageFee(),
		len(network.TransactionsPending), len(network.NetTxs))
	network.TxMutex.Unlock()

	var gs debug.GCStats
	debug.ReadGCStats(&gs)
	usif.BlockFeesMutex.Lock()
	fmt.Println("Go version:", runtime.Version(), "  LastGC:", time.Now().Sub(gs.LastGC).String(),
		"  NumGC:", gs.NumGC,
		"  PauseTotal:", gs.PauseTotal.String())
	usif.BlockFeesMutex.Unlock()
}

func show_counters(par string) {
	common.CounterMutex.Lock()
	ck := make([]string, 0)
	for k, _ := range common.Counter {
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

func show_pending(par string) {
	network.MutexRcv.Lock()
	out := make([]string, len(network.BlocksToGet))
	var idx int
	for _, v := range network.BlocksToGet {
		out[idx] = fmt.Sprintf(" * %d / %s / %d in progress", v.Block.Height, v.Block.Hash.String(), v.InProgress)
		idx++
	}
	network.MutexRcv.Unlock()
	sort.Strings(out)
	for _, s := range out {
		fmt.Println(s)
	}
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
	al, sy := sys.MemUsed()

	fmt.Println("Allocated:", al>>20, "MB")
	fmt.Println("SystemMem:", sy>>20, "MB")

	if p == "" {
		return
	}
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
	if h == nil {
		println("Specify block's hash")
		return
	}
	crec, _, er := common.BlockChain.Blocks.BlockGetExt(btc.NewUint256(h.Hash[:]))
	if er != nil {
		println("BlockGetExt:", er.Error())
		return
	}

	ioutil.WriteFile(h.String()+".bin", crec.Data, 0700)
	fmt.Println("Block saved")

	if crec.Block == nil {
		crec.Block, _ = btc.NewBlock(crec.Data)
	}
	/*
		if crec.Block.NoWitnessData == nil {
			crec.Block.BuildNoWitnessData()
		}
		if !bytes.Equal(crec.Data, crec.Block.NoWitnessData) {
			ioutil.WriteFile(h.String()+".old", crec.Block.NoWitnessData, 0700)
			fmt.Println("Old block saved")
		}
	*/

}

func ui_quit(par string) {
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
	d, e := ioutil.ReadFile(common.ConfigFile)
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

func show_addresses(par string) {
	pars := strings.Split(par, " ")
	if len(pars) < 1 || pars[0] == "" {
		bcnt, acnt := 0, 0
		peersdb.PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
			pr := peersdb.NewPeer(v)
			if pr.Banned != 0 {
				bcnt++
			}
			if pr.SeenAlive {
				acnt++
			}
			return 0
		})
		fmt.Println("Peers in DB:", peersdb.PeerDB.Count())
		fmt.Println("Peers seen alive:", acnt)
		fmt.Println("Peers banned:", bcnt)
		return
	}

	var only_ban, only_alive, show_help bool
	var ban_reason string
	limit := peersdb.PeerDB.Count()
	check_next := true
	switch pars[0] {
	case "ali":
		only_alive = true
	case "ban":
		only_ban = true
	case "help":
		show_help = true
	case "all":
		// do nothing
	case "purge":
		var torem []qdb.KeyType
		peersdb.PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
			pr := peersdb.NewPeer(v)
			if !pr.SeenAlive {
				torem = append(torem, k)
			}
			return 0
		})
		fmt.Println("Purginig", len(torem), "records")
		for _, k := range torem {
			peersdb.PeerDB.Del(k)
		}
		show_addresses("")
		return

	case "defrag":
		peersdb.PeerDB.Defrag(false)
		return

	case "save":
		peersdb.PeerDB.Sync()
		return

	default:
		if u, e := strconv.ParseUint(pars[0], 10, 32); e != nil {
			fmt.Println("Incorrect number A of peers max count:", e.Error())
			show_help = true
		} else {
			limit = int(u)
			check_next = false
		}
	}
	if check_next {
		if len(pars) >= 2 {
			if u, e := strconv.ParseUint(pars[1], 10, 32); e != nil {
				if only_ban {
					ban_reason = pars[1]
					if len(pars) >= 3 {
						if u, e := strconv.ParseUint(pars[2], 10, 32); e == nil {
							limit = int(u)
						}
					}
				} else {
					fmt.Println("Incorrect number B of peers max count:", e.Error())
					show_help = true
				}
			} else {
				limit = int(u)
			}
		}
	}

	if show_help {
		fmt.Println("Use 'peers all [max_cnt]' or 'peers [max_cnt]' to list all")
		fmt.Println("Use 'peers ali [max_cnt]' to list only those seen alive")
		fmt.Println("Use 'peers ban [ban_reason] [max_cnt]' to list the banned ones")
		fmt.Println("Use 'peers purge' to remove all peers never seen alive")
		fmt.Println("Use 'peers defrag' to defragment DB")
		fmt.Println("Use 'peers save' to save DB now")
		return
	}
	prs := peersdb.GetRecentPeers(uint(limit), true, func(p *peersdb.PeerAddr) bool {
		if only_alive && !p.SeenAlive {
			return true
		}
		if only_ban {
			if p.Banned == 0 || ban_reason != "" && p.BanReason != ban_reason {
				return true
			}
			p.Time = p.Banned // to sort them by when they were banned
		}
		return false
	})
	for i := range prs {
		fmt.Printf("%4d) %s", i+1, prs[i].String())
		if network.ConnectionActive(prs[i]) {
			fmt.Print("  CONNECTED")
		}
		fmt.Print("\n")
	}
}

func unban_peer(par string) {
	fmt.Print(usif.UnbanPeer(par))
}

func ban_peer(par string) {
	ad, er := peersdb.NewAddrFromString(par, false)
	if er != nil {
		fmt.Println(par, er.Error())
		return
	}
	ad.Ban("FromTUI")
}

func add_peer(par string) {
	ad, er := peersdb.NewAddrFromString(par, false)
	if er != nil {
		fmt.Println(par, er.Error())
		return
	}
	ad.Time = uint32(time.Now().Unix())
	ad.Save()
}

func show_cached(par string) {
	var hi, lo uint32
	for _, v := range network.CachedBlocks {
		//fmt.Printf(" * %s -> %s\n", v.Hash.String(), btc.NewUint256(v.ParentHash()).String())
		if hi == 0 {
			hi = v.Block.Height
			lo = v.Block.Height
		} else if v.Block.Height > hi {
			hi = v.Block.Height
		} else if v.Block.Height < lo {
			lo = v.Block.Height
		}
	}
	fmt.Println(len(network.CachedBlocks), "block cached with heights", lo, "to", hi, hi-lo)
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

func analyze_bip9(par string) {
	all := par == "all"
	n := common.BlockChain.BlockTreeRoot
	for n != nil {
		var i uint
		start_block := uint(n.Height)
		start_time := n.Timestamp()
		bits := make(map[byte]uint32)
		for i = 0; i < 2016 && n != nil; i++ {
			ver := n.BlockVersion()
			if (ver & 0x20000000) != 0 {
				for bit := byte(0); bit <= 28; bit++ {
					if (ver & (1 << bit)) != 0 {
						bits[bit]++
					}
				}
			}
			n = n.FindPathTo(common.BlockChain.LastBlock())
		}
		if len(bits) > 0 {
			var s string
			for k, v := range bits {
				if all || v >= common.BlockChain.Consensus.BIP9_Treshold {
					if s != "" {
						s += " | "
					}
					s += fmt.Sprint(v, " x bit(", k, ")")
				}
			}
			if s != "" {
				fmt.Println("Period from", time.Unix(int64(start_time), 0).Format("2006/01/02 15:04"),
					" block #", start_block, "-", start_block+i-1, ":", s, " - active from", start_block+2*2016)
			}
		}
	}
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

func purge_utxo(par string) {
	common.BlockChain.Unspent.PurgeUnspendable(true)
	if !common.CFG.Memory.PurgeUnspendableUTXO {
		fmt.Println("Save your config file (cs) to have all the futher records purged automatically.")
		common.CFG.Memory.PurgeUnspendableUTXO = true
	}
}

func undo_block(par string) {
	common.BlockChain.UndoLastBlock()
	common.Last.Mutex.Lock()
	common.Last.Block = common.BlockChain.LastBlock()
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
		println("ERROR: FindPathTo failed")
		return
	}

	if nxt.BlockSize == 0 {
		println("ERROR: BlockSize is zero - corrupt database")
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

	bl.Trusted = false // assume not trusted

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
	os.Exit(1)
}

func init() {
	newUi("ban", false, ban_peer, "Ban a peer specified by IP")
	newUi("bchain b", true, blchain_stats, "Display blockchain statistics")
	newUi("bip9", true, analyze_bip9, "Analyze current blockchain for BIP9 bits (add 'all' to see more)")
	newUi("cache", true, show_cached, "Show blocks cached in memory")
	newUi("configload cl", false, load_config, "Re-load settings from the common file")
	newUi("configsave cs", false, save_config, "Save current settings to a common file")
	newUi("configset cfg", false, set_config, "Set a specific common value - use JSON, omit top {}")
	newUi("counters c", false, show_counters, "Show all kind of debug counters")
	newUi("dlimit dl", false, set_dlmax, "Set maximum download speed. The value is in KB/second - 0 for unlimited")
	newUi("help h ?", false, show_help, "Shows this help")
	newUi("info i", false, show_info, "Shows general info about the node")
	newUi("inv", false, send_inv, "Send inv message to all the peers - specify type & hash")
	newUi("kill", true, kill_node, "Kill the node. WARNING: not safe - use 'quit' instead")
	newUi("mem", false, show_mem, "Show detailed memory stats (optionally free, gc or a numeric param)")
	newUi("peeradd", false, add_peer, "Add a peer to the database, mark it as alive")
	newUi("peers p", false, show_addresses, "Operation on pers database ('peers help' for help)")
	newUi("pend", false, show_pending, "Show pending blocks, to be fetched")
	newUi("purge", true, purge_utxo, "Purge all unspendable outputs from UTXO database")
	newUi("quit q", false, ui_quit, "Quit the node")
	newUi("redo", true, redo_block, "Redo last block")
	newUi("savebl", false, dump_block, "Saves a block with a given hash to a binary file")
	newUi("saveutxo s", true, save_utxo, "Save UTXO database now")
	newUi("trust t", true, switch_trust, "Assume all donwloaded blocks trusted (1) or un-trusted (0)")
	newUi("ulimit ul", false, set_ulmax, "Set maximum upload speed. The value is in KB/second - 0 for unlimited")
	newUi("unban", false, unban_peer, "Unban a peer specified by IP[:port] (or 'unban all')")
	newUi("undo", true, undo_block, "Undo last block")
	newUi("utxo u", true, blchain_utxodb, "Display UTXO-db statistics")
}
