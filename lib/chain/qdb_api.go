package chain

import (
	"os"
	"fmt"
	"bytes"
	"strconv"
	"io/ioutil"
	"github.com/piotrnar/gocoin/lib/btc"
)

// Used during BrowseUTXO()
const (
	WALK_ABORT   = 0x00000001 // Abort browsing
	WALK_NOMORE  = 0x00000002 // Do not browse through it anymore

	// Unspent DB
	SingeIndexSize = uint(700e3) // This should be optimal for realnet block #~300000, but not for testnet
	NumberOfUnspentSubDBs = 0x10

	UnwindBufferMaxHistory = 100  // Let's keep unwind history for so may last blocks
)

type FunctionWalkUnspent func(*QdbRec)

// Used to pass block's changes to UnspentDB
type BlockChanges struct {
	Height uint32
	LastKnownHeight uint32  // put here zero to disable this feature
	AddList []*QdbRec
	DeledTxs map[[32]byte] []bool
	UndoData *bytes.Buffer
}


type UnspentDB struct {
	LastBlockHash []byte
	LastBlockHeight uint32
	dir string
	unspent *unspentDb
}


func NewUnspentDb(dir string, init bool, ch *Chain) *UnspentDB {
	db := new(UnspentDB)
	db.dir = dir+"unspent4"+string(os.PathSeparator)

	if init {
		os.RemoveAll(db.dir)
	} else {
		fis, _ := ioutil.ReadDir(db.dir)
		var maxbl int
		for _, fi := range fis {
			if !fi.IsDir() && fi.Size()>=32 {
				cb, er := strconv.ParseUint(fi.Name(), 10, 32)
				if er == nil && int(cb) > maxbl {
					maxbl = int(cb)
				}
			}
		}
		if maxbl!=0 {
			f, _ := os.Open(fmt.Sprint(db.dir, maxbl))
			db.LastBlockHash = make([]byte, 32)
			f.Read(db.LastBlockHash)
			f.Close()
			println("max block found", maxbl, btc.NewUint256(db.LastBlockHash).String())
		}
	}

	db.unspent = newUnspentDB(db.dir, ch)


	return db
}


// Commit the given add/del transactions to UTXO and Wnwind DBs
func (db *UnspentDB) CommitBlockTxs(changes *BlockChanges, blhash []byte) (e error) {
	db.nosync()
	db.unspent.commit(changes)
	if changes.Height >= changes.LastKnownHeight {
		db.Sync()
	}
	if db.LastBlockHash==nil {
		db.LastBlockHash = make([]byte, 32)
	}
	copy(db.LastBlockHash, blhash)
	db.LastBlockHeight = changes.Height

	if changes.Height>=changes.LastKnownHeight || (changes.Height&0x3f)==0 {
		f, _ := os.Create(fmt.Sprint(db.dir, changes.Height))
		f.Write(blhash)
		if changes.UndoData != nil {
			f.Write(changes.UndoData.Bytes())
		}
		f.Close()
	}

	if changes.Height>UnwindBufferMaxHistory {
		os.Remove(fmt.Sprint(db.dir, changes.Height-UnwindBufferMaxHistory))
	}
	return
}


// Return DB statistics
func (db *UnspentDB) GetStats() (s string) {
	s += db.unspent.stats()
	return
}


// Flush all the data to files
func (db *UnspentDB) Sync() {
	db.unspent.sync()
	if db.LastBlockHash!=nil {
		fn := fmt.Sprint(db.dir, db.LastBlockHeight)
		fi, er := os.Stat(fn)
		if er!=nil || fi.Size()<32 {
			fmt.Println("Saving last block's hash")
			ioutil.WriteFile(fn, db.LastBlockHash, 0666)
		}
	}
}


// Hold on writing data to disk untill next sync is called
func (db *UnspentDB) nosync() {
	db.unspent.nosync()
}


// Flush the data and close all the files
func (db *UnspentDB) Close() {
	db.unspent.close()
}


// Call it when the main thread is idle - this will do DB defrag
func (db *UnspentDB) Idle() bool {
	return db.unspent.idle()
}


// Flush all the data to disk
func (db *UnspentDB) Save() {
	db.unspent.save()
}


// Get ne unspent output
func (db *UnspentDB) UnspentGet(po *btc.TxPrevOut) (res *btc.TxOut, e error) {
	return db.unspent.get(po)
}


// Browse through all unspent outputs
func (db *UnspentDB) BrowseUTXO(quick bool, walk FunctionWalkUnspent) {
	db.unspent.browse(walk, quick)
}
