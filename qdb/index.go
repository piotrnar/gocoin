package qdb

import (
	"os"
	"io/ioutil"
)


type dbidx struct {
	db *DB
	path string
	logfile *os.File
	datfile_idx int
	version_seq uint32
	max_dat_seq uint32

	index map[KeyType] *oneIdx
	cnt int
	needsdefrag bool
}

func NewDBidx(db *DB) (idx *dbidx) {
	idx = new(dbidx)
	idx.db = db
	idx.path = db.dir+"qdbidx."
	idx.index = make(map[KeyType] *oneIdx)
	used := make(map[uint32]bool, 10)
	idx.loaddat(used)
	idx.loadlog(used)
	idx.db.cleanupold(used)

	// pre-load data
	dats := make(map[uint32] []byte, len(used))
	for k, _ := range used {
		dats[k], _ = ioutil.ReadFile(db.seq2fn(k))
		if dats[k]==nil {
			println("Database corrupt - missing file:", db.seq2fn(k))
			os.Exit(1)
		}
	}
	idx.browse(func(k KeyType, v *oneIdx) bool {
		v.data = make([]byte, v.datlen)
		copy(v.data, dats[v.datseq][v.datpos:v.datpos+v.datlen])
		return true
	})

	return
}

func (idx *dbidx) size() int {
	return idx.cnt
}


func (idx *dbidx) get(k KeyType) *oneIdx {
	return idx.index[k]
}


func (idx *dbidx) memput(k KeyType, rec *oneIdx) {
	if _, ok := idx.index[k]; !ok {
		idx.cnt++
	} else {
		idx.needsdefrag = true // defrag will be needed only if we replaced an existing record
	}
	idx.index[k] = rec
	if rec.datseq>idx.max_dat_seq {
		idx.max_dat_seq = rec.datseq
	}
}


func (idx *dbidx) memdel(k KeyType) {
	if _, ok := idx.index[k]; ok {
		idx.cnt--
		idx.needsdefrag = true
		delete(idx.index, k)
	}
}

func (idx *dbidx) put(k KeyType, rec *oneIdx) {
	idx.memput(k, rec)
	idx.addtolog(nil, k, rec)
}


func (idx *dbidx) del(k KeyType) {
	idx.memdel(k)
	idx.deltolog(nil, k)
}


func (idx *dbidx) browse(walk func(key KeyType, idx *oneIdx) bool) {
	for k, v := range idx.index {
		if !walk(k, v) {
			break
		}
	}
}

func (idx *dbidx) close() {
	if idx.logfile!= nil {
		idx.logfile.Close()
		idx.logfile = nil
	}
	idx.index = nil
}
