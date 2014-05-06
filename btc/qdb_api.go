package btc

import (
	"os"
	"fmt"
	"encoding/binary"
)


type UnspentDB struct {
	unspent *unspentDb
	unwind *unwindDb
}


func NewUnspentDb(dir string, init bool) *UnspentDB {
	db := new(UnspentDB)

	if init {
		os.RemoveAll(dir+"unspent3")
	}

	if AbortNow {
		return nil
	}
	db.unwind = newUnwindDB(dir+"unspent3"+string(os.PathSeparator))

	if AbortNow {
		return nil
	}
	db.unspent = newUnspentDB(dir+"unspent3"+string(os.PathSeparator), db.unwind.lastBlockHeight)

	fmt.Println("Number of cached stealth outputs:", len(db.unspent.stealthOuts))

	return db
}


func (db *UnspentDB) GetLastBlockHash() ([]byte) {
	return db.unwind.GetLastBlockHash()
}


func (db *UnspentDB) CommitBlockTxs(changes *BlockChanges, blhash []byte) (e error) {
	// First the unwind data
	db.nosync()
	db.unspent.lastHeight = changes.Height
	db.unwind.commit(changes, blhash)
	db.unspent.commit(changes)
	if changes.Height >= changes.LastKnownHeight {
		db.sync()
	}
	return
}

func (db *UnspentDB) GetStats() (s string) {
	s += db.unspent.stats()
	s += db.unwind.stats()
	return
}


func (db *UnspentDB) SetTxNotify(fn TxNotifyFunc) {
	db.unspent.notifyTx = fn
}

// Flush all the data to files
func (db *UnspentDB) sync() {
	db.unwind.sync()
	db.unspent.sync()
}


func (db *UnspentDB) Sync() {
	db.sync()
}


func (db *UnspentDB) nosync() {
	db.unwind.nosync()
	db.unspent.nosync()
}


func (db *UnspentDB) Close() {
	db.unwind.close()
	db.unspent.close()
}


func (db *UnspentDB) Idle() bool {
	if db.unspent.idle() {
		return true
	}
	return db.unwind.idle()
}


func (db *UnspentDB) Save() {
	db.unwind.save()
	db.unspent.save()
}

func (db *UnspentDB) UndoBlockTransactions(height uint32) {
	db.nosync()
	db.unwind.undo(height, db.unspent)
	db.unspent.lastHeight = height-1
	db.sync()
}


func (db *UnspentDB) UnspentGet(po *TxPrevOut) (res *TxOut, e error) {
	return db.unspent.get(po)
}

func (db *UnspentDB) GetAllUnspent(addr []*BtcAddr, quick bool) (res AllUnspentTx) {
	return db.unspent.GetAllUnspent(addr, quick)
}


func (db *UnspentDB) ScanStealth(sa *StealthAddr, walk func([]byte,[]byte,uint32,[]byte,uint64)bool) {
	var remd, rem2, okd uint
	fmt.Println("Going through", len(db.unspent.stealthOuts), "...")
	for k, v := range db.unspent.stealthOuts {
		spend_v := db.unspent.dbN(int(v.dbidx)).Get(v.key)
		if spend_v==nil {
			delete(db.unspent.stealthOuts, k)
			remd++
		} else if sa.CheckPrefix(v.prefix[:]) {
			if walk(v.pkey[:], spend_v[0:32], binary.LittleEndian.Uint32(spend_v[32:36]),
				spend_v[48:], binary.LittleEndian.Uint64(spend_v[36:44])) {
				okd++
			} else {
				delete(db.unspent.stealthOuts, k)
				rem2++
			}
		}
	}
	if remd>0 || rem2>0 {
		fmt.Println(remd, "+", rem2, "stealth outputs have been removed")
	}
}
