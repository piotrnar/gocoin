package main

import (
	"fmt"
	"sync"
	"sort"
	"time"
	"github.com/piotrnar/gocoin/lib/btc"
)

var (
	_CNT map[string] uint = make(map[string] uint)
	cnt_mut sync.Mutex
	StallCount uint64
	EmptyInProgressCnt uint64
	LastBlockAsked uint32
)


func COUNTER(s string) {
	cnt_mut.Lock()
	_CNT[s]++
	cnt_mut.Unlock()
}


func print_counters() {
	var s string
	cnt_mut.Lock()
	ss := make([]string, len(_CNT))
	i := 0
	for k, v := range _CNT {
		ss[i] = fmt.Sprintf("%s=%d", k, v)
		i++
	}
	cnt_mut.Unlock()
	sort.Strings(ss)
	for i = range ss {
		s += "  "+ss[i]
	}
	fmt.Println(s)
	return
}

func print_stats() {
	sec := float64(time.Now().Sub(DlStartTime)) / 1e6
	fmt.Printf("Block:%d/%d/%d/%d (%d)  Pending:%d  InProgress:%d  ImMem:%d (%dMB)  "+
		"Conns:%d  [%.0f => %.0f KBps]  AvgSize:%d  EC_Ver:%d  Stall:%d/%d  %.1fmin  \n",
		TheBlockChain.BlockTreeEnd.Height, BlocksComplete, BlocksComplete, FetchBlocksTo,
		len(BlockQueue), len(BlocksToGet), len(BlocksInProgress), len(BlocksCached), BlocksCachedSize>>20,
		open_connection_count(),
		float64(DlBytesDownloaded)/sec, float64(DlBytesProcessed)/sec, avg_block_size(),
		btc.EcdsaVerifyCnt, StallCount, EmptyInProgressCnt, time.Now().Sub(StartTime).Minutes())
}
