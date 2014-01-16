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
		os.RemoveAll(dir+"unspent3")
	}

	if btc.AbortNow {
		return nil
	}
	db.unwind = newUnwindDB(dir+"unspent3"+string(os.PathSeparator))

	if btc.AbortNow {
		return nil
	}
	db.unspent = newUnspentDB(dir+"unspent3"+string(os.PathSeparator), db.unwind.lastBlockHeight)

	return &db
}


func (db UnspentDB) GetLastBlockHash() ([]byte) {
	return db.unwind.GetLastBlockHash()
}


func (db UnspentDB) CommitBlockTxs(changes *btc.BlockChanges, blhash []byte) (e error) {
	// First the unwind data
	db.nosync()
	db.unspent.lastHeight = changes.Height
	db.unwind.commit(changes, blhash)
	db.unspent.commit(changes)
	db.sync()
	return
}

func (db UnspentDB) GetStats() (s string) {
	s += db.unspent.stats()
	s += db.unwind.stats()
	return
}


func (db UnspentDB) SetTxNotify(fn btc.TxNotifyFunc) {
	db.unspent.notifyTx = fn
}

// Flush all the data to files
func (db UnspentDB) sync() {
	db.unwind.sync()
	db.unspent.sync()
}

func (db UnspentDB) nosync() {
	db.unwind.nosync()
	db.unspent.nosync()
}


func (db UnspentDB) Close() {
	db.unwind.close()
	db.unspent.close()
}


func (db UnspentDB) Idle() bool {
	if db.unspent.idle() {
		return true
	}
	return db.unwind.idle()
}


func (db UnspentDB) Save() {
	db.unwind.save()
	db.unspent.save()
}

func (db UnspentDB) UndoBlockTransactions(height uint32) {
	db.nosync()
	db.unwind.undo(height, db.unspent)
	db.unspent.lastHeight = height-1
	db.sync()
}


func (db UnspentDB) UnspentGet(po *btc.TxPrevOut) (res *btc.TxOut, e error) {
	return db.unspent.get(po)
}

func (db UnspentDB) GetAllUnspent(addr []*btc.BtcAddr, quick bool) (res btc.AllUnspentTx) {
	return db.unspent.GetAllUnspent(addr, quick)
}

func init() {
	btc.NewUnspentDb = NewDb
}
