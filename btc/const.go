package btc

const(
	MAX_BLOCK_SIZE = 1000000
	COIN = 1e8
	MAX_MONEY = 21000000 * COIN

	BlockMapInitLen = 300e3
	UnspentTxsMapInitLen = 4e6
	
	UnwindBufferMaxHistory = 7*24*6  // Let's give it about one week
)

