package btc

type BtcDB interface {
	StartTransaction()
	CommitTransaction()
	RollbackTransaction()
	
	UnspentPurge()
	UnspentAdd(idx *TxPrevOut, rec *TxOut) (error)
	UnspentGet(out *TxPrevOut) (*TxOut, error)
	UnspentDel(out *TxPrevOut) (error)

	UnwindDel(height uint32) (error)
	UnwindAdd(height uint32, added int, po *TxPrevOut, rec *TxOut) (error)
	UnwindNow(height uint32) (error)
	GetStats() (string)

	BlockAdd(height uint32, bl *Block) (error)
	BlockGet(hash *Uint256) ([]byte, error)

	LoadBlockIndex(*Chain, func(*Chain, []byte, []byte, uint32)) (error)

	Close()

	GetUnspentFromPkScr(scr []byte) (res []OneUnspentTx)
	ListUnspent()
}

var NewDb func() BtcDB
