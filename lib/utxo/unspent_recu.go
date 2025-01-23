package utxo

import (
	"github.com/piotrnar/gocoin/lib/btc"
)

/*
These are functions for dealing with uncompressed UTXO records
*/

func FullUtxoRecU(dat []byte) *UtxoRec {
	var key UtxoKeyType
	copy(key[:], dat[:UtxoIdxLen])
	return NewUtxoRec(key, dat[UtxoIdxLen:])
}

func NewUtxoRecOwnU(key UtxoKeyType, dat []byte, rec *UtxoRec, cbs *NewUtxoOutAllocCbs) {
	var off, n, i int
	var u64, idx uint64

	off = 32 - UtxoIdxLen
	copy(rec.TxID[:UtxoIdxLen], key[:])
	copy(rec.TxID[UtxoIdxLen:], dat[:off])

	u64, n = btc.VULe(dat[off:])
	off += n
	rec.InBlock = uint32(u64)

	u64, n = btc.VULe(dat[off:])
	off += n

	rec.Coinbase = (u64 & 1) != 0
	if cbs != nil {
		rec.Outs = cbs.OutsList(int(u64 >> 1))
	} else {
		rec.Outs = make([]*UtxoTxOut, u64>>1)
	}

	for off < len(dat) {
		idx, n = btc.VULe(dat[off:])
		off += n
		if cbs != nil {
			rec.Outs[idx] = cbs.OneOut()
		} else {
			rec.Outs[idx] = new(UtxoTxOut)
		}

		u64, n = btc.VULe(dat[off:])
		off += n
		rec.Outs[idx].Value = uint64(u64)

		i, n = btc.VLen(dat[off:])
		off += n

		rec.Outs[idx].PKScr = dat[off : off+i]
		off += i
	}
}

func NewUtxoRecStaticU(key UtxoKeyType, dat []byte) *UtxoRec {
	NewUtxoRecOwnU(key, dat, &sta_rec, &sta_cbs)
	return &sta_rec
}

func NewUtxoRecU(key UtxoKeyType, dat []byte) *UtxoRec {
	var rec UtxoRec
	NewUtxoRecOwnU(key, dat, &rec, nil)
	return &rec
}

func OneUtxoRecU(key UtxoKeyType, dat []byte, vout uint32) *btc.TxOut {
	var off, n, i int
	var u64, idx uint64
	var res btc.TxOut

	off = 32 - UtxoIdxLen

	u64, n = btc.VULe(dat[off:])
	off += n
	res.BlockHeight = uint32(u64)

	u64, n = btc.VULe(dat[off:])
	off += n

	res.VoutCount = uint32(u64 >> 1)
	if res.VoutCount <= vout {
		return nil
	}
	res.WasCoinbase = (u64 & 1) != 0

	for off < len(dat) {
		idx, n = btc.VULe(dat[off:])
		if uint32(idx) > vout {
			return nil
		}
		off += n

		u64, n = btc.VULe(dat[off:])
		off += n

		i, n = btc.VLen(dat[off:])
		off += n

		if uint32(idx) == vout {
			res.Value = uint64(u64)
			res.Pk_script = dat[off : off+i]
			return &res
		}
		off += i
	}
	return nil
}

// Serialize() returns UTXO-heap pointer to the freshly allocated serialized record.
//
//	rec - UTXO record to serialize
//	full - to have entire 256 bits of TxID at the beginning of the record.
//	use_buf - the data will be serialized into this memory. if nil, it will be allocated by Memory_Malloc().
func SerializeU(rec *UtxoRec, full bool, use_buf []byte) (buf []byte) {
	var le, of int
	var any_out bool

	outcnt := uint64(len(rec.Outs) << 1)
	if rec.Coinbase {
		outcnt |= 1
	}

	if full {
		le = 32
	} else {
		le = 32 - UtxoIdxLen
	}

	le += btc.VLenSize(uint64(rec.InBlock)) // block length
	le += btc.VLenSize(outcnt)              // out count

	for i, r := range rec.Outs {
		if rec.Outs[i] != nil {
			le += btc.VLenSize(uint64(i))
			le += btc.VLenSize(r.Value)
			le += btc.VLenSize(uint64(len(r.PKScr)))
			le += len(r.PKScr)
			any_out = true
		}
	}
	if !any_out {
		return
	}

	if use_buf == nil {
		buf = Memory_Malloc(le)
	} else {
		buf = use_buf[:le]
	}
	if full {
		copy(buf[:32], rec.TxID[:])
		of = 32
	} else {
		of = 32 - UtxoIdxLen
		copy(buf[:of], rec.TxID[UtxoIdxLen:])
	}

	of += btc.PutULe(buf[of:], uint64(rec.InBlock))
	of += btc.PutULe(buf[of:], outcnt)
	for i, r := range rec.Outs {
		if rec.Outs[i] != nil {
			of += btc.PutULe(buf[of:], uint64(i))
			of += btc.PutULe(buf[of:], r.Value)
			of += btc.PutULe(buf[of:], uint64(len(r.PKScr)))
			copy(buf[of:], r.PKScr)
			of += len(r.PKScr)
		}
	}
	return
}
