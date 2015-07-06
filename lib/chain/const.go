package chain

const(
	BlockMapInitLen = 300e3
	MovingCheckopintDepth = 2016  // Do not accept forks that wold go deeper in a past
	GenesisBlockTime = 1231006505
	BIP16SwitchTime = 1333238400 // BIP16 didn't become active until Apr 1 2012
	ForceBlockVer2From = 200000 // we just use fixed block number and do not enfoce it for testnet
	COINBASE_MATURITY = 100
	ForceBlockVer3From = 364000 // we just use fixed block number and do not enfoce it for testnet
)
