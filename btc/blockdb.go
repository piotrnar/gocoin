package btc

import (
	"os"
	"fmt"
	"sync"
	"errors"
	"encoding/binary"
)


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
		[36:68] - 256-bit block's Parent hash
		[68:72] - 32-bit block height (genesis is 0)
		[72:76] - 32-bit block's timestamp
		[76:80] - 32-bit block's bits
		[80:88] - 64-bit block pos in blockchain.dat file
		[88:92] - 32-bit block lenght in bytes
*/


type oneBl struct {
	fpos uint64 // where at the block is stored in blockchain.dat
	blen uint32 // how long the block is in blockchain.dat

	ipos int64  // where at the record is stored in blockchain.idx (used to set flags)
	trusted bool
}


type BlockDB struct {
	dirname string
	blockIndex map[[Uint256IdxLen]byte] *oneBl
	blockdata *os.File
	blockindx *os.File
	mutex sync.Mutex
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
	db.mutex.Lock()
	s += fmt.Sprintf("BlockDB: %d blocks\n", len(db.blockIndex))
	db.mutex.Unlock()
	return
}


func hash2idx (h []byte) (idx [Uint256IdxLen]byte) {
	copy(idx[:], h[:Uint256IdxLen])
	return
}


func (db *BlockDB) BlockAdd(height uint32, bl *Block) (e error) {
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

	db.mutex.Lock()
	db.blockIndex[hash2idx(bl.Hash.Hash[:])] = &oneBl{fpos:uint64(pos),
		blen:uint32(len(bl.Raw[:])), ipos:ipos, trusted:bl.Trusted}
	db.mutex.Unlock()
	return
}



func (db *BlockDB) BlockInvalid(hash []byte) {
	idx := hash2idx(hash[:])
	db.mutex.Lock()
	cur, ok := db.blockIndex[idx]
	if !ok {
		db.mutex.Unlock()
		println("BlockInvalid: no such block")
		return
	}
	println("mark", NewUint256(hash).String(), "as invalid")
	if cur.trusted {
		panic("if it is trusted - how can be invalid?")
	}
	db.setBlockFlag(cur, BLOCK_INVALID)
	delete(db.blockIndex, idx)
	db.mutex.Unlock()
}


func (db *BlockDB) BlockTrusted(hash []byte) {
	idx := hash2idx(hash[:])
	db.mutex.Lock()
	cur, ok := db.blockIndex[idx]
	if !ok {
		db.mutex.Unlock()
		println("BlockTrusted: no such block")
		return
	}
	if !cur.trusted {
		println("mark", NewUint256(hash).String(), "as trusted")
		db.setBlockFlag(cur, BLOCK_TRUSTED)
	}
	db.mutex.Unlock()
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
	db.mutex.Lock()
	rec, ok := db.blockIndex[hash2idx(hash.Hash[:])]
	db.mutex.Unlock()
	if !ok {
		e = errors.New("Block not in the index")
		return
	}
	bl = make([]byte, rec.blen)
	db.blockdata.Seek(int64(rec.fpos), os.SEEK_SET)
	db.blockdata.Read(bl[:])
	trusted = rec.trusted
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
			// just ignore it
			continue
		}

		ob := new(oneBl)
		ob.trusted = (b[0]&BLOCK_TRUSTED) != 0
		ob.fpos = binary.LittleEndian.Uint64(b[80:88])
		ob.blen = binary.LittleEndian.Uint32(b[88:92])
		ob.ipos = validpos

		BlockHash := b[4:36]
		db.blockIndex[hash2idx(BlockHash)] = ob

		walk(ch, BlockHash, b[36:68], binary.LittleEndian.Uint32(b[68:72]),
			binary.LittleEndian.Uint32(b[76:80]), binary.LittleEndian.Uint32(b[72:76]))
		validpos += 92
	}
	// In case if there was some trash, this should truncate it:
	db.blockindx.Seek(validpos, os.SEEK_SET)
	return
}
