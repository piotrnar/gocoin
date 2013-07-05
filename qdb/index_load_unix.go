// +build !windows

package qdb

import (
)

func (idx *dbidx) load() {
	idx.browse(func(k KeyType, v *oneIdx) bool {
		if (v.flags&NO_CACHE)==0 {
			db.loadrec(v)
		}
		return true
	})
	return
}
