package utxo

import (
	"github.com/piotrnar/gocoin/lib/btc"
)

/*
Each unspent key is 8 bytes long - these are the first 8 bytes of TXID
Eech value is variable length:
  [0:24] - remainig 24 bytes of TxID
  var_int: BlochHeight
  var_int: 2*out_cnt + is_coinbase
  And now set of records:
   var_int: Output index
   var_int: Value
   var_int: PKscrpt_length
   PKscript
  ...
*/

type UtxoRec struct {
	TxID     [32]byte
	Coinbase bool
	InBlock  uint32
	Outs     []*UtxoTxOut
}

type UtxoTxOut struct {
	Value uint64
	PKScr []byte
}

var (
	sta_rec  UtxoRec
	rec_outs = make([]*UtxoTxOut, 30001)
	rec_pool = make([]UtxoTxOut, 30001)
)

var (
	FullUtxoRec func(dat []byte) *UtxoRec = FullUtxoRecU
	NewUtxoRecStatic func(key UtxoKeyType, dat []byte) *UtxoRec = NewUtxoRecStaticU
	NewUtxoRec func(key UtxoKeyType, dat []byte) *UtxoRec = NewUtxoRecU
	OneUtxoRec func(key UtxoKeyType, dat []byte, vout uint32) *btc.TxOut = OneUtxoRecU
	Serialize func(rec *UtxoRec, full bool, use_buf []byte) (buf []byte) = SerializeU
)


func (r *UtxoRec) ToUnspent(idx uint32, ad *btc.BtcAddr) (nr *OneUnspentTx) {
	nr = new(OneUnspentTx)
	nr.TxPrevOut.Hash = r.TxID
	nr.TxPrevOut.Vout = idx
	nr.Value = r.Outs[idx].Value
	nr.Coinbase = r.Coinbase
	nr.MinedAt = r.InBlock
	nr.BtcAddr = ad
	nr.destString = ad.String()
	return
}
