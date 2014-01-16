// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Qdb is a fast persistent storage database.

The records are binary blobs that can have a variable length, up to 4GB.

The key must be a unique 64-bit value, most likely a hash of the actual key.

They data is stored on a disk, in a folder specified during the call to NewDB().
There are can be three possible files in that folder
 * qdb.0, qdb.1 - these files store a compact version of the entire database
 * qdb.log - this one stores the changes since the most recent qdb.0 or qdb.1

*/
package qdb

import (
	"os"
	"fmt"
	"sync"
	"bytes"
)

type KeyType uint64

const (
	KeySize = 8
	MaxPending = 1000
	MaxPendingNoSync = 10000

	NO_BROWSE = 0x00000001
	NO_CACHE  = 0x00000002
	BR_ABORT  = 0x00000004
)


type DB struct {
	// folder with the db files
	dir string

	logfile *os.File
	lastvalidlogpos int64
	datseq uint32

	// access mutex:
	mutex sync.Mutex

	//index:
	idx *dbidx

	nosync bool
	pending_recs map[KeyType] bool

	rdfile map[uint32] *os.File
}


type oneIdx struct {
	data []byte

	datseq uint32 // data file index
	datpos uint32 // position of the record in the data file
	datlen uint32 // length of the record in the data file

	flags uint32
}


func (i oneIdx) String() string {
	if i.data==nil {
		return fmt.Sprintf("Nodata:%d:%d:%d", i.datseq, i.datpos, i.datlen)
	} else {
		return fmt.Sprintf("Len(%d):%d:%d:%d", len(i.data), i.datseq, i.datpos, i.datlen)
	}
}


// Creates or opens a new database in the specified folder.
func NewDB(dir string, load bool) (db *DB, e error) {
	cnt("NewDB")
	db = new(DB)
	if len(dir)>0 && dir[len(dir)-1]!='\\' && dir[len(dir)-1]!='/' {
		dir += string(os.PathSeparator)
	}
	os.MkdirAll(dir, 0770)
	db.dir = dir
	db.rdfile = make(map[uint32] *os.File)
	db.pending_recs = make(map[KeyType] bool, MaxPending)
	db.idx = NewDBidx(db)
	if load {
		db.idx.load()
	}
	db.datseq = db.idx.max_dat_seq+1
	return
}


// Returns number of records in the DB
func (db *DB) Count() (l int) {
	db.mutex.Lock()
	l = db.idx.size()
	db.mutex.Unlock()
	return
}


// Browses through all the DB records calling the walk function for each record.
// If the walk function returns false, it aborts the browsing and returns.
func (db *DB) Browse(walk func(key KeyType, value []byte) uint32) {
	db.mutex.Lock()
	db.idx.browse(func(k KeyType, v *oneIdx) bool {
		if (v.flags&NO_BROWSE)!=0 {
			return true
		}
		if v.data == nil {
			db.loadrec(v)
		}
		res := walk(k, v.data)
		if (res&NO_BROWSE)!=0 {
			v.flags |= NO_BROWSE
		}
		if (res&NO_CACHE)!=0 {
			v.flags |= NO_CACHE
			v.data = nil
		}
		return (res&BR_ABORT)==0
	})
	//println("br", db.dir, "done")
	db.mutex.Unlock()
}


// works almost like normal browse except that it also returns non-browsable records
func (db *DB) BrowseAll(walk func(key KeyType, value []byte) uint32) {
	db.mutex.Lock()
	db.idx.browse(func(k KeyType, v *oneIdx) bool {
		if v.data == nil {
			db.loadrec(v)
		}
		res := walk(k, v.data)
		if (v.flags&NO_CACHE)!=0 {
			v.data = nil
		}
		return (res&BR_ABORT)==0
	})
	//println("br", db.dir, "done")
	db.mutex.Unlock()
}


func (db *DB) Get(key KeyType) (value []byte) {
	db.mutex.Lock()
	idx := db.idx.get(key)
	if idx!=nil {
		if idx.data == nil {
			db.loadrec(idx)
		}
		value = idx.data
	}
	//fmt.Printf("get %016x -> %s\n", key, hex.EncodeToString(value))
	db.mutex.Unlock()
	return
}


// Adds or updates record with a given key.
func (db *DB) Put(key KeyType, value []byte) {
	db.mutex.Lock()
	db.idx.memput(key, &oneIdx{data:value, datlen:uint32(len(value))})
	db.pending_recs[key] = true
	if db.syncneeded() {
		go func() {
			db.sync()
			db.mutex.Unlock()
		}()
	} else {
		db.mutex.Unlock()
	}
}


// Adds or updates record with a given key.
func (db *DB) PutExt(key KeyType, value []byte, flags uint32) {
	db.mutex.Lock()
	//fmt.Printf("put %016x %s\n", key, hex.EncodeToString(value))
	db.idx.memput(key, &oneIdx{data:value, datlen:uint32(len(value)), flags:flags})
	db.pending_recs[key] = true
	if db.syncneeded() {
		go func() {
			db.sync()
			db.mutex.Unlock()
		}()
	} else {
		db.mutex.Unlock()
	}
}


// Removes record with a given key.
func (db *DB) Del(key KeyType) {
	//println("del", hex.EncodeToString(key[:]))
	db.mutex.Lock()
	db.idx.memdel(key)
	db.pending_recs[key] = true
	if db.syncneeded() {
		go func() {
			db.sync()
			db.mutex.Unlock()
		}()
	} else {
		db.mutex.Unlock()
	}
}


// Defragments the DB on the disk.
// Return true if defrag hes been performed, and false if was not needed.
func (db *DB) Defrag() (doing bool) {
	db.mutex.Lock()
	doing = db.idx.extra_space_used > (db.idx.disk_space_needed/2) // defrag if we waste more than 50%
	if doing {
		cnt("DefragYes")
		go func() {
			db.defrag()
			db.mutex.Unlock()
		}()
	} else {
		db.mutex.Unlock()
	}
	return
}


// Disable writing changes to disk.
func (db *DB) NoSync() {
	cnt("NoSync")
	db.mutex.Lock()
	db.nosync = true
	db.mutex.Unlock()
}


// Write all the pending changes to disk now.
// Re enable syncing if it has been disabled.
func (db *DB) Sync() {
	cnt("Sync")
	db.mutex.Lock()
	db.nosync = false
	go func() {
		db.sync()
		db.mutex.Unlock()
	}()
}


// Close the database.
// Writes all the pending changes to disk.
func (db *DB) Close() {
	db.mutex.Lock()
	db.sync()
	if db.logfile!=nil {
		db.logfile.Close()
		db.logfile = nil
	}
	db.idx.close()
	db.idx = nil
	for _, f := range db.rdfile {
		f.Close()
	}
	db.mutex.Unlock()
}


func (db *DB) defrag() {
	db.datseq++
	if db.logfile!=nil {
		db.logfile.Close()
		db.logfile = nil
	}
	db.checklogfile()
	used := make(map[uint32]bool, 10)
	db.idx.browse(func(key KeyType, rec *oneIdx) bool {
		if rec.data==nil {
			db.loadrec(rec)
		}
		rec.datpos = uint32(db.addtolog(nil, key, rec.data))
		rec.datseq = db.datseq
		used[rec.datseq] = true
		return true
	})

	// first write & flush the data file:
	db.logfile.Sync()

	// now the index:
	db.idx.writedatfile() // this will close the file

	db.cleanupold(used)
	db.idx.extra_space_used = 0
}


func (db *DB) sync() {
	if len(db.pending_recs)>0 {
		cnt("SyncOK")
		bidx := new(bytes.Buffer)
		db.checklogfile()
		for k, _ := range db.pending_recs {
			rec := db.idx.get(k)
			if rec != nil {
				fpos := db.addtolog(nil, k, rec.data)
				rec.datlen = uint32(len(rec.data))
				rec.datpos = uint32(fpos)
				rec.datseq = db.datseq
				db.idx.addtolog(bidx, k, rec)
				if (rec.flags&NO_CACHE)!=0 {
					rec.data = nil
				}
			} else {
				db.idx.deltolog(bidx, k)
			}
		}
		db.idx.writebuf(bidx.Bytes())
	} else {
		cnt("SyncNO")
	}
	db.pending_recs = make(map[KeyType] bool, MaxPending)
}


func (db *DB) Flush() {
	cnt("Flush")
	if db.logfile!=nil {
		db.logfile.Sync()
	}
	if db.idx.logfile!=nil {
		db.idx.logfile.Sync()
	}
}


func (db *DB) syncneeded() bool {
	if len(db.pending_recs) > MaxPendingNoSync {
		println("SyncNeedBig")
		cnt("SyncNeedBig")
		return true
	}
	if !db.nosync && len(db.pending_recs) > MaxPending {
		cnt("SyncNeedSmall")
		return true
	}
	return false
}
