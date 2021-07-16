package qdb

import (
	"fmt"
	"sort"
)

func (db *DB) cnt(k string) {
	db.cntadd(k, 1)
}

func (db *DB) cntadd(k string, val uint64) {
	db.counter_mutex.Lock()
	db.counter[k] += val
	db.counter_mutex.Unlock()
}

func (db *DB) GetStats() (s string) {
	db.counter_mutex.Lock()
	ck := make([]string, len(db.counter))
	idx := 0
	for k, _ := range db.counter {
		ck[idx] = k
		idx++
	}
	sort.Strings(ck)

	for i := range ck {
		k := ck[i]
		v := db.counter[k]
		if s != "" {
			s += ", "
		}
		s += fmt.Sprint(k, "=", v)
	}
	db.counter_mutex.Unlock()
	return s
}
