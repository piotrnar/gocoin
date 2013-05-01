package qdb

import (
	"os"
	"github.com/piotrnar/gocoin/btc"
)

type UnspentDB struct {
	unspent *unspentDb
	unwind *unwindDb
}


func NewDb(dir string, init bool) btc.UnspentDB {
	var db UnspentDB
	
	if init {
		os.RemoveAll(dir+"unspent/")
		os.RemoveAll(dir+"unspent/unwind/")
	}

	db.unspent = newUnspentDB(dir+"unspent/")
	db.unwind = newUnwindDB(dir+"unspent/unwind/")
	
	return &db
}


func (db UnspentDB) GetLastBlockHash() ([]byte) {
	return db.unwind.GetLastBlockHash()
}


func (db UnspentDB) CommitBlockTxs(changes *btc.BlockChanges, blhash []byte) (e error) {
	// First the unwind data
	db.unwind.commit(changes, blhash)
	db.unspent.commit(changes)
	return
}

func (db UnspentDB) GetStats() (s string) {
	s += db.unspent.stats()
	s += db.unwind.stats()
	return
}


// Flush all the data to files
func (db UnspentDB) Sync() {
	db.unwind.sync()
	db.unspent.sync()
}

func (db UnspentDB) NoSync() {
	db.unwind.nosync()
	db.unspent.nosync()
}


func (db UnspentDB) Close() {
	db.unwind.close()
	db.unspent.close()
}


func (db UnspentDB) Idle() {
	if !db.unspent.idle() {
		//println("No Unspent to defrag")
		db.unwind.idle()
	}
}


func (db UnspentDB) Save() {
	db.unwind.save()
	db.unspent.save()
}

func (db UnspentDB) UndoBlockTransactions(height uint32) {
	db.unwind.undo(height, db.unspent)
}


func (db UnspentDB) UnspentGet(po *btc.TxPrevOut) (res *btc.TxOut, e error) {
	return db.unspent.get(po)
}

func (db UnspentDB) GetAllUnspent(addr []*btc.BtcAddr) (res btc.AllUnspentTx) {
	return db.unspent.GetAllUnspent(addr)
}

func init() {
	btc.NewUnspentDb = NewDb
}

