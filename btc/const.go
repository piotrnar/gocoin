package btc

import "runtime"

const(
	SourcesTag = "0.0.6"

	MAX_BLOCK_SIZE = 1000000
	COIN = 1e8
	MAX_MONEY = 21000000 * COIN

	BlockMapInitLen = 300e3

	MessageMagic = "Bitcoin Signed Message:\n"
)

// Increase the number of threads to optimize txs verification time,
// but if you set it too high, the UI may be non-responsive
// while parsing more complex blocks.
var useThreads int = 3 * runtime.NumCPU()

var taskDone chan bool

func init() {
	taskDone = make(chan bool, useThreads)
}