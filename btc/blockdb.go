package btc

import (
	"fmt"
	"os"
	"bytes"
	"errors"
)

type BlockPos struct {
	fidx uint32
	fpos int64
}


type BlockDB struct {
	dir string
	magic [4]byte
	f *os.File
	currfileidx uint32
	
	blockIndex map[[32]byte]BlockPos
}


func NewBlockDB(dir string, magic [4]byte) (res *BlockDB) {
	res = new(BlockDB)
	res.dir = dir
	res.magic = magic
	res.openFile(false)
	res.blockIndex = make(map[[32]byte]BlockPos, BlockMapInitLen)
	return
}


func (db *BlockDB)Save() {
	f, e := os.Create("blockdb_idx.bin")
	if e == nil {
		for k, v := range db.blockIndex {
			f.Write(k[:])
			write32bit(f, v.fidx)
			write32bit(f, uint32(v.fpos))
		}
		println(len(db.blockIndex), "saved in blockdb_idx.bin")
		f.Close()
	}
}


func (db *BlockDB)openFile(next bool) (er error) {
	if db.f != nil {
		db.f.Close()
		db.f = nil
		if next {
			db.currfileidx++
		}
	}
	s := fmt.Sprintf("%s/blk%05d.dat", db.dir, db.currfileidx)
	//fmt.Printf("Opening %s...\n", s)
	db.f, er = os.Open(s)
	return 
}


func (db *BlockDB)GetBlock(hash *Uint256) (res []byte, e error) {
	bp, yes := db.blockIndex[hash.Hash]
	if !yes {
		println("No such block in the index: ", hash.String())
		os.Exit(1)
	}
	if db.currfileidx != bp.fidx || db.f == nil {
		db.currfileidx = bp.fidx
		e = db.openFile(false)
		if e != nil {
			println("GetBlock1: ", e.Error())
			os.Exit(1)
		}
	}
	db.f.Seek(bp.fpos, 0)
	res, e = db.readOneBlock()
	if e != nil {
		println("GetBlock2: ", e.Error())
		os.Exit(1)
	}
	return
}


func (db *BlockDB)readOneBlock() (res []byte, e error) {
	var bp BlockPos

	bp.fidx = db.currfileidx
	bp.fpos, _ = db.f.Seek(0, 1)

	var buf [4]byte
	_, e = db.f.Read(buf[:])
	if e != nil {
		return
	}

	if !bytes.Equal(buf[:], db.magic[:]) {
		e = errors.New(fmt.Sprintf("BlockDB: Unexpected magic: %02x%02x%02x%02x", 
			buf[0], buf[1], buf[2], buf[3]))
		return
	}
	
	_, e = db.f.Read(buf[:])
	if e != nil {
		return
	}
	le := uint32(buf[3]) << 24
	le |= uint32(buf[2]) << 16
	le |= uint32(buf[1]) << 8
	le |= uint32(buf[0])
	if e != nil {
		return
	}

	if le<81 {
		e = errors.New(fmt.Sprintf("Block too short %d", le))
		return
	}

	res = make([]byte, le)
	_, e = db.f.Read(res[:])
	if e!=nil {
		res = nil
		return
	}

	db.blockIndex[Sha2Sum(res[:80])] = bp
	return
}

func (db *BlockDB)ReadNextBlock() (res []byte, e error) {
	res, e = db.readOneBlock()
	if e != nil {
		//fmt.Println("readOneBlock error:", e.Error())
		e = db.openFile(true)
		if e == nil {
			res, e = db.readOneBlock()
		}
	}
	return
}
