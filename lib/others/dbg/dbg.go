package dbg

const (
	WASTED = 1<<0
	UNSPENT = 1<<1
	BLOCKS = 1<<2
	ORPHAS = 1<<3
	TX = 1<<4
	SCRIPT = 1<<5
	VERIFY = 1<<6
	SCRERR = 1<<7
)

var dbgmask uint32 = 0

func IsOn(b uint32) bool {
	return (dbgmask&b)!=0
}

func DbgSwitch(b uint32, on bool) {
	if on {
		dbgmask |= b
	} else {
		dbgmask ^= (b&dbgmask)
	}
}
