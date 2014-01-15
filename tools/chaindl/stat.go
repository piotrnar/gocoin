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
		s += " "+ss[i]
	}
	return
}

func print_stats() {
	AddrMutex.Lock()
	adrs := len(AddrDatbase)
	AddrMutex.Unlock()
	BlocksMutex.Lock()
	indx := BlocksIndex
	inpr := len(BlocksInProgress)
	cach := len(BlocksCached)
	BlocksMutex.Unlock()
	sec := float64(time.Now().Sub(DlStartTime)) / 1e6
	DlMutex.Lock()
	fmt.Printf("H:%d/%d  InPr:%d  Got:%d  Cons:%d/%d  Indx:%d  DL:%.1fKBps  PR:%.1fKBps  %s\n",
		BlocksComplete, LastBlockHeight, inpr, cach, open_connection_count(), adrs, indx,
		float64(DlBytesDownloaded)/sec, float64(DlBytesProcesses)/sec, stats())
	DlMutex.Unlock()
}
