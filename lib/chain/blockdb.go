package chain

import (
	"os"
	"fmt"
	"sync"
	"time"
	"bufio"
	"bytes"
	"errors"
	"io/ioutil"
	"compress/gzip"
	"encoding/binary"
	"github.com/golang/snappy"
	"github.com/piotrnar/gocoin/lib/btc"
)


const (
	BLOCK_TRUSTED = 0x01
	BLOCK_INVALID = 0x02
	BLOCK_COMPRSD = 0x04
	BLOCK_SNAPPED = 0x08

	MAX_BLOCKS_TO_WRITE = 1024 // flush the data to disk when exceeding
	MAX_DATA_WRITE = 16*1024*1024
)

/*
	blockchain.dat - contains raw blocks data, no headers, nothing
	blockchain.new - contains records of 136 bytes (all values LSB):
		[0] - flags:
			bit(0) - "trusted" flag - this block's scripts have been verified
			bit(1) - "invalid" flag - this block's scripts have failed
			bit(2) - "compressed" flag - this block's data is compressed
			bit(3) - "snappy" flag - this block is compressed with snappy (not gzip'ed)
		[4:36]  - 256-bit block hash
		[36:40] - 32-bit block height (genesis is 0)
		[40:48] - 64-bit block pos in blockchain.dat file
		[48:52] - 32-bit block lenght in bytes
		[52:56] - 32-bit number of transaction in the block
		[56:136] - 80 bytes blocks header
*/


type oneBl struct {
	fpos uint64 // where at the block is stored in blockchain.dat
	ipos int64  // where at the record is stored in blockchain.idx (used to set flags) / -1 if not stored in the file (yet)
	blen uint32 // how long the block is in blockchain.dat
	trusted bool
	compressed bool
	snappied bool
}

type BlckCachRec struct {
	Data []byte
	*btc.Block

	// This is for BIP152
	BIP152 []byte // 8 bytes of nonce || 8 bytes of K0 LSB || 8 bytes of K1 LSB

	LastUsed time.Time
}

type BlockDBOpts struct {
	MaxCachedBlocks int
}

type oneB2W struct {
	idx [btc.Uint256IdxLen]byte
	h [32]byte
	data []byte
	height uint32
	txcount uint32
}

type BlockDB struct {
	DoNotCache bool // use it while rescanning

	dirname string
	blockIndex map[[btc.Uint256IdxLen]byte] *oneBl
	blockdata *os.File
	blockindx *os.File
	mutex, disk_access sync.Mutex
	max_cached_blocks int
	cache map[[btc.Uint256IdxLen]byte] *BlckCachRec

	maxidxfilepos, maxdatfilepos int64

	blocksToWrite chan oneB2W
	datToWrite uint64
}


func NewBlockDBExt(dir string, opts *BlockDBOpts) (db *BlockDB) {
	db = new(BlockDB)
	db.dirname = dir
	if db.dirname!="" && db.dirname[len(db.dirname )-1]!='/' && db.dirname[len(db.dirname )-1]!='\\' {
		db.dirname += "/"
	}
	db.blockIndex = make(map[[btc.Uint256IdxLen]byte] *oneBl)
	os.MkdirAll(db.dirname, 0770)
	db.blockdata, _ = os.OpenFile(db.dirname+"blockchain.dat", os.O_RDWR|os.O_CREATE, 0660)
	if db.blockdata == nil {
		panic("Cannot open blockchain.dat")
	}

	db.blockindx, _ = os.OpenFile(db.dirname+"blockchain.new", os.O_RDWR|os.O_CREATE, 0660)
	if db.blockindx == nil {
		panic("Cannot open blockchain.new")
	}
	if opts.MaxCachedBlocks>0 {
		db.max_cached_blocks = opts.MaxCachedBlocks
		db.cache = make(map[[btc.Uint256IdxLen]byte]*BlckCachRec, db.max_cached_blocks)
	}

	db.blocksToWrite = make(chan oneB2W, MAX_BLOCKS_TO_WRITE)
	return
}


func NewBlockDB(dir string) (db *BlockDB) {
	return NewBlockDBExt(dir, &BlockDBOpts{MaxCachedBlocks:500})
}


// Make sure to call with the mutex locked
func (db *BlockDB) addToCache(h *btc.Uint256, bl []byte, str *btc.Block) (crec *BlckCachRec) {
	if db.cache==nil {
		return
	}
	crec = db.cache[h.BIdx()]
	if crec!=nil {
		crec.Data = bl
		if str!=nil {
			crec.Block = str
		}
		crec.LastUsed = time.Now()
		return
	}
	if len(db.cache) >= db.max_cached_blocks {
		var oldest_t time.Time
		var oldest_k [btc.Uint256IdxLen]byte
		for k, v := range db.cache {
			if oldest_t.IsZero() || v.LastUsed.Before(oldest_t) {
				oldest_t = v.LastUsed
				oldest_k = k
			}
		}
		if rec := db.blockIndex[oldest_k]; rec!=nil && rec.ipos==-1 {
			// Oldest cache record not yet written - keep it
			//println("BlockDB: Oldest cache record not yet written - keep it")
		} else {
			delete(db.cache, oldest_k)
		}
	}
	crec = &BlckCachRec{LastUsed:time.Now(), Data:bl, Block:str}
	db.cache[h.BIdx()] = crec
	return
}


func (db *BlockDB) GetStats() (s string) {
	db.mutex.Lock()
	s += fmt.Sprintf("BlockDB: %d blocks, %d/%d in cache.  ToWriteCnt:%d (%dKB)\n",
		len(db.blockIndex), len(db.cache), db.max_cached_blocks, len(db.blocksToWrite), db.datToWrite>>10)
	db.mutex.Unlock()
	return
}


func hash2idx (h []byte) (idx [btc.Uint256IdxLen]byte) {
	copy(idx[:], h[:btc.Uint256IdxLen])
	return
}


func (db *BlockDB) BlockAdd(height uint32, bl *btc.Block) (e error) {
	var trust_it bool
	var flush_now bool
	db.mutex.Lock()
	idx := bl.Hash.BIdx()
	if rec, ok := db.blockIndex[idx]; !ok {
		db.blockIndex[idx] = &oneBl{ipos:-1, trusted:bl.Trusted}
		db.addToCache(bl.Hash, bl.Raw, bl)
		db.datToWrite += uint64(len(bl.Raw))
		db.blocksToWrite <- oneB2W{idx:idx, h:bl.Hash.Hash, data:bl.Raw, height:height, txcount:uint32(bl.TxCount)}
		flush_now = len(db.blocksToWrite)>=MAX_BLOCKS_TO_WRITE || db.datToWrite>=MAX_DATA_WRITE
	} else {
		//println("Block", bl.Hash.String(), "already in", rec.trusted, bl.Trusted)
		if !rec.trusted && bl.Trusted {
			//println(" ... but now it's getting trusted")
			if rec.ipos==-1 {
				// It's not saved yet - just change the flag
				rec.trusted = true
			} else {
				trust_it = true
			}
		}
	}
	db.mutex.Unlock()

	if trust_it {
		//println(" ... in the slow mode")
		db.BlockTrusted(bl.Hash.Hash[:])
	}

	if flush_now {
		//println("Too many blocksToWrite - flush the data...")
		if !db.writeAll() {
			panic("many to write but nothing stored")
		}
	}

	return
}

func (db *BlockDB) writeAll() (sync bool) {
	//sta := time.Now()
	for db.writeOne() {
		sync = true
	}
	if sync {
		db.blockdata.Sync()
		db.blockindx.Sync()
		//println("Block(s) saved in", time.Now().Sub(sta).String())
	}
	return
}

func (db *BlockDB) writeOne() (written bool) {
	var fl [136]byte
	var rec *oneBl
	var b2w oneB2W
	var e error

	select {
		case b2w = <- db.blocksToWrite:

		default:
			return
	}

	db.mutex.Lock()
	db.datToWrite -= uint64(len(b2w.data))
	rec = db.blockIndex[b2w.idx]
	db.mutex.Unlock()

	if rec==nil || rec.ipos!=-1 {
		println("Block not in the index anymore - discard")
		written = true
		return
	}

	db.disk_access.Lock()

	rec.fpos = uint64(db.maxdatfilepos)
	fl[0] |= BLOCK_COMPRSD|BLOCK_SNAPPED // gzip compression is deprecated
	if rec.trusted {
		fl[0] |= BLOCK_TRUSTED
	}

	rec.compressed, rec.snappied = true, true
	cbts := snappy.Encode(nil, b2w.data)
	rec.blen = uint32(len(cbts))
	rec.ipos = db.maxidxfilepos

	copy(fl[4:36], b2w.h[:])
	binary.LittleEndian.PutUint32(fl[36:40], uint32(b2w.height))
	binary.LittleEndian.PutUint64(fl[40:48], uint64(rec.fpos))
	binary.LittleEndian.PutUint32(fl[48:52], uint32(rec.blen))
	binary.LittleEndian.PutUint32(fl[52:56], uint32(b2w.txcount))
	copy(fl[56:136], b2w.data[:80])

	if _, e = db.blockdata.Write(cbts); e != nil {
		panic(e.Error())
	}

	if _, e = db.blockindx.Write(fl[:]); e != nil {
		panic(e.Error())
	}

	db.maxidxfilepos += 136
	db.maxdatfilepos += int64(rec.blen)

	db.disk_access.Unlock()

	written = true

	return
}


func (db *BlockDB) BlockInvalid(hash []byte) {
	idx := btc.NewUint256(hash).BIdx()
	db.mutex.Lock()
	cur, ok := db.blockIndex[idx]
	if !ok {
		db.mutex.Unlock()
		println("BlockInvalid: no such block", btc.NewUint256(hash).String())
		return
	}
	if cur.trusted {
		println("Looks like your UTXO database is corrupt")
		println("To rebuild it, remove folder: "+db.dirname+"unspent4")
		panic("Trusted block cannot be invalid")
	}
	//println("mark", btc.NewUint256(hash).String(), "as invalid")
	db.setBlockFlag(cur, BLOCK_INVALID)
	delete(db.blockIndex, idx)
	db.mutex.Unlock()
}


func (db *BlockDB) BlockTrusted(hash []byte) {
	idx := btc.NewUint256(hash).BIdx()
	db.mutex.Lock()
	cur, ok := db.blockIndex[idx]
	if !ok {
		db.mutex.Unlock()
		println("BlockTrusted: no such block")
		return
	}
	if !cur.trusted {
		//fmt.Println("mark", btc.NewUint256(hash).String(), "as trusted")
		db.setBlockFlag(cur, BLOCK_TRUSTED)
	}
	db.mutex.Unlock()
}

func (db *BlockDB) setBlockFlag(cur *oneBl, fl byte) {
	var b [1]byte
	cur.trusted = true
	db.disk_access.Lock()
	cpos, _ := db.blockindx.Seek(0, os.SEEK_CUR) // remember our position
	db.blockindx.ReadAt(b[:], cur.ipos)
	b[0] |= fl
	db.blockindx.WriteAt(b[:], cur.ipos)
	db.blockindx.Seek(cpos, os.SEEK_SET) // restore the end posistion
	db.disk_access.Unlock()
}


func (db *BlockDB) Idle() {
	if db.writeAll() {
		//println(" * block(s) stored from idle")
	}
}


func (db *BlockDB) Close() {
	if db.writeAll() {
		//println(" * block(s) stored from close")
	}
	db.blockdata.Close()
	db.blockindx.Close()
}


func (db *BlockDB) BlockGetExt(hash *btc.Uint256) (cacherec *BlckCachRec, trusted bool, e error) {
	db.mutex.Lock()
	rec, ok := db.blockIndex[hash.BIdx()]
	if !ok {
		db.mutex.Unlock()
		e = errors.New("Block not in the index")
		return
	}

	trusted = rec.trusted
	if db.cache!=nil {
		if crec, hit := db.cache[hash.BIdx()]; hit {
			cacherec = crec
			crec.LastUsed = time.Now()
			db.mutex.Unlock()
			return
		}
	}
	db.mutex.Unlock()

	if rec.ipos==-1 {
		e = errors.New("Block not written yet and not in the cache")
		return
	}

	if rec.blen==0 {
		e = errors.New("Block purged from disk")
		return
	}

	bl := make([]byte, rec.blen)

	db.disk_access.Lock()
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
	db.disk_access.Unlock()

	if rec.compressed {
		if rec.snappied {
			bl, _ = snappy.Decode(nil, bl)
		} else {
			gz, _ := gzip.NewReader(bytes.NewReader(bl))
			bl, _ = ioutil.ReadAll(gz)
			gz.Close()
		}
	}

	if !db.DoNotCache {
		db.mutex.Lock()
		cacherec = db.addToCache(hash, bl, nil)
		db.mutex.Unlock()
	} else {
		cacherec = &BlckCachRec{Data:bl}
	}

	return
}


func (db *BlockDB) BlockGet(hash *btc.Uint256) (bl []byte, trusted bool, e error) {
	var rec *BlckCachRec
	rec, trusted, e = db.BlockGetExt(hash)
	if rec!=nil {
		bl = rec.Data
	}
	return
}


func (db *BlockDB) LoadBlockIndex(ch *Chain, walk func(ch *Chain, hash, hdr []byte, height, blen, txs uint32)) (e error) {
	var b [136]byte
	var bh, txs uint32
	db.blockindx.Seek(0, os.SEEK_SET)
	db.maxidxfilepos = 0
	rd := bufio.NewReader(db.blockindx)
	for !AbortNow {
		if e := btc.ReadAll(rd, b[:]); e != nil {
			break
		}

		bh = binary.LittleEndian.Uint32(b[36:40])
		BlockHash := btc.NewUint256(b[4:36])

		if (b[0]&BLOCK_INVALID) != 0 {
			// just ignore it
			fmt.Println("BlockDB: Block", binary.LittleEndian.Uint32(b[36:40]), BlockHash.String(), "is invalid")
			continue
		}

		ob := new(oneBl)
		ob.trusted = (b[0]&BLOCK_TRUSTED) != 0
		ob.compressed = (b[0]&BLOCK_COMPRSD) != 0
		ob.snappied = (b[0]&BLOCK_SNAPPED) != 0
		ob.fpos = binary.LittleEndian.Uint64(b[40:48])
		ob.blen = binary.LittleEndian.Uint32(b[48:52])
		txs = binary.LittleEndian.Uint32(b[52:56])
		ob.ipos = db.maxidxfilepos

		db.blockIndex[BlockHash.BIdx()] = ob

		if int64(ob.fpos)+int64(ob.blen) > db.maxdatfilepos {
			db.maxdatfilepos = int64(ob.fpos)+int64(ob.blen)
		}

		walk(ch, b[4:36], b[56:136], bh, ob.blen, txs)
		db.maxidxfilepos += 136
	}
	// In case if there was some trash at the end of data or index file, this should truncate it:
	db.blockindx.Seek(db.maxidxfilepos, os.SEEK_SET)
	db.blockdata.Seek(db.maxdatfilepos, os.SEEK_SET)
	return
}
