package memdb

import (
	"os"
	"io"
	"fmt"
	"bytes"
	"github.com/piotrnar/gocoin/btc"
)

const (
	UnwindBufferMaxHistory = 3*24*6  // about 3 days...
)

var unwindCache map[uint32] []byte = make(map[uint32] []byte, UnwindBufferMaxHistory)


func unwindFileName(height uint32) string {
	return fmt.Sprint(dirname, "unwind/", height)
}



func unwindFromReader(f io.Reader) {
	for {
		po, to := readSpent(f)
		if po == nil {
			break
		}
		if to != nil {
			// record deleted - so add it
			addUnspent(po, to)
		} else {
			// record added - so delete it
			delUnspent(po)
		}
	}
}


func unwindDelete(height uint32) {
	delete(unwindCache, height)
	//os.Remove(unwindFileName(height))
}


func unwindSync() {
	os.MkdirAll(dirname+"/unwind", 0770)
	for {
		found := false
		for k, v := range unwindCache {
			f, _ := os.Create(unwindFileName(k))
			f.Write(v[:])
			f.Close()
			delete(unwindCache, k)
			found = true
			break
		}
		if !found {
			break
		}
	}
}


func (db UnspentDB) UndoBlockTransactions(height uint32, blhash []byte) (e error) {
	btc.ChSta("UndoBlockTransactions")

	if v, ok := unwindCache[height]; ok {
		unwindFromReader(bytes.NewReader(v[:]))
	} else {
		var f *os.File
		f, e = os.Open(unwindFileName(height))
		if e != nil {
			btc.ChSto("UndoBlockTransactions")
			panic("UndoBlockTransactions: "+e.Error())
			return
		}
		unwindFromReader(f)
		f.Close()            
	}
	unwindDelete(height)

	setLastBlock(blhash)

	btc.ChSto("UndoBlockTransactions")
	return
}


func unwindCommit(changes *btc.BlockChanges) {
	//f, e := os.Create(unwindFileName(changes.Height))
	f := new(bytes.Buffer)
	for k, _ := range changes.AddedTxs {
		writeSpent(f, &k, nil)
	}
	for k, v := range changes.DeledTxs {
		writeSpent(f, &k, v)
	}
	unwindCache[changes.Height] = f.Bytes()[:]
	             
	if changes.Height >= UnwindBufferMaxHistory {
		unwindDelete(changes.Height-UnwindBufferMaxHistory)
	}
}

