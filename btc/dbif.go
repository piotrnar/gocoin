package btc


// Returned by GetUnspentFromPkScr
type OneUnspentTx struct {
	Output TxPrevOut
	Value uint64
}

type OneAddedTx struct {
	Tx_Adr *TxPrevOut  // This is the unspent input address (hash-vout)
	Val_Pk *TxOut      // This is the amount and the Pk_script
}

type BlockChanges struct {
	Height uint32
	AddedTxs [] *OneAddedTx
	DeledTxs [] *OneAddedTx
}

type BtcDB interface {
	// Call this one before rescanning
	UnspentPurge()

	CommitBlockTxs(*BlockChanges) error
	UndoBlockTransactions(uint32) error
	
	UnspentGet(out *TxPrevOut) (*TxOut, error)

	BlockAdd(height uint32, bl *Block) (error)
	BlockGet(hash *Uint256) ([]byte, error)
	LoadBlockIndex(*Chain, func(*Chain, []byte, []byte, uint32)) (error)

	GetStats() (string)
	Close()

	GetUnspentFromPkScr(scr []byte) (res []OneUnspentTx)
}

var NewDb func() BtcDB
