package btc

type BtcDB interface {
	UnspentPurge()
	UnspentAdd(idx *TxPrevOut, rec *TxOut) (error)
	UnspentGet(out *TxPrevOut) (*TxOut, error)
	UnspentDel(out *TxPrevOut) (error)

	UnwindDel(height uint32) (error)
	UnwindNewRecord(height uint32, added bool, po *TxPrevOut, rec *TxOut) (error)
	UnwindBlock(height uint32) (error)
	GetStats() (string)

	BlockAdd(height uint32, bl *Block) (error)
	BlockGet(hash *Uint256) ([]byte, error)

	LoadBlockIndex(*Chain, func(*Chain, []byte, []byte, uint32)) (error)

	Close()

	GetUnspentFromPkScr(scr []byte) (res []OneUnspentTx)
}

var NewDb func() BtcDB
