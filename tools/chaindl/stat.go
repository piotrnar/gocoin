package main

import (
	"fmt"
	"sync"
)

var (
	_CNT map[string] uint = make(map[string] uint)
	cnt_mut sync.Mutex
)

func show_peers() {
	open_connection_mutex.Lock()
	println("********************* peers *********************", open_connection_count())
	for _, v := range open_connection_list {
		fmt.Println(" -", v.peerip, v.isbroken(), v.closed_s, v.closed_r)
	}
	open_connection_mutex.Unlock()
}


func COUNTER(s string) {
	cnt_mut.Lock()
	_CNT[s]++
	cnt_mut.Unlock()
}


func stats() (s string) {
	cnt_mut.Lock()
	for k, v := range _CNT {
		s += fmt.Sprintf("  %s=%d", k, v)
	}
	cnt_mut.Unlock()
	return
}
