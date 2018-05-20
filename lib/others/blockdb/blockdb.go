/*
This package is suposed to help importin Satoshi's bitcoin
client blockchain into Gocoin's bitcoin client.
*/
package blockdb

import (
	"fmt"
	"os"
	"bytes"
	"errors"
)


type BlockDB struct {
	dir string
	magic [4]byte
	f *os.File
	currfileidx uint32
}


func NewBlockDB(dir string, magic [4]byte) (res *BlockDB) {
	f, e := os.Open(idx2fname(dir, 0))
	if e != nil {
		panic(e.Error())
	}
	res = new(BlockDB)
	res.dir = dir
	res.magic = magic
	res.f = f
	return
}


func idx2fname(dir string, fidx uint32) string {
	if fidx == 0xffffffff {
		return "blk99999.dat"
	}
	return fmt.Sprintf("%s/blk%05d.dat", dir, fidx)
}


func readBlockFromFile(f *os.File, mag []byte) (res []byte, e error) {
	var buf [4]byte
	_, e = f.Read(buf[:])
	if e != nil {
		return
	}

	if !bytes.Equal(buf[:], mag[:]) {
		e = errors.New(fmt.Sprintf("BlockDB: Unexpected magic: %02x%02x%02x%02x",
			buf[0], buf[1], buf[2], buf[3]))
		return
	}

	_, e = f.Read(buf[:])
	if e != nil {
		return
	}
	le := uint32(lsb2uint(buf[:]))
	if le<81 {
		e = errors.New(fmt.Sprintf("Incorrect block size %d", le))
		return
	}

	res = make([]byte, le)
	_, e = f.Read(res[:])
	if e!=nil {
		return
	}

	return
}


func (db *BlockDB)readOneBlock() (res []byte, e error) {
	fpos, _ := db.f.Seek(0, 1)
	res, e = readBlockFromFile(db.f, db.magic[:])
	if e != nil {
		db.f.Seek(int64(fpos), os.SEEK_SET) // restore the original position
		return
	}
	return
}

func (db *BlockDB) FetchNextBlock() (bl []byte, e error) {
	if db.f == nil {
		println("DB file not open - this should never happen")
		os.Exit(1)
	}
	bl, e = db.readOneBlock()
	if e != nil {
		f, e2 := os.Open(idx2fname(db.dir, db.currfileidx+1))
		if e2 == nil {
			db.currfileidx++
			db.f.Close()
			db.f = f
			bl, e = db.readOneBlock()
		}
	}
	return
}

func lsb2uint(lt []byte) (res uint64) {
	for i:=0; i<len(lt); i++ {
		res |= uint64(lt[i]) << uint(i*8)
	}
	return
}
