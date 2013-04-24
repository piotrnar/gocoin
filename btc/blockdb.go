package btc

import (
	"os"
	"fmt"
	"encoding/binary"
)


/*
	LoadBlockIndex(*Chain, func(ch *Chain, ha []byte, pa []byte, h, b, t uint32)) (error)
	BlockAdd(height uint32, bl *Block) (error)
	BlockTrusted(h []byte)
	BlockInvalid(h []byte)
	BlockGet(hash *Uint256) ([]byte, bool, error)
	Sync()
	Close()
	GetStats() (string)
*/

const (
	BLOCK_TRUSTED = 0x01
	BLOCK_INVALID = 0x02
)

/*
	blockchain.dat - contains raw blocks data, no headers, nothing
	blockchain.idx - contains records of 92 bytes (all values LSB):
		[0] - flags:
			bit(0) - "trusted" flag - this block's scripts have been verified
			bit(1) - "invalid" flag - this block's scripts have failed
		[1:3] - reserved
		[4:36]  - 256-bit block hash
		[36:68] - 256-bit block's parent hash
		[68:72] - 32-bit block height (genesis is 0)
		[72:76] - 32-bit block's timestamp
		[76:80] - 32-bit block's bits
		[80:88] - 64-bit block pos in blockchain.dat file
		[88:92] - 32-bit block lenght in bytes
*/


type oneBl struct {
	fpos uint64 // where at the block is stored in blockchain.dat
	blen uint32 // how long the block is in blockchain.dat
	
	ipos int64  // where at the record is stored in blockchain.idx
	trusted bool
}


type BlockDB struct {
	dirname string
	blockIndex map[[Uint256IdxLen]byte] *oneBl
	blockdata *os.File
	blockindx *os.File
}


func NewBlockDB(dir string) (db *BlockDB) {
	db = new(BlockDB)
	db.dirname = dir
	if db.dirname!="" && db.dirname[len(db.dirname )-1]!='/' && db.dirname[len(db.dirname )-1]!='\\' {
		db.dirname += "/"
	}
	db.blockIndex = make(map[[Uint256IdxLen]byte] *oneBl)
	os.MkdirAll(db.dirname, 0770)
	db.blockdata, _ = os.OpenFile(db.dirname+"blockchain.dat", os.O_RDWR|os.O_CREATE, 0660)
	if db.blockdata == nil {
		panic("Cannot open blockchain.dat")
	}
	db.blockindx, _ = os.OpenFile(db.dirname+"blockchain.idx", os.O_RDWR|os.O_CREATE, 0660)
	if db.blockindx == nil {
		panic("Cannot open blockchain.idx")
	}
	return
}


func (db *BlockDB) GetStats() (s string) {
	s += fmt.Sprintf("BlockDB: %d blocks\n", len(db.blockIndex))
	return
}


func hash2idx (h []byte) (idx [Uint256IdxLen]byte) {
	copy(idx[:], h[:Uint256IdxLen])
	return
}


func (db *BlockDB) BlockAdd(height uint32, bl *Block) (e error) {
	ChSta("DB.BlockAdd")
	var pos int64
	var flagz [4]byte

	pos, e = db.blockdata.Seek(0, os.SEEK_END)
	if e != nil {
		panic(e.Error())
	}
	_, e = db.blockdata.Write(bl.Raw[:])
	if e != nil {
		panic(e.Error())
	}

	ipos, _ := db.blockindx.Seek(0, os.SEEK_CUR) // at this point the file shall always be at its end
	
	if bl.Trusted {
		flagz[0] |= BLOCK_TRUSTED
	}
	db.blockindx.Write(flagz[:])
	db.blockindx.Write(bl.Hash.Hash[0:32])
	db.blockindx.Write(bl.Raw[4:36])
	binary.Write(db.blockindx, binary.LittleEndian, uint32(height))
	binary.Write(db.blockindx, binary.LittleEndian, uint32(bl.BlockTime))
	binary.Write(db.blockindx, binary.LittleEndian, uint32(bl.Bits))
	binary.Write(db.blockindx, binary.LittleEndian, uint64(pos))
	binary.Write(db.blockindx, binary.LittleEndian, uint32(len(bl.Raw[:])))

	db.blockIndex[hash2idx(bl.Hash.Hash[:])] = &oneBl{fpos:uint64(pos), 
		blen:uint32(len(bl.Raw[:])), ipos:ipos, trusted:bl.Trusted}
	ChSto("DB.BlockAdd")
	return
}



func (db *BlockDB) BlockInvalid(hash []byte) {
	idx := hash2idx(hash[:])
	cur, ok := db.blockIndex[idx]
	if !ok {
		println("BlockInvalid: no such block")
		return
	}
	println("mark", NewUint256(hash).String(), "as invalid")
	if cur.trusted {
		panic("if it is strusted - how can be invalid?")
	}
	db.setBlockFlag(cur, BLOCK_INVALID)
	delete(db.blockIndex, idx)
}


func (db *BlockDB) BlockTrusted(hash []byte) {
	idx := hash2idx(hash[:])
	cur, ok := db.blockIndex[idx]
	if !ok {
		println("BlockTrusted: no such block")
		return
	}
	if !cur.trusted {
		println("mark", NewUint256(hash).String(), "as trusted")
		db.setBlockFlag(cur, BLOCK_TRUSTED)
	}
}   

func (db *BlockDB) setBlockFlag(cur *oneBl, fl byte) {
	var b [1]byte
	cur.trusted = true
	cpos, _ := db.blockindx.Seek(0, os.SEEK_CUR) // remember our position
	db.blockindx.ReadAt(b[:], cur.ipos)
	b[0] |= fl
	db.blockindx.WriteAt(b[:], cur.ipos)
	db.blockindx.Seek(cpos, os.SEEK_SET) // restore the end posistion
}


// Flush all the data to files
func (db *BlockDB) Sync() {
	db.blockindx.Sync()
	db.blockdata.Sync()
}


func (db *BlockDB) Close() {
	db.blockindx.Close()
	db.blockdata.Close()
}


func (db *BlockDB) BlockGet(hash *Uint256) (bl []byte, trusted bool, e error) {
	ChSta("Db.BlockGet")
	rec, ok := db.blockIndex[hash2idx(hash.Hash[:])]
	if !ok {
		panic("block not in the index")
	}
	bl = make([]byte, rec.blen)
	_, e = db.blockdata.Seek(int64(rec.fpos), os.SEEK_SET)
	if e != nil {
		panic(e.Error())
	}
	_, e = db.blockdata.Read(bl[:])
	trusted = rec.trusted
	ChSto("Db.BlockGet")
	return
}


func (db *BlockDB) LoadBlockIndex(ch *Chain, walk func(ch *Chain, hash, prv []byte, h, bits, tim uint32)) (e error) {
	var b [92]byte
	validpos, _ := db.blockindx.Seek(0, os.SEEK_SET)
	for {
		_, e := db.blockindx.Read(b[:])
		if e != nil {
			break
		}
		
		if (b[0]&BLOCK_INVALID) != 0 {
			println("Block #", binary.LittleEndian.Uint32(b[68:72]), "is invalid", b[0])
			continue
		}
		
		trusted := (b[0]&BLOCK_TRUSTED) != 0
		blh := b[4:36]
		pah := b[36:68]
		height := binary.LittleEndian.Uint32(b[68:72])
		timestamp := binary.LittleEndian.Uint32(b[72:76])
		bits := binary.LittleEndian.Uint32(b[76:80])
		filepos := binary.LittleEndian.Uint64(b[80:88])
		bocklen := binary.LittleEndian.Uint32(b[88:92])

		db.blockIndex[hash2idx(blh)] = &oneBl{
			fpos: filepos,
			blen: bocklen,
			ipos: validpos,
			trusted : trusted}

		walk(ch, blh, pah, height, bits, timestamp)
		validpos += 92
	}
	db.blockindx.Seek(validpos, os.SEEK_SET)
	return
}


