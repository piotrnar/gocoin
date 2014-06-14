package chain

import (
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

func vlen2size(uvl uint64) int {
	if uvl<0xfd {
		return 1
	} else if uvl<0x10000 {
		return 3
	} else if uvl<0x100000000 {
		return 5
	}
	return 9
}


func (rec *QdbRec) Serialize(full bool) (buf []byte) {
	var le, of int
	var any_out bool

	outcnt := uint64(len(rec.Outs)<<1)
	if rec.Coinbase {
		outcnt |= 1
	}

	if full {
		le = 32
	} else {
		le = 24
	}

	le += vlen2size(uint64(rec.InBlock))  // block length
	le += vlen2size(outcnt)  // out count

	for i := range rec.Outs {
		if rec.Outs[i] != nil {
			le += vlen2size(uint64(i))
			le += vlen2size(rec.Outs[i].Value)
			le += vlen2size(uint64(len(rec.Outs[i].PKScr)))
			le += len(rec.Outs[i].PKScr)
			any_out = true
		}
	}
	if !any_out {
		return
	}

	buf = make([]byte, le)
	if full {
		copy(buf[:32], rec.TxID[:])
		of = 32
	} else {
		copy(buf[:24], rec.TxID[8:])
		of = 24
	}

	of += btc.PutULe(buf[of:], uint64(rec.InBlock))
	of += btc.PutULe(buf[of:], outcnt)
	for i := range rec.Outs {
		if rec.Outs[i] != nil {
			of += btc.PutULe(buf[of:], uint64(i))
			of += btc.PutULe(buf[of:], rec.Outs[i].Value)
			of += btc.PutULe(buf[of:], uint64(len(rec.Outs[i].PKScr)))
			copy(buf[of:], rec.Outs[i].PKScr)
			of += len(rec.Outs[i].PKScr)
		}
	}
	return
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
