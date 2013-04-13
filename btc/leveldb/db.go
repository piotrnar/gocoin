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

var dirname string

type BtcDB struct {

	blockfile *os.File

	blockstorage *storage.FileStorage
	blockdbase *leveldb.DB
}

func NewDb() btc.BtcDB {
	var e error
	var db BtcDB

	if Testnet {
		dirname = "testnet"
	} else {
		dirname = "bitcoin"
	}
	dirname = os.Getenv("HOME")+"/"+dirname
	
	db.blockstorage, e = storage.OpenFile(dirname+"/blockindex")
	if e != nil {
		panic(e.Error())
	}

	db.blockdbase, e = leveldb.Open(db.blockstorage, &opt.Options{Flag: opt.OFCreateIfMissing})
	if e != nil {
		panic(e.Error())
	}

	db.blockfile, e = os.OpenFile(dirname+"/blockchain.dat", os.O_RDWR|os.O_APPEND|os.O_CREATE, 0660)
	if e != nil {
		panic(e.Error())
	}

	unspentOpen()

	return &db
}


func (db BtcDB) GetStats() (s string) {
	cnt := uint64(0)
	sum := uint64(0)
	it := unspentdbase.NewIterator(&opt.ReadOptions{})
	for it.Next() {
		v := it.Value()
		for i:=0; i<8; i++ {
			sum += uint64(v[i])<<(uint(i)*8)
		}
		cnt++
	}
	return fmt.Sprintf("UNSPENT: %.8f BTC in %d outputs\n", float64(sum)/1e8, cnt)
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
	db.blockdbase.Put(bl.Hash.Hash[:], record[:], &opt.WriteOptions{})
	return
}

func (db BtcDB) Close() {
	unspentClose()
	db.blockdbase.Close()
	db.blockdbase.Close()
	db.blockstorage.Close()
}


func (db BtcDB) BlockGet(hash *btc.Uint256) (bl []byte, e error) {
	var record []byte
	record, e = db.blockdbase.Get(hash.Hash[:], &opt.ReadOptions{})
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
	iter := db.blockdbase.NewIterator(&opt.ReadOptions{})
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

