package qdb

import (
	"io"
	"fmt"
	"bytes"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/qdb"
)

const (
	UnwindBufferMaxHistory = 7*24*6  // we keep unwind history for about 7 days...
)

type unwindDb struct {
	tdb *qdb.DB
	lastBlockHeight uint32
	lastBlockHash [32]byte
	defragCount uint64
}


func newUnwindDB(dir string) (db *unwindDb) {
	db = new(unwindDb)
	db.tdb, _ = qdb.NewDB(dir)
	db.lastBlockHeight = 0
	db.tdb.Browse(func(k qdb.KeyType, v []byte) bool {
		h := uint32(k)
		if h > db.lastBlockHeight {
			db.lastBlockHeight = h
			copy(db.lastBlockHash[:], v[:32])
		}
		return true
	})
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
	db.tdb.Del(qdb.KeyType(height))
}


func (db *unwindDb) sync() {
	db.tdb.Sync()
}

func (db *unwindDb) nosync() {
	db.tdb.NoSync()
}

func (db *unwindDb) save() {
	db.tdb.Defrag()
}

func (db *unwindDb) close() {
	db.tdb.Close()
	db.tdb = nil
}

func (db *unwindDb) idle() bool {
	if db.tdb.Defrag() {
		db.defragCount++
		return true
	}
	return false
}

func (db *unwindDb) undo(height uint32, unsp *unspentDb) {
	if height != db.lastBlockHeight {
		panic("Unexpected height")
	}
	
	v := db.tdb.Get(qdb.KeyType(height))
	if v == nil {
		panic("Unwind data not found")
	}

	unwindFromReader(bytes.NewReader(v[32:]), unsp)
	db.del(height)

	db.lastBlockHeight--
	v = db.tdb.Get(qdb.KeyType(db.lastBlockHeight))
	if v == nil {
		panic("Parent data not found")
	}
	copy(db.lastBlockHash[:], v[:32])
	return
}


func (db *unwindDb) commit(changes *btc.BlockChanges, blhash []byte) {
	if db.lastBlockHeight+1 != changes.Height {
		panic("Unexpected height")
	}
	db.lastBlockHeight++
	copy(db.lastBlockHash[:], blhash[0:32])

	f := new(bytes.Buffer)
	f.Write(blhash[0:32])
	for k, _ := range changes.AddedTxs {
		writeSpent(f, &k, nil)
	}
	for k, v := range changes.DeledTxs {
		writeSpent(f, &k, v)
	}
	db.tdb.Put(qdb.KeyType(changes.Height), f.Bytes())
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
	s = fmt.Sprintf("UNWIND: len:%d  last:%d  defrags:%d\n", 
		db.tdb.Count(), db.lastBlockHeight, db.defragCount)
	s += "Last block: " + btc.NewUint256(db.lastBlockHash[:]).String() + "\n"
	return
}

