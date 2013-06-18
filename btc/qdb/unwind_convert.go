package qdb

import (
	"os"
	"fmt"
	"github.com/piotrnar/qdb"
)


func convertOldUnwindDb(dir string) bool {
	olddb, _ := qdb.NewDB(dir)
	if olddb == nil {
		return false
	}


	var newdb [0x100] *qdb.DB
	for i := range newdb {
		newdb[i], _ = qdb.NewDB(dir+fmt.Sprintf("%02x/", i))
	}

	olddb.Browse(func(k qdb.KeyType, v []byte) bool {
		newdb[k&0xff].Put(k, v)
		return true
	})
	olddb.Close()
	for i := range newdb {
		newdb[i].Close()
	}

	dir = dir[:len(dir)-1] // remove tra trailing slash
	rento := dir+".obsolete"
	e := os.Rename(dir, rento)
	if e != nil {
		println(e.Error())
	} else {
		fmt.Println("Unwind database has been converted to a new format.")
		fmt.Println("The old version was ranamed to "+rento)
		fmt.Println("You may delete it, if you don't plan to go back to old tag.")
	}

	return true
}
