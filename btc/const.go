package btc

import "runtime"

const(
	SourcesTag = "0.2.10"

	MAX_BLOCK_SIZE = 1000000
	COIN = 1e8
	MAX_MONEY = 21000000 * COIN

	BlockMapInitLen = 300e3

	MessageMagic = "Bitcoin Signed Message:\n"
)

// Increase the number of threads to optimize txs verification time,
// proportionaly among cores, but if you set it too high, the UI and
// network threads may be laggy while parsing blocks.
var UseThreads int = 3 * runtime.NumCPU()
