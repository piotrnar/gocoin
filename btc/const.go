package btc

import "runtime"

const(
	MAX_BLOCK_SIZE = 1000000
	COIN = 1e8
	MAX_MONEY = 21000000 * COIN

	BlockMapInitLen = 300e3
	UnspentTxsMapInitLen = 4e6
)

var CpuCount int = runtime.NumCPU()

const useThreads = 16

var taskDone chan bool = make(chan bool, useThreads)

 
