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

func init() {
	time.Sleep(60e9)
	fir := true
	go func() {
		time.Sleep(10e9)
		whe := atomic.LoadInt64(&BlockMutexTime)
		now := time.Now().UnixNano()
		if time.Duration(now-whe) > time.Minute {
			if fir {
				fir = false
				fmt.Println("\007\007\007")
			}
			fmt.Println("Mutex locked for too long", BlockMutexFile, BlockMutexLine)
		}
	}()
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
