package utxo

import (
	//"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
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


const (
	UtxoIdxLen = 8
)

type UtxoKeyType [UtxoIdxLen]byte

type UtxoRec struct {
	TxID [32]byte
	Coinbase bool
	InBlock uint32
	Outs []*UtxoTxOut
}

type UtxoTxOut struct {
	Value uint64
	PKScr []byte
}


func FullUtxoRec(dat []byte) *UtxoRec {
	var key UtxoKeyType
	copy(key[:], dat[:UtxoIdxLen])
	return NewUtxoRec(key, dat[UtxoIdxLen:])
}


var (
	sta_rec UtxoRec
	rec_outs = make([]*UtxoTxOut, 30001)
	rec_pool = make([]UtxoTxOut, 30001)
)


func NewUtxoRecStatic(key UtxoKeyType, dat []byte) *UtxoRec {
	var off, n, i int
	var u64, idx uint64

	off = 32-UtxoIdxLen
	copy(sta_rec.TxID[:UtxoIdxLen], key[:])
	copy(sta_rec.TxID[UtxoIdxLen:], dat[:off])

	u64, n = btc.VULe(dat[off:])
	off += n
	sta_rec.InBlock = uint32(u64)

	u64, n = btc.VULe(dat[off:])
	off += n

	sta_rec.Coinbase = (u64&1) != 0
	u64 >>= 1
	if len(rec_outs) < int(u64) {
		rec_outs = make([]*UtxoTxOut, u64)
		rec_pool = make([]UtxoTxOut, u64)
	}
	sta_rec.Outs = rec_outs[:u64]
	for i := range sta_rec.Outs {
		sta_rec.Outs[i] = nil
	}

	for off < len(dat) {
		idx, n = btc.VULe(dat[off:])
		off += n

		sta_rec.Outs[idx] = &rec_pool[idx]

		u64, n = btc.VULe(dat[off:])
		off += n
		sta_rec.Outs[idx].Value = uint64(u64)

		i, n = btc.VLen(dat[off:])
		off += n

		sta_rec.Outs[idx].PKScr = dat[off:off+i]
		off += i
	}

	return &sta_rec
}


func NewUtxoRec(key UtxoKeyType, dat []byte) *UtxoRec {
	var off, n, i int
	var u64, idx uint64
	var rec UtxoRec

	off = 32-UtxoIdxLen
	copy(rec.TxID[:UtxoIdxLen], key[:])
	copy(rec.TxID[UtxoIdxLen:], dat[:off])

	u64, n = btc.VULe(dat[off:])
	off += n
	rec.InBlock = uint32(u64)

	u64, n = btc.VULe(dat[off:])
	off += n

	rec.Coinbase = (u64&1) != 0
	rec.Outs = make([]*UtxoTxOut, u64>>1)

	for off < len(dat) {
		idx, n = btc.VULe(dat[off:])
		off += n
		rec.Outs[idx] = new(UtxoTxOut)

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


func (rec *UtxoRec) Serialize(full bool) (buf []byte) {
	var le, of int
	var any_out bool

	outcnt := uint64(len(rec.Outs)<<1)
	if rec.Coinbase {
		outcnt |= 1
	}

	if full {
		le = 32
	} else {
		le = 32-UtxoIdxLen
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
		of = 32-UtxoIdxLen
		copy(buf[:of], rec.TxID[UtxoIdxLen:])
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


func (rec *UtxoRec) Bytes() []byte {
	return rec.Serialize(false)
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

func (out *UtxoTxOut) IsP2KH() bool {
	return len(out.PKScr)==25 && out.PKScr[0]==0x76 && out.PKScr[1]==0xa9 &&
		out.PKScr[2]==0x14 && out.PKScr[23]==0x88 && out.PKScr[24]==0xac
}

func (r *UtxoTxOut) IsP2SH() bool {
	return len(r.PKScr)==23 && r.PKScr[0]==0xa9 && r.PKScr[1]==0x14 && r.PKScr[22]==0x87
}

func (r *UtxoTxOut) IsP2WPKH() bool {
	return len(r.PKScr)==22 && r.PKScr[0]==0 && r.PKScr[1]==20
}

func (r *UtxoTxOut) IsP2WSH() bool {
	return len(r.PKScr)==34 && r.PKScr[0]==0 && r.PKScr[1]==32
}
