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
	"sync/atomic"
)

type KeyType uint64

// defrag if we waste more than this percent of disk space (use atomic functoin to modify it)
var (
	DefragPercentVal uint32 = 150
	MaxPending uint32       = 1000
	MaxPendingNoSync uint32 = 100000
	ExtraMemoryConsumed int64  // if we are using the glibc memory manager
	ExtraMemoryAllocCnt int64  // if we are using the glibc memory manager
)

const (
	KeySize = 8

	NO_BROWSE  = 0x00000001
	NO_CACHE   = 0x00000002
	BR_ABORT   = 0x00000004
	YES_CACHE  = 0x00000008
	YES_BROWSE = 0x00000010
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
	data data_ptr_t

	datseq uint32 // data file index
	datpos uint32 // position of the record in the data file
	datlen uint32 // length of the record in the data file

	flags uint32
}


type QdbWalkFunction func(key KeyType, val []byte) uint32


func (i oneIdx) String() string {
	if i.data==nil {
		return fmt.Sprintf("Nodata:%d:%d:%d", i.datseq, i.datpos, i.datlen)
	} else {
		return fmt.Sprintf("YesData:%d:%d:%d", i.datseq, i.datpos, i.datlen)
	}
}


// Creates or opens a new database in the specified folder.
func NewDBExt(_db **DB, dir string, load bool, walk QdbWalkFunction, recs uint) (e error) {
	cnt("NewDB")
	db := new(DB)
	*_db = db
	if len(dir)>0 && dir[len(dir)-1]!='\\' && dir[len(dir)-1]!='/' {
		dir += string(os.PathSeparator)
	}
	os.MkdirAll(dir, 0770)
	db.dir = dir
	db.rdfile = make(map[uint32] *os.File)
	db.pending_recs = make(map[KeyType] bool, MaxPending)
	db.idx = NewDBidx(db, recs)
	if load {
		db.idx.load(walk)
	}
	db.datseq = db.idx.max_dat_seq+1
	return
}


func NewDBrowse(db **DB, dir string, walk QdbWalkFunction, recs uint) (e error) {
	return NewDBExt(db, dir, true, walk, recs)
}

func NewDB(dir string, load bool) (*DB, error) {
	var db *DB
	e := NewDBExt(&db, dir, load, nil, 0)
	return db, e
}

func NewDBCnt(dir string, load bool, recs uint) (*DB, error) {
	var db *DB
	e := NewDBExt(&db, dir, load, nil, recs)
	return db, e
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
func (db *DB) Browse(walk QdbWalkFunction) {
	db.mutex.Lock()
	db.idx.browse(func(k KeyType, v *oneIdx) bool {
		if (v.flags&NO_BROWSE)!=0 {
			return true
		}
		db.loadrec(v)
		res := walk(k, v.Slice())
		v.aply_browsing_flags(res)
		v.freerec()
		return (res&BR_ABORT)==0
	})
	//println("br", db.dir, "done")
	db.mutex.Unlock()
}


// works almost like normal browse except that it also returns non-browsable records
func (db *DB) BrowseAll(walk QdbWalkFunction) {
	db.mutex.Lock()
	db.idx.browse(func(k KeyType, v *oneIdx) bool {
		db.loadrec(v)
		res := walk(k, v.Slice())
		v.aply_browsing_flags(res)
		v.freerec()
		return (res&BR_ABORT)==0
	})
	//println("br", db.dir, "done")
	db.mutex.Unlock()
}


func (db *DB) Get(key KeyType) (value []byte) {
	db.mutex.Lock()
	idx := db.idx.get(key)
	if idx!=nil {
		db.loadrec(idx)
		idx.aply_browsing_flags(YES_CACHE)  // we are giving out the pointer, so keep it in cache
		value = idx.Slice()
	}
	//fmt.Printf("get %016x -> %s\n", key, hex.EncodeToString(value))
	db.mutex.Unlock()
	return
}


// Use this one inside Browse
func (db *DB) GetNoMutex(key KeyType) (value []byte) {
	idx := db.idx.get(key)
	if idx!=nil {
		db.loadrec(idx)
		value = idx.Slice()
	}
	//fmt.Printf("get %016x -> %s\n", key, hex.EncodeToString(value))
	return
}


// Adds or updates record with a given key.
func (db *DB) Put(key KeyType, value []byte) {
	db.mutex.Lock()
	db.idx.memput(key, newIdx(value, 0))
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
	db.idx.memput(key, newIdx(value, flags))
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


func (db *DB) ApplyFlags(key KeyType, fl uint32) {
	db.mutex.Lock()
	if idx:=db.idx.get(key); idx!=nil {
		idx.aply_browsing_flags(fl)
	}
	db.mutex.Unlock()
}



// Defragments the DB on the disk.
// Return true if defrag hes been performed, and false if was not needed.
func (db *DB) Defrag() (doing bool) {
	db.mutex.Lock()
	doing = db.idx.extra_space_used > (uint64(DefragPercentVal)*db.idx.disk_space_needed/100)
	if doing {
		cnt("DefragYes")
		go func() {
			db.defrag()
			db.mutex.Unlock()
		}()
	} else {
		cnt("DefragNo")
		db.mutex.Unlock()
	}
	return
}


// Disable writing changes to disk.
func (db *DB) NoSync() {
	db.mutex.Lock()
	db.nosync = true
	db.mutex.Unlock()
}


// Write all the pending changes to disk now.
// Re enable syncing if it has been disabled.
func (db *DB) Sync() {
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
		db.loadrec(rec)
		rec.datpos = uint32(db.addtolog(nil, key, rec.Slice()))
		rec.datseq = db.datseq
		used[rec.datseq] = true
		rec.freerec()
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
				fpos := db.addtolog(nil, k, rec.Slice())
				//rec.datlen = uint32(len(rec.data))
				rec.datpos = uint32(fpos)
				rec.datseq = db.datseq
				db.idx.addtolog(bidx, k, rec)
				if (rec.flags&NO_CACHE)!=0 {
					rec.FreeData()
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
	if len(db.pending_recs) > int(MaxPendingNoSync) {
		cnt("SyncNeedBig")
		return true
	}
	if !db.nosync && len(db.pending_recs) > int(MaxPending) {
		cnt("SyncNeedSmall")
		return true
	}
	return false
}


// Defrag files on disk if we waste more than this percent of disk space
func SetDefragPercent(val uint32) {
	atomic.StoreUint32(&DefragPercentVal, val)
}

// How many records to buffer in memory before writing them to disk
func SetMaxPending(sync uint32, nosync uint32) {
	atomic.StoreUint32(&MaxPending, sync)
	atomic.StoreUint32(&MaxPendingNoSync, nosync)
}


func (idx *oneIdx) freerec() {
	if (idx.flags&NO_CACHE) != 0 {
		idx.FreeData()
	}
}


func (v *oneIdx) aply_browsing_flags(res uint32) {
	if (res&NO_BROWSE)!=0 {
		v.flags |= NO_BROWSE
	} else if (res&YES_BROWSE)!=0 {
		v.flags &= ^uint32(NO_BROWSE)
	}

	if (res&NO_CACHE)!=0 {
		v.flags |= NO_CACHE
	} else if (res&YES_CACHE)!=0 {
		v.flags &= ^uint32(NO_CACHE)
	}
}
