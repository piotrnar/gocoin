package btc

import (
	"fmt"
	"os"
	"bytes"
	"errors"
)

type BlockPos struct {
	hash [32]byte
	fidx uint32
	fpos uint32 // We dont plan to use 4GB+ files 
}



type BlockDB struct {
	dir string
	magic [4]byte
	f *os.File
	currfileidx uint32
	
	blockIndex map[[blockMapLen]byte]*BlockPos
	
	lastvalidpos uint32 // last known file pos where a block was read from
	
	last_valid_fidx uint32
	last_valid_fpos uint32 // We dont plan to use 4GB+ files 
}


func NewBlockDB(dir string, magic [4]byte) (res *BlockDB) {
	f, e := os.Open(idx2fname(dir, 0))
	errorFatal(e, "Cannot open block database file")
	res = new(BlockDB)
	res.dir = dir
	res.magic = magic
	res.f = f
	res.blockIndex = make(map[[blockMapLen]byte]*BlockPos, BlockMapInitLen)
	return
}


func (db *BlockDB)Load(f *os.File) {
	db.currfileidx, _ = read32bit(f)
	db.lastvalidpos, _ = read32bit(f)
	if db.lastvalidpos!=0xffffffff {
		db.f, _ = os.Open(idx2fname(db.dir, db.currfileidx))
		db.f.Seek(int64(db.lastvalidpos), os.SEEK_SET)
	}

	db.blockIndex = make(map[[blockMapLen]byte]*BlockPos, BlockMapInitLen)
	for {
		v := new(BlockPos)
		_, er := f.Read(v.hash[:])
		if er != nil {
			break
		}
		v.fidx, _ = read32bit(f)
		v.fpos, _ = read32bit(f)
		db.blockIndex[NewBlockIndex(v.hash[:])] = v
	}
	println(len(db.blockIndex), "loaded into BlockDB")
}


func (db *BlockDB)Save(f *os.File) {
	write32bit(f, db.last_valid_fidx)
	write32bit(f, db.last_valid_fpos)
	println("BlockDB last valid pos:", db.last_valid_fidx, db.last_valid_fpos)
	for _, v := range db.blockIndex {
		f.Write(v.hash[:])
		write32bit(f, v.fidx)
		write32bit(f, v.fpos)
	}
	println(len(db.blockIndex), "saved in BlockDB")
}


func idx2fname(dir string, fidx uint32) string {
	if fidx == 0xffffffff {
		return "blk99999.dat"
	}
	return fmt.Sprintf("%s/blk%05d.dat", dir, fidx)
}


func (db *BlockDB)GetBlock(hash *Uint256) (res []byte, e error) {
	bp, yes := db.blockIndex[hash.BIdx()]
	if !yes {
		println("No such block in the index: ", hash.String())
		os.Exit(1)
	}

	f, e := os.Open(idx2fname(db.dir, bp.fidx))
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
	fidx := db.currfileidx
	fpos := uint32(getfilepos(db.f))
	if fpos != db.lastvalidpos {
		println("readOneBlock: file pos inconsistent", fpos, db.lastvalidpos)
		os.Exit(1)
	}

	res, e = readBlockFromFile(db.f, db.magic[:])
	if e != nil {
		db.f.Seek(int64(fpos), os.SEEK_SET) // restore the original position
		return
	}
	
	db.lastvalidpos = uint32(getfilepos(db.f))
	if db.lastvalidpos != fpos+uint32(len(res))+8 {
		println("readOneBlock: end file pos inconsistent", db.lastvalidpos, len(res))
		os.Exit(1)
	}
	
	db.last_valid_fidx = fidx
	db.last_valid_fpos = fpos
	
	return
}

func (db *BlockDB) FetchNextBlock() (bl *Block) {
	if db.f == nil {
		println("DB file not open - this shoudl never happen")
		os.Exit(1)
	}
	raw, e := db.readOneBlock()
	if e != nil {
		f, e2 := os.Open(idx2fname(db.dir, db.currfileidx+1))
		if e2 == nil {
			db.currfileidx++
			db.f.Close()
			db.f = f
			db.lastvalidpos = 0
			raw, e = db.readOneBlock()
		}
	}
	if e==nil {
		bl = new(Block)
		bl.Raw = raw
		bl.Hash = NewSha2Hash(raw[:80])
		bp := new(BlockPos)
		copy(bp.hash[:], bl.Hash.Hash[:])
		bp.fidx = db.last_valid_fidx
		bp.fpos = db.last_valid_fpos
		db.blockIndex[bl.Hash.BIdx()] = bp
	}
	return 
}

func (db *BlockDB) AddToExtraIndex(bl *Block) {
	f, e := os.OpenFile(idx2fname(db.dir, 0xffffffff), 
		os.O_RDWR|os.O_APPEND|os.O_CREATE, 0660)
	if e != nil {
		println(" *** AddToExtraIndex", e.Error())
		return
	}
	defer f.Close()
	
	pos := uint32(getfilepos(f))
	
	var b [4]byte
	put32lsb(b[:], uint32(len(bl.Raw)))
	f.Write(db.magic[:])
	f.Write(b[:])
	f.Write(bl.Raw[:])
	
	bp := new(BlockPos)
	copy(bp.hash[:], bl.Hash.Hash[:])
	bp.fidx = 0xffffffff
	bp.fpos = pos
	db.blockIndex[bl.Hash.BIdx()] = bp
}

