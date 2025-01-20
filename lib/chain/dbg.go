package chain

const (
	DBG_WASTED  = 1 << 0
	DBG_UNSPENT = 1 << 1
	DBG_BLOCKS  = 1 << 2
	DBG_ORPHAS  = 1 << 3
	DBG_TX      = 1 << 4
	DBG_SCRIPT  = 1 << 5
	DBG_VERIFY  = 1 << 6
	DBG_SCRERR  = 1 << 7
)

var dbgmask uint32 = 0

func DbgSwitch(b uint32, on bool) {
	if on {
		dbgmask |= b
	} else {
		dbgmask ^= (b & dbgmask)
	}
}
