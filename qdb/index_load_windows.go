// +build windows

package qdb

import (
	"os"
	"io/ioutil"
)


func (idx *dbidx) load() {
	dats := make(map[uint32] []byte)
	idx.browse(func(k KeyType, v *oneIdx) bool {
		if (v.flags&NO_CACHE)==0 {
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
		}
		return true
	})
}
