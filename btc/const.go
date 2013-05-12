package btc

import "runtime"

const(
	MAX_BLOCK_SIZE = 1000000
	COIN = 1e8
	MAX_MONEY = 21000000 * COIN

	BlockMapInitLen = 300e3
	UnspentTxsMapInitLen = 4e6

	MessageMagic = "Bitcoin Signed Message:\n"
)

var useThreads int = 3 * runtime.NumCPU() // use few times more go-routines to optimize an idle time

var taskDone chan bool

func init() {
	taskDone = make(chan bool, useThreads)
}