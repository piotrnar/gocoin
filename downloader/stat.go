package main

import (
	"fmt"
	"sync"
	"sort"
	"time"
	"sync/atomic"
	"github.com/piotrnar/gocoin/lib/btc"
)

var (
	_CNT map[string] uint = make(map[string] uint)
	cnt_mut sync.Mutex
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
	BlocksMutex.Lock()
	s := fmt.Sprintf("Block:%d/%d/%d/"+
		"%d (%d)  Pending:%d  "+
		"InProgress:%d (Empty:%d)  "+
		"ImMem:%d (%dMB)  AvgBLen:%d  "+
		"Conns:%d  [%.0f => %.0f KBps]  "+
		"ECnt:%d  %.1fmin",
		atomic.LoadUint32(&LastStoredBlock), BlocksComplete, FetchBlocksTo,
		LastBlockHeight, len(BlockQueue), len(BlocksToGet),
		len(BlocksInProgress), EmptyInProgressCnt,
		len(BlocksCached), BlocksCachedSize>>20, avg_block_size(),
		open_connection_count(), float64(DlBytesDownloaded)/sec, float64(DlBytesProcessed)/sec,
		btc.EcdsaVerifyCnt, time.Now().Sub(StartTime).Minutes())
	BlocksMutex.Unlock()
	fmt.Println(s)
}
