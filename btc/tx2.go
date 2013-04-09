package btc

import (
    "bytes"
)


func NewTxOut(rd *bytes.Reader) (res *TxOut, e error) {
	var le uint64
	var txout TxOut
	
	txout.Value, e = ReadUint64(rd)
	if e != nil {
		return
	}
	
	le, e = ReadVLen64(rd)
	if e != nil {
		return
	}
	txout.Pk_script = make([]byte, le)
	_, e = rd.Read(txout.Pk_script[:])
	if e != nil {
		return
	}
	
	res = &txout
	return
}


func NewTx(data []byte) (res *Tx, e error) {
	rd := bytes.NewReader(data[:])

	var tx Tx
	var le uint64
	var siz int64
	
	tx.Version, e = ReadUint32(rd)
	if e != nil {
		return
	}

	// TxOut
	le, e = ReadVLen64(rd)
	if e != nil {
		return
	}
	tx.TxIn = make([]*TxIn, le)
	for i := range tx.TxIn {
		tx.TxIn[i], e = NewTxIn(rd)
		if e != nil {
			return
		}
	}
	
	// TxOut
	le, e = ReadVLen64(rd)
	if e != nil {
		return
	}
	tx.TxOut = make([]*TxOut, le)
	for i := range tx.TxOut {
		tx.TxOut[i], e = NewTxOut(rd)
		if e != nil {
			return
		}
	}
	
	tx.Lock_time, e = ReadUint32(rd)

	siz, e = rd.Seek(0, 1)
	if e != nil {
		return
	}

	tx.Raw = data[0:siz]
	tx.Hash = NewSha2Hash(tx.Raw)
	res = &tx

	return
}
