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


func FullQdbRec(dat []byte) *QdbRec {
	return NewQdbRec(qdb.KeyType(binary.LittleEndian.Uint64(dat[:8])), dat[8:])
}


var (
	sta_rec QdbRec
	rec_outs = make([]*QdbTxOut, 3075)
	rec_pool = make([]QdbTxOut, 3075)
)


func NewQdbRecStatic(key qdb.KeyType, dat []byte) *QdbRec {
	var off, n, i int
	var u64, idx, exp_idx uint64

	binary.LittleEndian.PutUint64(sta_rec.TxID[:8], uint64(key))
	copy(sta_rec.TxID[8:], dat[:24])
	off = 24

	u64, n = btc.VULe(dat[off:])
	off += n
	sta_rec.InBlock = uint32(u64)

	u64, n = btc.VULe(dat[off:])
	off += n

	sta_rec.Coinbase = (u64&1) != 0
	u64 >>= 1
	if len(rec_outs) < int(u64) {
		rec_outs = make([]*QdbTxOut, u64)
		rec_pool = make([]QdbTxOut, u64)
	}
	sta_rec.Outs = rec_outs[:u64]

	for off < len(dat) {
		idx, n = btc.VULe(dat[off:])
		off += n

		for exp_idx < idx {
			sta_rec.Outs[exp_idx] = nil
			exp_idx++
		}
		sta_rec.Outs[idx] = &rec_pool[idx]

		u64, n = btc.VULe(dat[off:])
		off += n
		sta_rec.Outs[idx].Value = uint64(u64)

		i, n = btc.VLen(dat[off:])
		off += n

		sta_rec.Outs[idx].PKScr = dat[off:off+i]
		off += i

		exp_idx = idx+1
	}

	return &sta_rec
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


func (rec *QdbRec) Serialize(full bool) []byte {
	var any_out bool
	bu := new(bytes.Buffer)
	if full {
		bu.Write(rec.TxID[:])
	} else {
		bu.Write(rec.TxID[8:])
	}
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
			any_out = true
		}
	}
	if any_out {
		return bu.Bytes()
	}
	return nil
}


func (rec *QdbRec) Bytes() []byte {
	return rec.Serialize(false)
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
