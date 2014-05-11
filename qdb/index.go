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

	disk_space_needed uint64
	extra_space_used uint64
}

func NewDBidx(db *DB, recs uint) (idx *dbidx) {
	idx = new(dbidx)
	idx.db = db
	idx.path = db.dir+"qdbidx."
	if recs==0 {
		idx.index = make(map[KeyType] *oneIdx)
	} else {
		idx.index = make(map[KeyType] *oneIdx, recs)
	}
	used := make(map[uint32]bool, 10)
	idx.loaddat(used)
	idx.loadlog(used)
	idx.db.cleanupold(used)
	return
}


func (idx *dbidx) load(walk func(key KeyType, value []byte) uint32) {
	dats := make(map[uint32] []byte)
	idx.browse(func(k KeyType, v *oneIdx) bool {
		if walk!=nil || (v.flags&NO_CACHE)==0 {
			dat := dats[v.datseq]
			if dat == nil {
				dat, _ = ioutil.ReadFile(idx.db.seq2fn(v.datseq))
				if dat==nil {
					println("Database corrupt - missing file:", idx.db.seq2fn(v.datseq))
					os.Exit(1)
				}
				dats[v.datseq] = dat
			}
			v.data = make([]byte, v.datlen)
			copy(v.data, dat[v.datpos:v.datpos+v.datlen])
			if walk!=nil {
				res := walk(k, v.data)
				applyBrowsingFlags(res, v)
			}
		}
		return true
	})
}


func (idx *dbidx) size() int {
	return idx.cnt
}


func (idx *dbidx) get(k KeyType) *oneIdx {
	return idx.index[k]
}


func (idx *dbidx) memput(k KeyType, rec *oneIdx) {
	if prv, ok := idx.index[k]; !ok {
		idx.cnt++
	} else {
		dif := uint64(24+prv.datlen)
		idx.extra_space_used += dif
		idx.disk_space_needed -= dif
	}
	idx.index[k] = rec
	idx.disk_space_needed += uint64(24+rec.datlen)
	if rec.datseq>idx.max_dat_seq {
		idx.max_dat_seq = rec.datseq
	}
}


func (idx *dbidx) memdel(k KeyType) {
	if cur, ok := idx.index[k]; ok {
		idx.cnt--
		dif := uint64(12+cur.datlen)
		idx.extra_space_used += dif
		idx.disk_space_needed -= dif
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
