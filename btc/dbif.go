package btc

import "fmt"

// Returned by GetUnspentFromPkScr
type OneUnspentTx struct {
	Output TxPrevOut
	Value uint64
}

func (ou *OneUnspentTx) String() string {
	return fmt.Sprintf("%15.8f BTC from ", float64(ou.Value)/1e8) + ou.Output.String()
}

type BlockChanges struct {
	Height uint32
	AddedTxs map[TxPrevOut] *TxOut
	DeledTxs map[TxPrevOut] *TxOut
}

type UnspentDB interface {
	CommitBlockTxs(*BlockChanges, []byte) error
	UndoBlockTransactions(uint32, []byte) error
	GetLastBlockHash() []byte
	
	UnspentGet(out *TxPrevOut) (*TxOut, error)
	//GetAllUnspent(addr *BtcAddr) []OneUnspentTx

	Save()
	Close()
	Sync()
	GetStats() (string)
}

var NewUnspentDb func(string, bool) UnspentDB
