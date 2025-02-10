package textui

import (
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/peersdb"
	"github.com/piotrnar/gocoin/client/txpool"
)

type SortedKeys []struct {
	Key    uint64
	ConnID uint32
}

func (sk SortedKeys) Len() int {
	return len(sk)
}

func (sk SortedKeys) Less(a, b int) bool {
	return sk[a].ConnID < sk[b].ConnID
}

func (sk SortedKeys) Swap(a, b int) {
	sk[a], sk[b] = sk[b], sk[a]
}

func net_drop(par string) {
	conid, e := strconv.ParseUint(par, 10, 32)
	if e != nil {
		println(e.Error())
		return
	}
	network.DropPeer(uint32(conid))
}

func node_info(par string) {
	conid, e := strconv.ParseUint(par, 10, 32)
	if e != nil {
		return
	}

	var r network.ConnInfo

	if v := network.GetConnFromID(uint32(conid)); v != nil {
		v.GetStats(&r)
	} else {
		fmt.Println("Connection ID", conid, "not found")
		return
	}

	fmt.Printf("Connection ID %d:\n", r.ID)
	if r.Incomming {
		fmt.Println("Coming from", r.PeerIp)
	} else {
		fmt.Println("Going to", r.PeerIp)
	}
	if !r.ConnectedAt.IsZero() {
		fmt.Println("Connected at", r.ConnectedAt.Format("2006-01-02 15:04:05"))
		if r.Version != 0 {
			fmt.Println("Node Version:", r.Version, "/ Services:", fmt.Sprintf("0x%x", r.Services))
			fmt.Println("User Agent:", r.Agent)
			fmt.Println("Chain Height:", r.Height)
			fmt.Printf("Reported IP: %d.%d.%d.%d\n", byte(r.ReportedIp4>>24), byte(r.ReportedIp4>>16),
				byte(r.ReportedIp4>>8), byte(r.ReportedIp4))
			fmt.Println("SendHeaders:", r.SendHeaders)
		}
		fmt.Println("Invs Done:", r.InvsDone)
		fmt.Println("Last data got:", time.Since(r.LastDataGot).String())
		fmt.Println("Last data sent:", time.Since(r.LastSent).String())
		fmt.Println("Last command received:", r.LastCmdRcvd, " ", r.LastBtsRcvd, "bytes")
		fmt.Println("Last command sent:", r.LastCmdSent, " ", r.LastBtsSent, "bytes")
		fmt.Print("Invs  Recieved:", r.InvsRecieved, "  Pending:", r.InvsToSend, "\n")
		fmt.Print("Bytes to send:", r.BytesToSend, " (", r.MaxSentBufSize, " max)\n")
		fmt.Print("BlockInProgress:", r.BlocksInProgress, "  GetHeadersInProgress:", r.GetHeadersInProgress, "\n")
		fmt.Println("GetBlocksDataNow:", r.GetBlocksDataNow)
		fmt.Println("AllHeadersReceived:", r.AllHeadersReceived)
		fmt.Println("Total Received:", r.BytesReceived, " /  Sent:", r.BytesSent)
		mts := make(map[string]bool)
		var had bool
		for k, v := range r.Counters {
			if len(k) > 5 && strings.HasPrefix(k, "rbts_") || strings.HasPrefix(k, "sbts_") ||
				strings.HasPrefix(k, "rcvd_") || strings.HasPrefix(k, "sent_") {
				mts[k[5:]] = true
			} else {
				fmt.Print(k, ":", v, "  ")
				had = true
			}
		}
		if had {
			fmt.Println()
		}
		mms := make([]string, 0, len(mts))
		for k := range mts {
			mms = append(mms, k)
		}
		sort.Strings(mms)
		for _, msg := range mms {
			fmt.Printf("\t%-12s      R:[%8d | %10s]     S:[%8d | %10s]\n", msg,
				r.Counters["rcvd_"+msg], common.BytesToString(r.Counters["rbts_"+msg]),
				r.Counters["sent_"+msg], common.BytesToString(r.Counters["sbts_"+msg]))
		}
	} else {
		fmt.Println("Not yet connected")
	}
}

func net_conn(par string) {
	ad, er := peersdb.NewAddrFromString(par, false)
	if er != nil {
		fmt.Println(par, er.Error())
		return
	}
	fmt.Println("Connecting to", ad.Ip())
	ad.Manual = true
	network.DoNetwork(ad)
}

func net_stats(par string) {
	if par == "bw" {
		common.PrintBWStats()
		return
	} else if par != "" {
		node_info(par)
		return
	}

	network.Mutex_net.Lock()
	fmt.Printf("%d active net connections, %d outgoing\n", len(network.OpenCons), network.OutConsActive)
	srt := make(SortedKeys, len(network.OpenCons))
	cnt := 0
	for k, v := range network.OpenCons {
		srt[cnt].Key = k
		srt[cnt].ConnID = v.ConnID
		cnt++
	}
	sort.Sort(srt)
	for idx := range srt {
		v := network.OpenCons[srt[idx].Key]
		v.Mutex.Lock()
		fmt.Printf("%8d) ", v.ConnID)

		if v.X.Incomming {
			fmt.Print("<- ")
		} else {
			fmt.Print(" ->")
		}
		fmt.Printf(" %21s %5dms", v.PeerAddr.Ip(), v.GetAveragePing())
		//fmt.Printf(" %7d : %-16s %7d : %-16s", v.X.LastBtsRcvd, v.X.LastCmdRcvd, v.X.LastBtsSent, v.X.LastCmdSent)
		fmt.Printf(" %10s %10s", common.BytesToString(v.X.BytesReceived), common.BytesToString(v.X.BytesSent))
		if len(v.GetBlockInProgress) != 0 {
			fmt.Printf(" %4d", len(v.GetBlockInProgress))
		} else {
			fmt.Print("     ")
		}
		fmt.Print("  ", v.Node.Agent)
		if v.X.IsSpecial {
			fmt.Print(" F")
		}

		if b2s := v.BytesToSent(); b2s > 0 {
			fmt.Print("  ", b2s)
		}
		v.Mutex.Unlock()
		fmt.Println()
	}

	if network.ExternalAddrLen() > 0 {
		fmt.Print("External addresses:")
		network.ExternalIpMutex.Lock()
		for ip, cnt := range network.ExternalIp4 {
			fmt.Printf(" %d.%d.%d.%d(%d)", byte(ip>>24), byte(ip>>16), byte(ip>>8), byte(ip), cnt)
		}
		network.ExternalIpMutex.Unlock()
		fmt.Println()
	} else {
		fmt.Println("No known external address")
	}

	network.Mutex_net.Unlock()

	fmt.Println("GetMPInProgress:", len(txpool.GetMPInProgressTicket) != 0)

	common.PrintBWStats()
}

func net_rd(par string) {
	network.HammeringMutex.Lock()
	srt := make([]string, len(network.RecentlyDisconencted))
	var idx int
	for ip, rd := range network.RecentlyDisconencted {
		srt[idx] = fmt.Sprintf("%31d %16s %3d %16s - %s", rd.Time.UnixNano(),
			fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3]), rd.Count,
			time.Since(rd.Time).String(), rd.Why)
		idx++
	}
	sort.Strings(srt)
	network.HammeringMutex.Unlock()
	fmt.Println("Recently disconencted incoming connections:")
	for _, s := range srt {
		fmt.Println(s[32:])
	}
	fmt.Println("Baned if", network.HammeringMaxAllowedCount+1, "conections (within", network.HammeringMinReconnect,
		"...", network.HammeringMinReconnect+network.HammeringExpirePeriod, "in between)")
}

func net_friends(par string) {
	network.FriendsAccess.Lock()
	for _, pk := range network.AuthPubkeys {
		fmt.Println("Pubkey", hex.EncodeToString(pk))
	}
	for _, ua := range network.SpecialAgents {
		fmt.Println("Agent Prefix", ua)
	}
	for _, p := range network.SpecialIPs {
		fmt.Println("IP ", fmt.Sprintf("%d.%d.%d.%d", p[0], p[1], p[2], p[3]))
	}
	network.FriendsAccess.Unlock()
}

func print_fetch_counters() {
	network.MutexRcv.Lock()
	defer network.MutexRcv.Unlock()
	fmt.Print("\t")
	fmt.Print("NoBlocksToGet:", network.Fetch.NoBlocksToGet)
	fmt.Print(",  HasBlocksExpired:", network.Fetch.HasBlocksExpired)
	fmt.Print(",  MaxCountInProgress:", network.Fetch.MaxCountInProgress)
	fmt.Print("\n\t")
	fmt.Print("MaxBytesInProgress:", network.Fetch.MaxBytesInProgress)
	fmt.Print(",  NoWitness:", network.Fetch.NoWitness)
	fmt.Print(",  Nothing:", network.Fetch.Nothing)
	fmt.Print("\n\t")
	fmt.Print("ReachEndOfLoop:", network.Fetch.ReachEndOfLoop)
	fmt.Print(",  ReachMaxCnt:", network.Fetch.ReachMaxCnt)
	fmt.Print(",  ReachMaxData:", network.Fetch.ReachMaxData)
	fmt.Print("\n\tBlksCntMax   ")
	for i, v := range network.Fetch.BlksCntMax {
		if v != 0 {
			fmt.Print("  x", i, ":", v)
		}
	}
	fmt.Println()
}

func sync_stats(par string) {
	common.Last.Mutex.Lock()
	lb := common.Last.Block.Height
	common.Last.Mutex.Unlock()

	network.MutexRcv.Lock()
	li2get := network.LowestIndexToBlocksToGet
	network.MutexRcv.Unlock()

	network.CachedBlocksMutex.Lock()
	lencb := len(network.CachedBlocks)
	network.CachedBlocksMutex.Unlock()

	fmt.Printf("@%d\tReady: %d   InCacheCnt: %d   Avg.Bl.Size: %d   EmptyCache: %d\n",
		lb, li2get-lb-1, lencb,
		common.AverageBlockSize.Get(), network.Fetch.CacheEmpty)
	tot := common.DlBytesTotal
	if tot > 0 {
		wst := network.Fetch.BlockBytesWasted
		fmt.Printf("\tWasted %d blocks carrying %d / %dMB, which was %.2f%% of total DL bandwidth\n",
			network.Fetch.BlockSameRcvd, wst>>20, tot>>20, 100*float64(wst)/float64(tot))
	}

	if strings.Contains(par, "c") {
		var lowest_cached_height, highest_cached_height uint32
		var ready_cached_cnt uint32
		var cached_ready_bytes int
		m := make(map[uint32]*network.BlockRcvd)
		for _, b := range network.CachedBlocks {
			bh := b.BlockTreeNode.Height
			m[bh] = b
			if lowest_cached_height == 0 {
				lowest_cached_height, highest_cached_height = bh, bh
			} else if b.BlockTreeNode.Height < lowest_cached_height {
				lowest_cached_height = bh
			} else if bh > highest_cached_height {
				highest_cached_height = bh
			}
		}
		for {
			if b, ok := m[lb+ready_cached_cnt+1]; ok {
				ready_cached_cnt++
				cached_ready_bytes += b.Size
			} else {
				break
			}
		}
		fmt.Printf("\tCache Ready: %d:%dMB   Used: %d:%dMB   Limit: %dMB   Max Used: %d%%\n",
			ready_cached_cnt, cached_ready_bytes>>20, lencb, network.CachedBlocksBytes.Get()>>20,
			common.SyncMaxCacheBytes.Get()>>20,
			100*network.MaxCachedBlocksSize.Get()/common.SyncMaxCacheBytes.Get())
	} else {
		fmt.Printf("\tCache Used: %d:%dMB   Limit: %dMB   Max Used: %d%%\n",
			lencb, network.CachedBlocksBytes.Get()>>20, common.SyncMaxCacheBytes.Get()>>20,
			100*network.MaxCachedBlocksSize.Get()/common.SyncMaxCacheBytes.Get())
	}

	if strings.Contains(par, "x") {
		var bip_cnt, ip_min, ip_max uint32
		network.MutexRcv.Lock()
		for _, bip := range network.BlocksToGet {
			if bip.InProgress > 0 {
				if ip_min == 0 {
					ip_min = bip.BlockTreeNode.Height
					ip_max = ip_min
				} else if bip.BlockTreeNode.Height < ip_min {
					ip_min = bip.BlockTreeNode.Height
				} else if bip.BlockTreeNode.Height > ip_max {
					ip_max = bip.BlockTreeNode.Height
				}
				bip_cnt++
			}
		}
		network.MutexRcv.Unlock()
		fmt.Printf("\tIn Progress: %d, starting from %d, up to %d (%d)\n",
			bip_cnt, ip_min, ip_max, ip_max-ip_min)
	}

	if d := network.Fetc.HeightD; d != 0 {
		a := network.Fetc.HeightA
		if siz := d - a; siz > 0 {
			b := network.Fetc.HeightB
			c := network.Fetc.HeightC
			fil := b - a
			fmt.Printf("\tLast Fetch from %d:%d to %d:%d -> BTG:%d (ready %d%% of %d)\n", a, b, d, c,
				network.Fetc.B2G, 100*fil/siz, siz)
		}
	}

	max_blocks_at_once := common.Get(&common.CFG.Net.MaxBlockAtOnce)
	fmt.Print("\t")
	for i := uint32(0); i < max_blocks_at_once; i++ {
		cnt := network.Fetc.C[i]
		if cnt == 0 {
			fmt.Printf("  C%d:-", i)
		} else {
			fmt.Printf("  C%d:%d", i, cnt)
		}
	}
	fmt.Println()

	print_fetch_counters()
	if strings.Contains(par, "r") {
		common.CountSafeStore("BlocksUnderflowCount", 0)
		println("Error counter set to 0")
	}
}

func init() {
	newUi("net n", false, net_stats, "Show network statistics. Specify ID to see its details.")
	newUi("netconn conn", false, net_conn, "Connect to the given node (specify IP and optionally a port)")
	newUi("netdrop drop", false, net_drop, "Disconenct from node with a given IP")
	newUi("netfriend", false, net_friends, "Show current friends settings")
	newUi("netlimd dl", false, set_dlmax, "Set maximum download speed: 0|<max_kbps>")
	newUi("netlimu ul", false, set_ulmax, "Set maximum download speed: 0|<max_kbps>")
	newUi("netrd rd", false, net_rd, "Show recently disconnected incoming connections")
	newUi("netsync ss", true, sync_stats, "Show chain sync statistics. Add charecter 'c' (cache), 'x' (in progress) or 'r' (reset couter)")
}
