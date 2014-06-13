package chain

import (
	"os"
	"fmt"
	"bytes"
	"errors"
	"strconv"
	"io/ioutil"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/qdb"
)


const (
	NumberOfUnspentSubDBs = 0x10
	UnwindBufferMaxHistory = 256  // Let's keep unwind history for so may last blocks
)

type FunctionWalkUnspent func(*QdbRec)

// Used to pass block's changes to UnspentDB
type BlockChanges struct {
	Height uint32
	LastKnownHeight uint32  // put here zero to disable this feature
	AddList []*QdbRec
	DeledTxs map[[32]byte] []bool
	UndoData *bytes.Buffer
}


type UnspentDB struct {
	LastBlockHash []byte
	LastBlockHeight uint32
	dir string
	tdb [NumberOfUnspentSubDBs] *qdb.DB
	defragIndex int
	defragCount uint64
	nosyncinprogress bool
	ch *Chain
}


func NewUnspentDb(dir string, init bool, ch *Chain) (db *UnspentDB) {
	db = new(UnspentDB)
	db.dir = dir+"unspent4"+string(os.PathSeparator)

	if init {
		os.RemoveAll(db.dir)
	} else {
		fis, _ := ioutil.ReadDir(db.dir)
		var maxbl int
		for _, fi := range fis {
			if !fi.IsDir() && fi.Size()>=32 {
				cb, er := strconv.ParseUint(fi.Name(), 10, 32)
				if er == nil && int(cb) > maxbl {
					maxbl = int(cb)
				}
			}
		}
		if maxbl!=0 {
			f, _ := os.Open(fmt.Sprint(db.dir, maxbl))
			db.LastBlockHash = make([]byte, 32)
			f.Read(db.LastBlockHash)
			f.Close()
		}
	}

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


// Commit the given add/del transactions to UTXO and Wnwind DBs
func (db *UnspentDB) CommitBlockTxs(changes *BlockChanges, blhash []byte) (e error) {
	db.nosync()
	db.commit(changes)
	if changes.Height >= changes.LastKnownHeight {
		db.Sync()
	}
	if db.LastBlockHash==nil {
		db.LastBlockHash = make([]byte, 32)
	}
	copy(db.LastBlockHash, blhash)
	db.LastBlockHeight = changes.Height

	if changes.UndoData!=nil || (changes.Height%UnwindBufferMaxHistory)==0 {
		f, _ := os.Create(fmt.Sprint(db.dir, changes.Height))
		f.Write(blhash)
		if changes.UndoData != nil {
			f.Write(changes.UndoData.Bytes())
		}
		f.Close()
	}

	if changes.Height>UnwindBufferMaxHistory {
		os.Remove(fmt.Sprint(db.dir, changes.Height-UnwindBufferMaxHistory))
	}
	return
}


// Return DB statistics
func (db *UnspentDB) GetStats() (s string) {
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
	s += fmt.Sprintf(" Defrags:%d  Recs/db : %d..%d   TotalData:%.1fMB\n",
		db.defragCount, mincnt, maxcnt, float64(totdatasize)/1e6)
	return
}


// Flush all the data to files
func (db *UnspentDB) Sync() {
	db.nosyncinprogress = false
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].Sync()
		}
	}
	if db.LastBlockHash!=nil {
		fn := fmt.Sprint(db.dir, db.LastBlockHeight)
		fi, er := os.Stat(fn)
		if er!=nil || fi.Size()<32 {
			fmt.Println("Saving last block's hash")
			ioutil.WriteFile(fn, db.LastBlockHash, 0666)
		}
	}
}


// Hold on writing data to disk untill next sync is called
func (db *UnspentDB) nosync() {
	db.nosyncinprogress = true
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].NoSync()
		}
	}
}


// Flush the data and close all the files
func (db *UnspentDB) Close() {
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].Close()
			db.tdb[i] = nil
		}
	}
}


// Call it when the main thread is idle - this will do DB defrag
func (db *UnspentDB) Idle() bool {
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


// Flush all the data to disk
func (db *UnspentDB) Save() {
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].Flush()
		}
	}
}


// Get ne unspent output
func (db *UnspentDB) UnspentGet(po *btc.TxPrevOut) (res *btc.TxOut, e error) {
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


// Browse through all unspent outputs
func (db *UnspentDB) BrowseUTXO(quick bool, walk FunctionWalkUnspent) {
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

func (db *UnspentDB) dbN(i int) (*qdb.DB) {
	if db.tdb[i]==nil {
		qdb.NewDBrowse(&db.tdb[i], db.dir+fmt.Sprintf("%06d", i), func(k qdb.KeyType, v []byte) uint32 {
			if db.ch.CB.LoadWalk!=nil {
				db.ch.CB.LoadWalk(NewQdbRec(k, v))
			}
			return 0
		}, 20000/*size of pre-allocated map*/)

		if db.nosyncinprogress {
			db.tdb[i].NoSync()
		}
	}
	return db.tdb[i]
}


func (db *UnspentDB) del(hash []byte, outs []bool) {
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


func (db *UnspentDB) commit(changes *BlockChanges) {
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


func (db *UnspentDB) PrintCoinAge() {
	const chunk = 10000
	var maxbl uint32
	type onerec struct {
		cnt, bts, val, valcb uint64
	}
	age := make(map[uint32] *onerec)
	for i := range db.tdb {
		db.dbN(i).BrowseAll(func(k qdb.KeyType, v []byte) uint32 {
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
