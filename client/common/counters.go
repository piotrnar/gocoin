package common

import (
	"sync"
)

var (
	CounterMutex sync.Mutex
	Counter map[string] uint64 = make(map[string]uint64)

	BusyWith string
	Busy_mutex sync.Mutex
)

func CountSafe(k string) {
	CounterMutex.Lock()
	Counter[k]++
	CounterMutex.Unlock()
}

func CountSafeAdd(k string, val uint64) {
	CounterMutex.Lock()
	Counter[k] += val
	CounterMutex.Unlock()
}


func Busy(b string) {
	Busy_mutex.Lock()
	BusyWith = b
	Busy_mutex.Unlock()
}
