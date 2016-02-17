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
	var max_block_height_in_progress uint32
	for _, v := range BlocksInProgress {
		if v.Height>max_block_height_in_progress {
			max_block_height_in_progress = v.Height
		}
	}
	s := fmt.Sprintf("Block:%d/%d/%d/%d  Pending:%d  Processing:%d  Downloading:%d  "+
		"Cached:%d (%dMB)  AvgSize:%dkB  Conns:%d  [%.0f => %.0f KBps]  ECnt:%d  %.1fmin",
		atomic.LoadUint32(&LastStoredBlock), BlocksComplete, max_block_height_in_progress,
		LastBlockHeight, len(BlocksToGet), len(BlockQueue),len(BlocksInProgress),
		len(BlocksCached), BlocksCachedSize>>20, avg_block_size()/1000,
		open_connection_count(), float64(DlBytesDownloaded)/sec, float64(DlBytesProcessed)/sec,
		btc.EcdsaVerifyCnt, time.Now().Sub(StartTime).Minutes())
	BlocksMutex.Unlock()
	fmt.Println(s)
}
