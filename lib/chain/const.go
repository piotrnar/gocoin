package chain

const(
	BlockMapInitLen = 500e3
	MovingCheckopintDepth = 2016  // Do not accept forks that wold go deeper in a past
	BIP16SwitchTime = 1333238400 // BIP16 didn't become active until Apr 1 2012
	COINBASE_MATURITY = 100
	MedianTimeSpan = 11
)
