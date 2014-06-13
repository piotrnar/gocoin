package chain

import (
	"bytes"
	//"encoding/hex"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/qdb"
)


/*
Each unspent key is 8 bytes long - thats firt 8 bytes of TXID
Eech value is variable length:
  [0:24] - remainig 24 bytes of TxID
  var_int: BlochHeight
  var_int: 2*out_cnt + is_coibase
  And now set of records:
   var_int: Output index
   var_int: Value
   var_int: PKscrpt_length
   PKscript
  ...
*/


type QdbRec struct {
	TxID [32]byte
	Coinbase bool
	InBlock uint32
	Outs []*QdbTxOut
}

type QdbTxOut struct {
	Value uint64
	PKScr []byte
}


func NewQdbRec(key qdb.KeyType, dat []byte) *QdbRec {
	var off, n, i int
	var u64, idx uint64
	var rec QdbRec

	binary.LittleEndian.PutUint64(rec.TxID[:8], uint64(key))
	copy(rec.TxID[8:], dat[:24])
	off = 24

	u64, n = btc.VULe(dat[off:])
	off += n
	rec.InBlock = uint32(u64)

	u64, n = btc.VULe(dat[off:])
	off += n

	rec.Coinbase = (u64&1) != 0
	rec.Outs = make([]*QdbTxOut, u64>>1)

	for off < len(dat) {
		idx, n = btc.VULe(dat[off:])
		off += n
		rec.Outs[idx] = new(QdbTxOut)

		u64, n = btc.VULe(dat[off:])
		off += n
		rec.Outs[idx].Value = uint64(u64)

		i, n = btc.VLen(dat[off:])
		off += n

		rec.Outs[idx].PKScr = dat[off:off+i]
		off += i
	}
	return &rec
}


func (rec *QdbRec) Bytes() []byte {
	bu := new(bytes.Buffer)
	bu.Write(rec.TxID[8:])
	btc.WriteVlen(bu, uint64(rec.InBlock))
	if rec.Coinbase {
		btc.WriteVlen(bu, uint64(len(rec.Outs)<<1)|1)
	} else {
		btc.WriteVlen(bu, uint64(len(rec.Outs)<<1))
	}
	for i := range rec.Outs {
		if rec.Outs[i] != nil {
			btc.WriteVlen(bu, uint64(i))
			btc.WriteVlen(bu, rec.Outs[i].Value)
			btc.WriteVlen(bu, uint64(len(rec.Outs[i].PKScr)))
			bu.Write(rec.Outs[i].PKScr)
		}
	}
	//println("enc:", hex.EncodeToString(bu.Bytes()))
	return bu.Bytes()
}

func (r *QdbRec) ToUnspent(idx uint32, ad *btc.BtcAddr) (nr *OneUnspentTx) {
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

func (out *QdbTxOut) IsP2KH() bool {
	return len(out.PKScr)==25 && out.PKScr[0]==0x76 && out.PKScr[1]==0xa9 &&
		out.PKScr[2]==0x14 && out.PKScr[23]==0x88 && out.PKScr[24]==0xac
}

func (r *QdbTxOut) IsP2SH() bool {
	return len(r.PKScr)==23 && r.PKScr[0]==0xa9 && r.PKScr[1]==0x14 && r.PKScr[22]==0x87
}

func (r *QdbTxOut) IsStealthIdx() bool {
	return len(r.PKScr)==40 && r.PKScr[0]==0x6a && r.PKScr[49]==0x26 && r.PKScr[50]==0x06
}
