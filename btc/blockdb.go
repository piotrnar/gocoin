package btc

import (
	"os"
	"fmt"
	"sync"
	"time"
	"bytes"
	"errors"
	"io/ioutil"
	"compress/gzip"
	"encoding/binary"
	"code.google.com/p/snappy-go/snappy"
)


const (
	BLOCK_TRUSTED = 0x01
	BLOCK_INVALID = 0x02
	BLOCK_COMPRSD = 0x04
	BLOCK_SNAPPED = 0x08

	MaxCachedBlocks = 500
)

/*
	blockchain.dat - contains raw blocks data, no headers, nothing
	blockchain.idx - contains records of 93 bytes (all values LSB):
		[0] - flags:
			bit(0) - "trusted" flag - this block's scripts have been verified
			bit(1) - "invalid" flag - this block's scripts have failed
			bit(2) - "compressed" flag - this block's data is compressed
			bit(3) - "snappy" flag - this block is compressed with snappy (not gzip'ed)
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
	compressed bool
	snappied bool
}

type cacheRecord struct {
	data []byte
	used time.Time
}

type BlockDB struct {
	dirname string
	blockIndex map[[Uint256IdxLen]byte] *oneBl
	blockdata *os.File
	blockindx *os.File
	mutex sync.Mutex
	cache map[[Uint256IdxLen]byte] *cacheRecord
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
	db.cache = make(map[[Uint256IdxLen]byte]*cacheRecord, MaxCachedBlocks)
	return
}


func (db *BlockDB) addToCache(h *Uint256, bl []byte) {
	if rec, ok := db.cache[h.BIdx()]; ok {
		rec.used = time.Now()
		return
	}
	if len(db.cache) >= MaxCachedBlocks {
		var oldest_t time.Time
		var oldest_k [Uint256IdxLen]byte
		for k, v := range db.cache {
			if oldest_t.IsZero() || v.used.Before(oldest_t) {
				oldest_t = v.used
				oldest_k = k
			}
		}
		delete(db.cache, oldest_k)
	}
	db.cache[h.BIdx()] = &cacheRecord{used:time.Now(), data:bl}
}


func (db *BlockDB) GetStats() (s string) {
	db.mutex.Lock()
	s += fmt.Sprintf("BlockDB: %d blocks, %d in cache\n", len(db.blockIndex), len(db.cache))
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

	flagz[0] |= BLOCK_COMPRSD|BLOCK_SNAPPED // gzip compression is deprecated
	cbts, _ := snappy.Encode(nil, bl.Raw)

	blksize := uint32(len(cbts))

	_, e = db.blockdata.Write(cbts)
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
	binary.Write(db.blockindx, binary.LittleEndian, blksize)

	db.mutex.Lock()
	db.blockIndex[bl.Hash.BIdx()] = &oneBl{fpos:uint64(pos),
		blen:blksize, ipos:ipos, trusted:bl.Trusted, compressed:true, snappied:true}
	db.addToCache(bl.Hash, bl.Raw)
	db.mutex.Unlock()
	return
}



func (db *BlockDB) BlockInvalid(hash []byte) {
	idx := NewUint256(hash).BIdx()
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
	idx := NewUint256(hash).BIdx()
	db.mutex.Lock()
	cur, ok := db.blockIndex[idx]
	if !ok {
		db.mutex.Unlock()
		println("BlockTrusted: no such block")
		return
	}
	if !cur.trusted {
		//fmt.Println("mark", NewUint256(hash).String(), "as trusted")
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
	rec, ok := db.blockIndex[hash.BIdx()]
	if !ok {
		db.mutex.Unlock()
		e = errors.New("Block not in the index")
		return
	}

	trusted = rec.trusted
	if crec, hit := db.cache[hash.BIdx()]; hit {
		bl = crec.data
		crec.used = time.Now()
		db.mutex.Unlock()
		return
	}
	db.mutex.Unlock()

	bl = make([]byte, rec.blen)

	// we will re-open the data file, to not spoil the writting pointer
	f, e := os.Open(db.dirname+"blockchain.dat")
	if e != nil {
		return
	}

	_, e = f.Seek(int64(rec.fpos), os.SEEK_SET)
	if e == nil {
		_, e = f.Read(bl[:])
	}
	f.Close()

	if rec.compressed {
		if rec.snappied {
			bl, _ = snappy.Decode(nil, bl)
		} else {
			gz, _ := gzip.NewReader(bytes.NewReader(bl))
			bl, _ = ioutil.ReadAll(gz)
			gz.Close()
		}
	}

	db.addToCache(hash, bl)

	return
}


func (db *BlockDB) LoadBlockIndex(ch *Chain, walk func(ch *Chain, hash, prv []byte, h, bits, tim, blen uint32)) (e error) {
	var b [92]byte
	var maxdatfilepos int64
	validpos, _ := db.blockindx.Seek(0, os.SEEK_SET)
	for !AbortNow {
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
		ob.compressed = (b[0]&BLOCK_COMPRSD) != 0
		ob.snappied = (b[0]&BLOCK_SNAPPED) != 0
		ob.fpos = binary.LittleEndian.Uint64(b[80:88])
		ob.blen = binary.LittleEndian.Uint32(b[88:92])
		ob.ipos = validpos


		BlockHash := b[4:36]
		db.blockIndex[NewUint256(BlockHash).BIdx()] = ob

		if int64(ob.fpos)+int64(ob.blen) > maxdatfilepos {
			maxdatfilepos = int64(ob.fpos)+int64(ob.blen)
		}

		walk(ch, BlockHash, b[36:68], binary.LittleEndian.Uint32(b[68:72]),
			binary.LittleEndian.Uint32(b[76:80]), binary.LittleEndian.Uint32(b[72:76]),
			ob.blen)
		validpos += 92
	}
	// In case if there was some trash at the end of data or index file, this should truncate it:
	db.blockindx.Seek(validpos, os.SEEK_SET)
	db.blockdata.Seek(maxdatfilepos, os.SEEK_SET)
	return
}
