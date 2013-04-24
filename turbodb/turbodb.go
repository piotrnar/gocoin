package turbodb

import (
	"os"
//	"encoding/hex"
//	"errors"
)


const KeySize = 4


type TurboDB struct {
	pathname string
	
	Cache map[[KeySize]byte] []byte
	file_index int // can be only 0 or 1
	version_seq uint32
	
	logfile *os.File
}


// Opens a new database
func NewDB(dir string) (db *TurboDB, e error) {
	db = new(TurboDB)
	if len(dir)>0 && dir[len(dir)-1]!='\\' && dir[len(dir)-1]!='/' {
		dir += "/"
	}
	e = os.MkdirAll(dir, 0770)
	db.pathname = dir+"turbodb."
	return
}


func (db *TurboDB) Load() {
	if db.Cache == nil {
		db.loadfiledat()
		db.loadfilelog()
	}
}


func (db *TurboDB) Count() int {
	db.Load()
	return len(db.Cache)
}


func (db *TurboDB) FetchAll(walk func(k, v []byte) bool) {
	db.Load()
	for k, v := range db.Cache {
		if !walk(k[:], v) {
			break
		}
	}
}


func (db *TurboDB) Get(key [KeySize]byte) (val []byte, e error) {
	db.Load()
	val, _ = db.Cache[key]
	return
}


func (db *TurboDB) Put(key [KeySize]byte, val []byte) (e error) {
	//println("put", hex.EncodeToString(key[:]))
	db.addtolog(key[:], val)
	if db.Cache != nil {
		db.Cache[key] = val
	}
	return
}


func (db *TurboDB) Del(key [KeySize]byte) (e error) {
	//println("del", hex.EncodeToString(key[:]))
	db.deltolog(key[:])
	if db.Cache != nil {
		delete(db.Cache, key)
	}
	return
}


func (db *TurboDB) Defrag() (e error) {
	db.Load()
	if db.logfile != nil {
		db.logfile.Close()
		db.logfile = nil
	}
	e = db.savefiledat()
	return
}


func (db *TurboDB) Close() {
	if db.logfile != nil {
		db.logfile.Close()
		db.logfile = nil
	}
	db.Cache = nil
}
