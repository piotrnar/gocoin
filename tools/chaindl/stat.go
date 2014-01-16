package main

import (
	"fmt"
	"sync"
	"sort"
	"time"
	"github.com/piotrnar/gocoin/btc"
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
	println(s)
	return
}

func print_stats() {
	BlocksMutex_Lock()
	indx := BlocksIndex
	inpr := len(BlocksInProgress)
	cach := len(BlocksCached)
	toge := len(BlocksToGet)
	bcmp := BlocksComplete
	camb := BlocksCachedSize>>20
	BlocksMutex_Unlock()
	sec := float64(time.Now().Sub(DlStartTime)) / 1e6
	fmt.Printf("Block:%d/%d  Pending:%d  InProgress:%d  Index:%d  Memory:%d/%dMB  Conns:%d  Dload:%.0fKB/s  Output:%.0fKB/s  AvgSize:%d  %.1fmin  EC_Ver:%d\n",
		bcmp, LastBlockHeight, toge, inpr, indx, cach, camb, open_connection_count(),
		float64(DlBytesDownloaded)/sec, float64(DlBytesProcesses)/sec, avg_block_size(),
		time.Now().Sub(StartTime).Minutes(), btc.EcdsaVerifyCnt)
}
