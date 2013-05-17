package btc

import "fmt"

type AllUnspentTx []OneUnspentTx

// Returned by GetUnspentFromPkScr
type OneUnspentTx struct {
	TxPrevOut
	Value uint64
	AskIndex uint32
	MinedAt uint32
}

func (ou *OneUnspentTx) String() string {
	return fmt.Sprintf("%15.8f BTC from ", float64(ou.Value)/1e8) + ou.TxPrevOut.String()
}

// Used to pass block's changes to UnspentDB
type BlockChanges struct {
	Height uint32
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

	Idle()
	Save()
	Close()
	NoSync()
	Sync()
	GetStats() (string)

	SetTxNotify(TxNotifyFunc)
}

// Set automatically by a specific UnspentDB backend, in its init()
var NewUnspentDb func(string, bool) UnspentDB
