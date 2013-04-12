package leveldb

import (
	"os"
//	"errors"
	"fmt"
//	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

var Testnet bool


type BtcDB struct {
	blockfile *os.File

	blidx *storage.FileStorage
	blidxdb *leveldb.DB

	mapUnspent map[[poutIdxLen]byte] *oneUnspent
	mapUnwind map[uint32] *oneUnwindSet
}

func NewDb() btc.BtcDB {
	var e error
	var db BtcDB

	var dirname string
	if Testnet {
		dirname = "testnet"
	} else {
		dirname = "bitcoin"
	}
	
	db.blidx, e = storage.OpenFile(dirname+"/blockindex")
	if e != nil {
		panic(e.Error())
	}

	db.blidxdb, e = leveldb.Open(db.blidx, &opt.Options{Flag: opt.OFCreateIfMissing})
	if e != nil {
		panic(e.Error())
	}

	db.blockfile, e = os.OpenFile(dirname+"/blockchain.dat", os.O_RDWR|os.O_APPEND|os.O_CREATE, 0660)
	if e != nil {
		panic(e.Error())
	}

	db.mapUnspent = make(map[[poutIdxLen]byte] *oneUnspent, 1000000)
	db.mapUnwind = make(map[uint32] *oneUnwindSet, 144)

	return &db
}


func (db BtcDB) StartTransaction() {
	panic("StartTransaction not implemented")
}

func (db BtcDB) CommitTransaction() {
	panic("CommitTransaction not implemented")
}

func (db BtcDB) RollbackTransaction() {
	panic("RollbackTransaction not implemented")
}


func (db BtcDB) GetStats() (s string) {
	sum := uint64(0)
	for _, v := range db.mapUnspent {
		sum += v.out.Value*uint64(v.cnt)
	}
	s += fmt.Sprintf("UNSPENT : tx_cnt=%d  tot_btc:%.8f\n", 
		len(db.mapUnspent), float64(sum)/1e8)
	
	s += fmt.Sprintf("UNWIND : blk_cnt=%d\n", len(db.mapUnwind))
	
	/*
	rows, _, er := mysql.Query(db.con, 
		"SELECT MAX(height), COUNT(*), SUM(orph), SUM(`len`) from "+blocksTable)
	if er==nil && len(rows)==1 {
		s += fmt.Sprintf("BCHAIN  : height=%d  OrphanedBlocks=%d/%d  : siz:~%dMB\n", 
			rows[0].Uint64(0), rows[0].Uint64(2), rows[0].Uint64(1), rows[0].Uint64(3)>>20)
	}
	*/

	return
}


func (db BtcDB) BlockAdd(height uint32, bl *btc.Block) (e error) {
	var pos int64
	var blen int
	var i uint32

	pos, e = db.blockfile.Seek(0, os.SEEK_END)
	if e != nil {
		panic(e.Error())
	}
	blen, e = db.blockfile.Write(bl.Raw[:])
	if e != nil {
		panic(e.Error())
	}

	var record [4+4+8+32]byte
	// bock heigt
	for i=0; i<4; i++ {
		record[i] = byte(height>>(i*8))
	}
	// bock length
	for i=0; i<4; i++ {
		record[4+i] = byte(uint32(blen)>>(i*8))
	}
	// bock position in the blockchain.dat file
	for i=0; i<8; i++ {
		record[8+i] = byte(uint64(pos)>>(i*8))
	}
	copy(record[4+4+8:], bl.GetParent().Hash[:])
	db.blidxdb.Put(bl.Hash.Hash[:], record[:], &opt.WriteOptions{})
	return
}

func (db BtcDB) Close() {
	db.blidxdb.Close()
	db.blidx.Close()
}


func (db BtcDB) BlockGet(hash *btc.Uint256) (bl []byte, e error) {
	var record []byte
	record, e = db.blidxdb.Get(hash.Hash[:], &opt.ReadOptions{})
	if e != nil {
		panic(e.Error())
	}
	var i, blen uint32
	var fpos uint64
	for i=0; i<4; i++ {
		blen |= uint32(record[4+i])<<(i*8)
	}
	for i=0; i<8; i++ {
		fpos |= uint64(record[8+i])<<(i*8)
	}
	bl = make([]byte, blen)
	_, e = db.blockfile.Seek(int64(fpos), os.SEEK_SET)
	if e != nil {
		panic(e.Error())
	}
	_, e = db.blockfile.Read(bl[:])
	return
}


func (db BtcDB) LoadBlockIndex(ch *btc.Chain, walk func(ch *btc.Chain, hash, prev []byte, height uint32)) (e error) {
	var i, h uint32
	iter := db.blidxdb.NewIterator(&opt.ReadOptions{})
	for iter.Next() {
		key := iter.Key()
		val := iter.Value()
		h = 0
		for i=0; i<4; i++ {
			h |= uint32(val[i])<<(i*8)
		}
		walk(ch, key[:], val[4+4+8:], h)
	}
	return
}


func init() {
	btc.NewDb = NewDb
}

