package main

import (
	"fmt"
	"sync"
	"sort"
    "time"
)

var (
	_CNT map[string] uint = make(map[string] uint)
	cnt_mut sync.Mutex
)


func COUNTER(s string) {
	cnt_mut.Lock()
	_CNT[s]++
	cnt_mut.Unlock()
}


func stats() (s string) {
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
	return
}

func print_stats() {
	BlocksMutex.Lock()
	indx := BlocksIndex
	inpr := len(BlocksInProgress)
	cach := len(BlocksCached)
	toge := len(BlocksToGet)
	bcmp := BlocksComplete
	camb := BlocksCachedSize>>20
	BlocksMutex.Unlock()
	sec := float64(time.Now().Sub(DlStartTime)) / 1e3
	fmt.Printf("Block:%d/%d  Pending:%d  InProgress:%d  Index:%d  Memory:%d/%dMB  Conns:%d  Dload:%.1fMB/s  Output:%.1fMB/s  AvgSize:%d  %.1fmin\n %s\n",
		bcmp, LastBlockHeight, toge, inpr, indx, cach, camb, open_connection_count(),
		float64(DlBytesDownloaded)/sec, float64(DlBytesProcesses)/sec, avg_block_size(),
		time.Now().Sub(StartTime).Minutes(), stats())
}
