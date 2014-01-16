package main

import (
	"runtime"
)

var (
	BlockMutexFile string
	BlockMutexLine int
)

func BlocksMutex_Lock() {
	BlocksMutex.Lock()
	_, BlockMutexFile, BlockMutexLine, _ = runtime.Caller(1)
}

func BlocksMutex_Unlock() {
	_, f, l, _ := runtime.Caller(1)
	BlockMutexFile = f+"-done"
	BlockMutexLine = l
	BlocksMutex_Unlock()
}
