package main

import (
	"fmt"
	"time"
	"runtime"
	"sync/atomic"
)

var (
	BlockMutexFile string
	BlockMutexLine int
	BlockMutexTime int64
)

func BlocksMutex_Monitor() {
	fir := true
	for {
		time.Sleep(10e9)
		whe := atomic.LoadInt64(&BlockMutexTime)
		if time.Duration(time.Now().UnixNano()-whe) > time.Minute {
			if fir {
				fir = false
				fmt.Println("\007\007\007")
			}
			fmt.Println("Mutex locked for too long", BlockMutexFile, BlockMutexLine)
		}
	}
}

func BlocksMutex_Lock() {
	BlocksMutex.Lock()
	_, BlockMutexFile, BlockMutexLine, _ = runtime.Caller(1)
	atomic.StoreInt64(&BlockMutexTime, time.Now().UnixNano())
}

func BlocksMutex_Unlock() {
	_, f, l, _ := runtime.Caller(1)
	BlockMutexFile = f+"-done"
	BlockMutexLine = l
	BlocksMutex.Unlock()
}
