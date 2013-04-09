package btc

import (
    "bytes"
)


func NewTxIn(rd *bytes.Reader) (res *TxIn, e error) {
	var txin TxIn
	var le uint64
	
	_, e = rd.Read(txin.Input.Hash[:])
	if e != nil {
		return
	}
	
	txin.Input.Vout, e = ReadUint32(rd)
	if e != nil {
		return
	}
	
	le, e = ReadVLen64(rd)
	if e != nil {
		return
	}
	txin.ScriptSig = make([]byte, le)
	_, e = rd.Read(txin.ScriptSig[:])
	if e != nil {
		return
	}
	
	// Sequence
	txin.Sequence, e = ReadUint32(rd)
	if e==nil {
		res = &txin
	}

	return 
}

func (txin *TxIn) GetKeyAndSig() (sig *Signature, key *PublicKey, e error) {
	sig, e = NewSignature(txin.ScriptSig[1:1+txin.ScriptSig[0]])
	if e != nil {
		return
	}
	offs := 1+txin.ScriptSig[0]
	key, e = NewPublicKey(txin.ScriptSig[1+offs:1+offs+txin.ScriptSig[offs]])
	return
}
