package chain

import (
	"fmt"
	"errors"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/qdb"
)



type unspentDb struct {
	dir string
	tdb [NumberOfUnspentSubDBs] *qdb.DB
	defragIndex int
	defragCount uint64
	nosyncinprogress bool
	ch *Chain
}


func newUnspentDB(dir string, ch *Chain) (db *unspentDb) {
	db = new(unspentDb)
	db.dir = dir
	db.ch = ch

	for i := range db.tdb {
		fmt.Print("\rLoading new unspent DB - ", 100*i/len(db.tdb), "% complete ... ")
		db.dbN(i) // Load each of the sub-DBs into memory
		if AbortNow {
			return
		}
	}
	fmt.Print("\r                                                              \r")

	return
}


func (db *unspentDb) dbN(i int) (*qdb.DB) {
	if db.tdb[i]==nil {
		qdb.NewDBrowse(&db.tdb[i], db.dir+fmt.Sprintf("%06d", i), func(k qdb.KeyType, v []byte) uint32 {
			if db.ch.CB.LoadWalk!=nil {
				db.ch.CB.LoadWalk(NewQdbRec(k, v))
			}
			return 0
		}, SingeIndexSize)

		if db.nosyncinprogress {
			db.tdb[i].NoSync()
		}
	}
	return db.tdb[i]
}


func (db *unspentDb) get(po *btc.TxPrevOut) (res *btc.TxOut, e error) {
	ind := qdb.KeyType(binary.LittleEndian.Uint64(po.Hash[:8]))
	v := db.dbN(int(po.Hash[31])%NumberOfUnspentSubDBs).Get(ind)
	if v==nil {
		e = errors.New("Unspent TX not found")
		return
	}

	rec := NewQdbRec(ind, v)
	if len(rec.Outs)<int(po.Vout) || rec.Outs[po.Vout]==nil {
		e = errors.New("Unspent VOut not found")
		return
	}
	res = new(btc.TxOut)
	res.VoutCount = uint32(len(rec.Outs))
	res.WasCoinbase = rec.Coinbase
	res.BlockHeight = rec.InBlock
	res.Value = rec.Outs[po.Vout].Value
	res.Pk_script = rec.Outs[po.Vout].PKScr
	return
}


func (db *unspentDb) del(hash []byte, outs []bool) {
	ind := qdb.KeyType(binary.LittleEndian.Uint64(hash[:8]))
	_db := db.dbN(int(hash[31])%NumberOfUnspentSubDBs)
	v := _db.Get(ind)
	if v==nil {
		panic("Cannot delete this")
	}
	rec := NewQdbRec(ind, v)
	var anyout bool
	for i, rm := range outs {
		if rm {
			rec.Outs[i] = nil
		} else if rec.Outs[i] != nil {
			anyout = true
		}
	}
	if anyout {
		_db.Put(ind, rec.Bytes())
	} else {
		_db.Del(ind)
	}
}


func (db *unspentDb) commit(changes *BlockChanges) {
	// Now aplly the unspent changes
	for _, rec := range changes.AddList {
		ind := qdb.KeyType(binary.LittleEndian.Uint64(rec.TxID[:8]))
		db.dbN(int(rec.TxID[31])%NumberOfUnspentSubDBs).PutExt(ind, rec.Bytes(), 0)
		if db.ch.CB.NotifyTxAdd!=nil {
			db.ch.CB.NotifyTxAdd(rec)
		}
	}
	for k, v := range changes.DeledTxs {
		db.del(k[:], v)
		if db.ch.CB.NotifyTxDel!=nil {
			db.ch.CB.NotifyTxDel(k[:], v)
		}
	}
}


func (db *unspentDb) sync() {
	db.nosyncinprogress = false
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].Sync()
		}
	}
}

func (db *unspentDb) nosync() {
	db.nosyncinprogress = true
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].NoSync()
		}
	}
}

func (db *unspentDb) save() {
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].Flush()
		}
	}
}

func (db *unspentDb) close() {
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].Close()
			db.tdb[i] = nil
		}
	}
}

func (db *unspentDb) idle() bool {
	for _ = range db.tdb {
		db.defragIndex++
		if db.defragIndex >= len(db.tdb) {
			db.defragIndex = 0
		}
		if db.tdb[db.defragIndex]!=nil && db.tdb[db.defragIndex].Defrag() {
			db.defragCount++
			return true
		}
	}
	return false
}


func (db *unspentDb) browse(walk FunctionWalkUnspent, quick bool) {
	var i int
	brfn := func(k qdb.KeyType, v []byte) (fl uint32) {
		walk(NewQdbRec(k, v))
		return
	}

	if quick {
		for i = range db.tdb {
			db.dbN(i).Browse(brfn)
		}
	} else {
		for i = range db.tdb {
			db.dbN(i).BrowseAll(brfn)
		}
	}
}


func (db *unspentDb) stats() (s string) {
	var tot, brcnt, sum, sumcb uint64
	var mincnt, maxcnt, totdatasize uint64
	for i := range db.tdb {
		dbcnt := uint64(db.dbN(i).Count())
		if i==0 {
			mincnt, maxcnt = dbcnt, dbcnt
		} else if dbcnt < mincnt {
			mincnt = dbcnt
		} else if dbcnt > maxcnt {
			maxcnt = dbcnt
		}
		tot += dbcnt
		db.dbN(i).Browse(func(k qdb.KeyType, v []byte) uint32 {
			totdatasize += uint64(len(v))
			brcnt++
			rec := NewQdbRec(k, v)
			for idx := range rec.Outs {
				if rec.Outs[idx]!=nil {
					sum += rec.Outs[idx].Value
					if rec.Coinbase {
						sumcb += rec.Outs[idx].Value
					}
				}
			}
			return 0
		})
	}
	s = fmt.Sprintf("UNSPENT: %.8f BTC in %d/%d outputs. %.8f BTC in coinbase.\n",
		float64(sum)/1e8, brcnt, tot, float64(sumcb)/1e8)
	s += fmt.Sprintf(" Defrags:%d  Recs/db : %d..%d   (config:%d)   TotalData:%.1fMB\n",
		db.defragCount, mincnt, maxcnt, SingeIndexSize, float64(totdatasize)/1e6)
	return
}


func (db *UnspentDB) PrintCoinAge() {
	const chunk = 10000
	var maxbl uint32
	type onerec struct {
		cnt, bts, val, valcb uint64
	}
	age := make(map[uint32] *onerec)
	for i := range db.unspent.tdb {
		db.unspent.dbN(i).BrowseAll(func(k qdb.KeyType, v []byte) uint32 {
			rec := NewQdbRec(k, v)
			a := rec.InBlock
			if a>maxbl {
				maxbl = a
			}
			a /= chunk
			tmp := age[a]
			if tmp==nil {
				tmp = new(onerec)
			}
			for _, ou := range rec.Outs {
				if ou!=nil {
					tmp.val += ou.Value
					if rec.Coinbase {
						tmp.valcb += ou.Value
					}
					tmp.cnt++
				}
			}
			tmp.bts += uint64(len(v))
			age[a] = tmp
			return 0
		})
	}
	for i:=uint32(0); i<=(maxbl/chunk); i++ {
		tb := (i+1)*chunk-1
		if tb>maxbl {
			tb = maxbl
		}
		cnt := uint64(tb-i*chunk)+1
		fmt.Printf(" Blocks  %6d ... %6d: %9d records, %5d MB, %18s/%16s BTC.  Per block:%7.1f records,%8d,%15s BTC\n",
			i*chunk, tb, age[i].cnt, age[i].bts>>20, btc.UintToBtc(age[i].val), btc.UintToBtc(age[i].valcb),
			float64(age[i].cnt)/float64(cnt), (age[i].bts/cnt), btc.UintToBtc(age[i].val/cnt))
	}
}
