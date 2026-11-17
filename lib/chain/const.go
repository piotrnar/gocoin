package chain

const(
	BlockMapInitLen = 500e3
	MovingCheckopintDepth = 2016  // Do not accept forks that wold go deeper in a past
	BIP16SwitchTime = 1333238400 // BIP16 didn't become active until Apr 1 2012
	COINBASE_MATURITY = 100
	MedianTimeSpan = 11
	MaxTimewarp = 600 // BIP94: how many seconds the first block of a difficulty period can go back in time
	BIP34ImpliesBIP30Limit = 1983702 // from this height a coinbase can (in theory) duplicate a pre-BIP34 one
)
