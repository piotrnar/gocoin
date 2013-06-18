package qdb

import (
	"os"
	"fmt"
	"github.com/piotrnar/qdb"
)


func convertOldUnwindDb(dir string) bool {
	olddb, _ := qdb.NewDB(dir)
	if olddb == nil || olddb.Count()==0 {
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

	os.Mkdir(dir+"old/", 0770)
	os.Rename(dir+"qdb.0", dir+"old/qdb.0")
	os.Rename(dir+"qdb.1", dir+"old/qdb.1")
	os.Rename(dir+"qdb.log", dir+"old/qdb.log")

	fmt.Println("Unwind database has been converted to a new format.")
	fmt.Println("The old files were moved to "+dir+"old/")
	fmt.Println("Delete them when you don't plan to go back to a previous s/w version anymore.")

	return true
}
