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

type NewUtxoOutAllocCbs struct {
	OutsList func(cnt int) []*UtxoTxOut
	OneOut   func() *UtxoTxOut
}

/*
So far the highest number of tx outpts on BTC chain is 13107, tx
dd9f6bbf80ab36b722ca95d93268667a3ea6938288e0d4cf0e7d2e28a7a91ab3
from block number 391204.

For testnet4 it is 32223, from block number 69603, in tx
fefdc13fdb2e5e53d31549491cf133bee359c60b8d3d7fad97c971834b1ee6cc
but we will use the mainnet's constant and adjust for testnet in
runtime.
*/
const MAX_OUTS_SEEN = 13107

var (
	sta_rec  UtxoRec
	rec_outs = make([]*UtxoTxOut, MAX_OUTS_SEEN)
	rec_pool = make([]UtxoTxOut, MAX_OUTS_SEEN)
	rec_idx  int
	sta_cbs  = NewUtxoOutAllocCbs{
		OutsList: func(cnt int) (res []*UtxoTxOut) {
			if len(rec_outs) < cnt {
				println("utxo.MAX_OUTS_SEEN", len(rec_outs), "->", cnt)
				rec_outs = make([]*UtxoTxOut, cnt)
				rec_pool = make([]UtxoTxOut, cnt)
			}
			rec_idx = 0
			res = rec_outs[:cnt]
			for i := range res {
				res[i] = nil
			}
			return
		},
		OneOut: func() (res *UtxoTxOut) {
			res = &rec_pool[rec_idx]
			rec_idx++
			return
		},
	}
)

var (
	NewUtxoRecOwn func(UtxoKeyType, []byte, *UtxoRec, *NewUtxoOutAllocCbs)   = NewUtxoRecOwnU
	OneUtxoRec    func(key UtxoKeyType, dat []byte, vout uint32) *btc.TxOut  = OneUtxoRecU
	Serialize     func(rec *UtxoRec, full bool, use_buf []byte) (buf []byte) = SerializeU
)

func NewUtxoRecStatic(key UtxoKeyType, dat []byte) *UtxoRec {
	NewUtxoRecOwn(key, dat, &sta_rec, &sta_cbs)
	return &sta_rec
}

func NewUtxoRec(key UtxoKeyType, dat []byte) *UtxoRec {
	var rec UtxoRec
	NewUtxoRecOwn(key, dat, &rec, nil)
	return &rec
}

func FullUtxoRec(dat []byte) *UtxoRec {
	var key UtxoKeyType
	var rec UtxoRec
	copy(key[:], dat[:UtxoIdxLen])
	NewUtxoRecOwn(key, dat[UtxoIdxLen:], &rec, nil)
	return &rec
}

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
