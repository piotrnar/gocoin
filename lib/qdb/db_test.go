package qdb

import (
	"os"
	"fmt"
	"time"
	"bytes"
	"testing"
	mr "math/rand"
	cr "crypto/rand"
	"encoding/hex"
)

const (
	dbname = "test"
	oneRound = 10000
	delRound = 1000
)

func getRecSize() int {
	return 4
	//return mr.Intn(4096)
}


func kim(v []byte) bool {
	return (mr.Int63()&1)==0
}


func dumpidx(db *DB) {
	println("index")
	for k, v := range db.Idx.Index {
		println(k2s(k), v.datpos, v.datlen)
	}
}


func TestDatabase(t *testing.T) {
	var key KeyType
	var val, v []byte
	var db *DB
	var e error

	os.RemoveAll(dbname)
	mr.Seed(time.Now().UnixNano())

	db, e = NewDB(dbname, true)
	if e != nil {
		t.Error("Cannot create db")
		return
	}

	// Add oneRound random records
	for i:=0; i<oneRound; i++ {
		vlen := getRecSize()
		val = make([]byte, vlen)
		key = KeyType(mr.Int63())
		cr.Read(val[:])
		db.Put(key, val)
	}
	db.Close()

	// Reopen DB, verify, defrag and close
	db, e = NewDB(dbname, true)
	if e != nil {
		t.Error("Cannot reopen db")
		return
	}
	if db.Count()!=oneRound {
		t.Error("Bad count", db.Count(), oneRound)
		return
	}
	//dumpidx(db)
	v = db.Get(key)
	if !bytes.Equal(val, v) {
		t.Error("Key data mismatch ", k2s(key), "/", hex.EncodeToString(val), "/", hex.EncodeToString(v))
		return
	}
	if db.Count() != oneRound {
		t.Error("Wrong number of records", db.Count(), oneRound)
	}
	db.Defrag(false)
	db.Close()

	// Reopen DB, verify, add oneRound more records and Close
	db, e = NewDB(dbname, true)
	if e != nil {
		t.Error("Cannot reopen db")
		return
	}
	v = db.Get(key)
	if !bytes.Equal(val[:], v[:]) {
		t.Error("Key data mismatch")
	}
	if db.Count() != oneRound {
		t.Error("Wrong number of records", db.Count())
	}
	db.NoSync()
	for i:=0; i<oneRound; i++ {
		vlen := getRecSize()
		val = make([]byte, vlen)
		key = KeyType(mr.Int63())
		cr.Read(val[:])
		db.Put(key, val)
	}
	db.Sync()
	db.Close()

	// Reopen DB, verify, defrag and close
	db, e = NewDB(dbname, true)
	if e != nil {
		t.Error("Cannot reopen db")
		return
	}
	v = db.Get(key)
	if !bytes.Equal(val[:], v[:]) {
		t.Error("Key data mismatch")
	}
	if db.Count() != 2*oneRound {
		t.Error("Wrong number of records", db.Count())
		return
	}
	db.Defrag(true)
	db.Close()

	// Reopen DB, verify, close...
	db, e = NewDB(dbname, true)
	if e != nil {
		t.Error("Cannot reopen db")
		return
	}
	v = db.Get(key)
	if !bytes.Equal(val[:], v[:]) {
		t.Error("Key data mismatch")
	}
	if db.Count() != 2*oneRound {
		t.Error("Wrong number of records", db.Count())
	}
	db.Close()

	// Reopen, delete 100 records, close...
	db, e = NewDB(dbname, true)
	if e != nil {
		t.Error("Cannot reopen db")
		return
	}

	var keys []KeyType
	db.Browse(func (key KeyType, v []byte) uint32 {
		keys = append(keys, key)
		if len(keys)<delRound {
			return 0
		} else {
			return BR_ABORT
		}
	})
	for i := range keys {
		db.Del(keys[i])
	}
	db.Close()

	// Reopen DB, verify, close...
	db, e = NewDB(dbname, true)
	if db.Count() != 2*oneRound-delRound {
		t.Error("Wrong number of records", db.Count())
	}
	db.Close()

	// Reopen DB, verify, close...
	db, e = NewDB(dbname, true)
	db.Defrag(false)
	if db.Count() != 2*oneRound-delRound {
		t.Error("Wrong number of records", db.Count())
	}
	db.Close()

	// Reopen DB, verify, close...
	db, e = NewDB(dbname, true)
	if db.Count() != 2*oneRound-delRound {
		t.Error("Wrong number of records", db.Count())
	}
	db.Close()

	os.RemoveAll(dbname)
}

func k2s(k KeyType) string {
	return fmt.Sprintf("%16x", k)
}
