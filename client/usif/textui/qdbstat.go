// If it doesnt build, just remove this file

package textui

import (
	"fmt"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/lib/qdb"
	"github.com/piotrnar/gocoin/client/common"
)


func qdb_stats(par string) {
	fmt.Print(qdb.GetStats())
	var DiskSpaceNeeded, ExtraSpaceUsed uint64
	for i:=0; i<chain.NumberOfUnspentSubDBs; i++ {
		db := common.BlockChain.Unspent.DbN(i)
		db.Mutex.Lock()
		DiskSpaceNeeded += db.Idx.DiskSpaceNeeded
		ExtraSpaceUsed += db.Idx.ExtraSpaceUsed
		db.Mutex.Unlock()
	}
	fmt.Printf("DiskSpaceNeeded : %14.6f MB\n", float64(DiskSpaceNeeded)/(1e6))
	fmt.Printf("ExtraSpaceUsed  : %14.6f MB\n", float64(ExtraSpaceUsed)/(1e6))
	fmt.Println("QDB Extra mem:", qdb.ExtraMemoryConsumed>>20, "MB in", qdb.ExtraMemoryAllocCnt, "parts")
}


func init() {
	newUi("qdbstats qs", false, qdb_stats, "Show statistics of QDB engine")
}
