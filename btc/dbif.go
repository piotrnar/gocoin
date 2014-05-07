package btc

import (
	"github.com/piotrnar/gocoin/qdb"
)

// Used during BrowseUTXO()
const (
	WALK_ABORT   = 0x00000001 // Abort browsing
	WALK_NOMORE  = 0x00000002 // Do not browse through it anymore
)
type FunctionWalkUnspent func(*qdb.DB, qdb.KeyType, *OneWalkRecord) uint32

// Used to pass block's changes to UnspentDB
type BlockChanges struct {
	Height uint32
	LastKnownHeight uint32  // put here zero to disable this feature
	AddedTxs map[TxPrevOut] *TxOut
	DeledTxs map[TxPrevOut] *TxOut
}

// If TxNotifyFunc is set, it will be called each time a new unspent
// output is being added or removed. When being removed, TxOut is nil.
type TxNotifyFunc func (*TxPrevOut, *TxOut)
