package utxo

import (
	"sync"

	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/script"
)

/*
	These are functions for dealing with compressed UTXO records
*/

var (
	comp_pool_mutex sync.Mutex //<- consider using this mutex for multi-thread serializations
	comp_val        []uint64
	comp_scr        [][]byte
	ComprScrLen     = []int{21, 21, 33, 33, 33, 33}
)

func NewUtxoRecOwnC(key UtxoKeyType, dat []byte, rec *UtxoRec, cbs *NewUtxoOutAllocCbs) {
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
		rec.Outs[idx].Value = btc.DecompressAmount(uint64(u64))

		i, n = btc.VLen(dat[off:])
		if i < 6 {
			i = ComprScrLen[i]
			rec.Outs[idx].PKScr = script.DecompressScript(dat[off : off+i])
		} else {
			off += n
			i -= 6
			rec.Outs[idx].PKScr = dat[off : off+i]
		}
		off += i
	}
}

func OneUtxoRecC(key UtxoKeyType, dat []byte, vout uint32) *btc.TxOut {
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

		if uint32(idx) == vout {
			res.Value = btc.DecompressAmount(uint64(u64))
			if i < 6 {
				i = ComprScrLen[i]
				res.Pk_script = script.DecompressScript(dat[off : off+i])
			} else {
				off += n
				i -= 6
				res.Pk_script = dat[off : off+i]
			}
			return &res
		}

		if i < 6 {
			i = ComprScrLen[i]
		} else {
			off += n
			i -= 6
		}
		off += i
	}
	return nil
}

func SerializeC(rec *UtxoRec, full bool, use_buf []byte) (buf []byte) {
	var le, of int
	var any_out bool

	outcnt := uint64(len(rec.Outs) << 1)
	if rec.Coinbase {
		outcnt |= 1
	}

	// <- consider anabling this for multi-thread serializations
	comp_pool_mutex.Lock()
	defer comp_pool_mutex.Unlock()

	// Only allocate when used for a first time, so no mem is wasted when not using compression
	if int(outcnt) > len(comp_val) {
		if outcnt > 30001 {
			comp_val = make([]uint64, outcnt)
			comp_scr = make([][]byte, outcnt)
		} else {
			comp_val = make([]uint64, 30001)
			comp_scr = make([][]byte, 30001)
		}
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
			comp_val[i] = btc.CompressAmount(r.Value)
			comp_scr[i] = script.CompressScript(r.PKScr)
			le += btc.VLenSize(comp_val[i])
			if comp_scr[i] != nil {
				le += len(comp_scr[i])
			} else {
				le += btc.VLenSize(uint64(6 + len(r.PKScr)))
				le += len(r.PKScr)
			}
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
			of += btc.PutULe(buf[of:], comp_val[i])
			if comp_scr[i] != nil {
				copy(buf[of:], comp_scr[i])
				of += len(comp_scr[i])
			} else {
				of += btc.PutULe(buf[of:], uint64(6+len(r.PKScr)))
				copy(buf[of:], r.PKScr)
				of += len(r.PKScr)
			}
		}
	}
	return
}
