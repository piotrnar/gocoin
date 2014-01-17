package qdb

import (
	"io"
	"fmt"
	"bytes"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/qdb"
)

const (
	UnwindBufferMaxHistory = 5000  // Let's keep unwind history for so may last blocks
	NumberOfUnwindSubDBs = 10
)

type unwindDb struct {
	dir string
	tdb [NumberOfUnwindSubDBs] *qdb.DB
	lastBlockHeight uint32
	lastBlockHash [32]byte
	defragIndex int
	defragCount uint64
	nosyncinprogress bool
}


func (db *unwindDb) dbH(i int) (*qdb.DB) {
	i &= 0xff
	if db.tdb[i]==nil {
		db.tdb[i], _ = qdb.NewDB(db.dir+fmt.Sprintf("unw%03d", i), true)
		if db.nosyncinprogress {
			db.tdb[i].NoSync()
		}
	}
	return db.tdb[i]
}


func newUnwindDB(dir string) (db *unwindDb) {
	db = new(unwindDb)
	db.dir = dir
	for i := range db.tdb {
		// Load each of the sub-DBs into memory and try to find the highest block
		db.dbH(i).Browse(func(k qdb.KeyType, v []byte) uint32 {
			h := uint32(k)
			if h > db.lastBlockHeight {
				db.lastBlockHeight = h
				copy(db.lastBlockHash[:], v[:32])
			}
			return qdb.NO_CACHE
		})
		if btc.AbortNow {
			return
		}
	}
	return
}


func unwindFromReader(f io.Reader, unsp *unspentDb) {
	for {
		po, to := readSpent(f)
		if po == nil {
			break
		}
		if to != nil {
			// record deleted - so add it
			unsp.add(po, to)
		} else {
			// record added - so delete it
			unsp.del(po)
		}
	}
}


func (db *unwindDb) del(height uint32) {
	db.tdb[height%NumberOfUnwindSubDBs].Del(qdb.KeyType(height))
}


func (db *unwindDb) sync() {
	db.nosyncinprogress = false
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].Sync()
		}
	}
}

func (db *unwindDb) nosync() {
	db.nosyncinprogress = true
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].NoSync()
		}
	}
}

func (db *unwindDb) save() {
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].Flush()
		}
	}
}

func (db *unwindDb) close() {
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].Close()
			db.tdb[i] = nil
		}
	}
}

func (db *unwindDb) idle() bool {
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

func (db *unwindDb) undo(height uint32, unsp *unspentDb) {
	if height != db.lastBlockHeight {
		panic("Unexpected height")
	}

	v := db.dbH(int(height)%NumberOfUnwindSubDBs).Get(qdb.KeyType(height))
	if v == nil {
		panic("Unwind data not found")
	}

	unwindFromReader(bytes.NewReader(v[32:]), unsp)
	db.del(height)

	db.lastBlockHeight--
	v = db.dbH(int(db.lastBlockHeight)%NumberOfUnwindSubDBs).Get(qdb.KeyType(db.lastBlockHeight))
	if v == nil {
		panic("Parent data not found")
	}
	copy(db.lastBlockHash[:], v[:32])
	return
}


func (db *unwindDb) commit(changes *btc.BlockChanges, blhash []byte) {
	if db.lastBlockHeight+1 != changes.Height {
		println(db.lastBlockHeight+1, changes.Height)
		panic("Unexpected height")
	}
	db.lastBlockHeight++
	copy(db.lastBlockHash[:], blhash[0:32])

	f := new(bytes.Buffer)
	f.Write(blhash[0:32])
	if changes.LastKnownHeight - changes.Height < UnwindBufferMaxHistory {
		for k, _ := range changes.AddedTxs {
			writeSpent(f, &k, nil)
		}
		for k, v := range changes.DeledTxs {
			writeSpent(f, &k, v)
		}
	}
	db.dbH(int(changes.Height)%NumberOfUnwindSubDBs).PutExt(qdb.KeyType(changes.Height), f.Bytes(), qdb.NO_CACHE)
	if changes.Height >= UnwindBufferMaxHistory {
		db.del(changes.Height-UnwindBufferMaxHistory)
	}
}


func (db *unwindDb) GetLastBlockHash() (val []byte) {
	if db.lastBlockHeight != 0 {
		val = make([]byte, 32)
		copy(val, db.lastBlockHash[:])
	}
	return
}


func (db *unwindDb) stats() (s string) {
	var cnt int
	for i := range db.tdb {
		cnt += db.dbH(i).Count()
	}
	s = fmt.Sprintf("UNWIND: len:%d  last:%d  defrags:%d/%d\n",
		cnt, db.lastBlockHeight, db.defragCount, db.defragIndex)
	s += "Last block: " + btc.NewUint256(db.lastBlockHash[:]).String() + "\n"
	return
}
