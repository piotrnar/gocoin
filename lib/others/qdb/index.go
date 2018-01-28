package qdb

import (
	"os"
	"io/ioutil"
)


type QdbIndex struct {
	db *DB
	IdxFilePath string
	file *os.File
	DatfileIndex int
	VersionSequence uint32
	MaxDatfileSequence uint32

	Index map[KeyType] *oneIdx

	DiskSpaceNeeded uint64
	ExtraSpaceUsed uint64
}

func NewDBidx(db *DB, recs uint) (idx *QdbIndex) {
	idx = new(QdbIndex)
	idx.db = db
	idx.IdxFilePath = db.Dir+"qdbidx."
	if recs==0 {
		idx.Index = make(map[KeyType] *oneIdx)
	} else {
		idx.Index = make(map[KeyType] *oneIdx, recs)
	}
	used := make(map[uint32]bool, 10)
	idx.loaddat(used)
	idx.loadlog(used)
	idx.db.cleanupold(used)
	return
}


func (idx *QdbIndex) load(walk QdbWalkFunction) {
	dats := make(map[uint32] []byte)
	idx.browse(func(k KeyType, v *oneIdx) bool {
		if walk!=nil || (v.flags&NO_CACHE)==0 {
			dat := dats[v.DataSeq]
			if dat == nil {
				dat, _ = ioutil.ReadFile(idx.db.seq2fn(v.DataSeq))
				if dat==nil {
					println("Database corrupt - missing file:", idx.db.seq2fn(v.DataSeq))
					os.Exit(1)
				}
				dats[v.DataSeq] = dat
			}
			v.SetData(dat[v.datpos:v.datpos+v.datlen])
			if walk!=nil {
				res := walk(k, v.Slice())
				v.aply_browsing_flags(res)
				v.freerec()
			}
		}
		return true
	})
}


func (idx *QdbIndex) size() int {
	return len(idx.Index)
}


func (idx *QdbIndex) get(k KeyType) *oneIdx {
	return idx.Index[k]
}


func (idx *QdbIndex) memput(k KeyType, rec *oneIdx) {
	if prv, ok := idx.Index[k]; ok {
		prv.FreeData()
		dif := uint64(24+prv.datlen)
		if !idx.db.VolatileMode {
			idx.ExtraSpaceUsed += dif
			idx.DiskSpaceNeeded -= dif
		}
	}
	idx.Index[k] = rec

	if !idx.db.VolatileMode {
		idx.DiskSpaceNeeded += uint64(24+rec.datlen)
	}
	if rec.DataSeq>idx.MaxDatfileSequence {
		idx.MaxDatfileSequence = rec.DataSeq
	}
}


func (idx *QdbIndex) memdel(k KeyType) {
	if cur, ok := idx.Index[k]; ok {
		cur.FreeData()
		dif := uint64(12+cur.datlen)
		if !idx.db.VolatileMode {
			idx.ExtraSpaceUsed += dif
			idx.DiskSpaceNeeded -= dif
		}
		delete(idx.Index, k)
	}
}

func (idx *QdbIndex) put(k KeyType, rec *oneIdx) {
	idx.memput(k, rec)
	if idx.db.VolatileMode {
		return
	}
	idx.addtolog(nil, k, rec)
}


func (idx *QdbIndex) del(k KeyType) {
	idx.memdel(k)
	if idx.db.VolatileMode {
		return
	}
	idx.deltolog(nil, k)
}


func (idx *QdbIndex) browse(walk func(key KeyType, idx *oneIdx) bool) {
	for k, v := range idx.Index {
		if !walk(k, v) {
			break
		}
	}
}

func (idx *QdbIndex) close() {
	if idx.file!= nil {
		idx.file.Close()
		idx.file = nil
	}
	idx.Index = nil
}
