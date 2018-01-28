package qdb

import (
	"fmt"
	"sync"
	"sort"
)

var (
	counter map[string] uint64 = make(map[string]uint64)
	counter_mutex sync.Mutex
)


func cnt(k string) {
	cntadd(k, 1)
}

func cntadd(k string, val uint64) {
	counter_mutex.Lock()
	counter[k] += val
	counter_mutex.Unlock()
}


func GetStats() (s string) {
	counter_mutex.Lock()
	ck := make([]string, len(counter))
	idx := 0
	for k, _ := range counter {
		ck[idx] = k
		idx++
	}
	sort.Strings(ck)

	for i := range ck {
		k := ck[i]
		v := counter[k]
		s += fmt.Sprintln(k, ": ", v)
	}
	counter_mutex.Unlock()
	return s
}
