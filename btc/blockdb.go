package btc

import (
	"fmt"
	"os"
	"bytes"
	"errors"
)

type BlockPos struct {
	fidx uint32
	fpos uint32 // We dont plan to use 4GB+ files 
}


type BlockDB struct {
	dir string
	magic [4]byte
	f *os.File
	currfileidx uint32
	
	blockIndex map[[32]byte]BlockPos
	
	lastvalidpos uint32 // last known file pos where a block was read from
	
	blockpos BlockPos // index of block_hash -> file pos
}


func NewBlockDB(dir string, magic [4]byte) (res *BlockDB) {
	res = new(BlockDB)
	res.dir = dir
	res.magic = magic
	res.openFile(false)
	res.blockIndex = make(map[[32]byte]BlockPos, BlockMapInitLen)
	return
}


func (db *BlockDB)Load(f *os.File) {
	db.currfileidx, _ = read32bit(f)
	db.lastvalidpos, _ = read32bit(f)
	if db.lastvalidpos!=0xffffffff {
		db.openFile(false)
		db.f.Seek(int64(db.lastvalidpos), os.SEEK_SET)
	}

	db.blockIndex = make(map[[32]byte]BlockPos, BlockMapInitLen)
	var k [32]byte
	var v BlockPos
	for {
		n, _ := f.Read(k[:])
		if n!=len(k) {
			break
		}
		v.fidx, _ = read32bit(f)
		v.fpos, _ = read32bit(f)
		db.blockIndex[k] = v
	}
	println(len(db.blockIndex), "loaded into BlockDB")
}


func (db *BlockDB)Save(f *os.File) {
	write32bit(f, db.blockpos.fidx)
	write32bit(f, db.blockpos.fpos)
	println("BlockDB last valid pos:", db.blockpos.fidx, db.blockpos.fpos)
	for k, v := range db.blockIndex {
		f.Write(k[:])
		write32bit(f, v.fidx)
		write32bit(f, v.fpos)
	}
	println(len(db.blockIndex), "saved in BlockDB")
}


func (db *BlockDB)idx2fname(fidx uint32) string {
	return fmt.Sprintf("%s/blk%05d.dat", db.dir, fidx)
}


func (db *BlockDB)openFile(next bool) (er error) {
	if db.f != nil {
		db.f.Close()
		db.f = nil
		if next {
			db.currfileidx++
		}
	}
	db.f, er = os.Open(db.idx2fname(db.currfileidx))
	return 
}


func (db *BlockDB)GetBlock(hash *Uint256) (res []byte, e error) {
	bp, yes := db.blockIndex[hash.Hash]
	if !yes {
		println("No such block in the index: ", hash.String())
		os.Exit(1)
	}

	f, e := os.Open(db.idx2fname(bp.fidx))
	if e != nil {
		println("GetBlock1:", e.Error())
		os.Exit(1)
	}
	defer f.Close()

	f.Seek(int64(bp.fpos), os.SEEK_SET)

	res, e = readBlockFromFile(f, db.magic[:])
	if e != nil {
		println("GetBlock2:", e.Error())
		os.Exit(1)
	}
	
	return
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
	if le<81 || le>MAX_BLOCK_SIZE {
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
	var bp BlockPos

	bp.fidx = db.currfileidx
	bp.fpos = uint32(getfilepos(db.f))
	if bp.fpos != db.lastvalidpos {
		println("readOneBlock: file pos inconsistent", bp.fpos, db.lastvalidpos)
		os.Exit(1)
	}

	res, e = readBlockFromFile(db.f, db.magic[:])
	if e != nil {
		return
	}
	
	db.lastvalidpos = uint32(getfilepos(db.f))
	if db.lastvalidpos != bp.fpos+uint32(len(res))+8 {
		println("readOneBlock: end file pos inconsistent", db.lastvalidpos, len(res))
		os.Exit(1)
	}
	
	db.blockpos = bp
	
	return
}

func (db *BlockDB) FetchNextBlock() (bl *Block) {
	raw, e := db.readOneBlock()
	if e != nil {
		//fmt.Println("readOneBlock error:", e.Error())
		e = db.openFile(true)
		if e == nil {
			db.lastvalidpos = 0
			raw, e = db.readOneBlock()
		}
	}
	if e==nil {
		bl = new(Block)
		bl.Raw = raw
		bl.Hash = NewSha2Hash(raw[:80])
		db.blockIndex[bl.Hash.Hash] = db.blockpos
	}
	return 
}

