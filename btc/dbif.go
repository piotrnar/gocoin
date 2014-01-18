package btc

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

type UnspentDB interface {
	CommitBlockTxs(*BlockChanges, []byte) error
	UndoBlockTransactions(uint32)
	GetLastBlockHash() []byte

	UnspentGet(out *TxPrevOut) (*TxOut, error)
	GetAllUnspent(addr []*BtcAddr, quick bool) AllUnspentTx

	Idle() bool
	Sync()
	Save()
	Close()
	GetStats() (string)

	SetTxNotify(TxNotifyFunc)
}

// Set automatically by a specific UnspentDB backend, in its init()
var NewUnspentDb func(string, bool) UnspentDB
