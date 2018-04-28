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
	BLOCK_LENGTH  = 0x10
	BLOCK_INDEX   = 0x20

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
			bit(4) - if this bit is set, bytes [32:36] carry length of uncompressed block
			bit(5) - if this bit is set, bytes [28:32] carry data file index

		Used to be:
		[4:36]  - 256-bit block hash - DEPRECATED! (hash the header to get the value)

		[4:28] - reserved
		[28:32] - specifies which blockchain.dat file is used (if not zero, the filename is: blockchain-%08x.dat)
		[32:36] - length of uncompressed block

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
	olen uint32 // original length fo the block (before compression)

	datfileidx uint32 // use different blockchain.dat (if not zero, the filename is: blockchain-%08x.dat)

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
	MaxDataFileSize uint64
	DataFilesKeep uint32
}

type oneB2W struct {
	idx [btc.Uint256IdxLen]byte
	h [32]byte
	data []byte
	height uint32
	txcount uint32
}

type BlockDB struct {
	dirname string
	blockIndex map[[btc.Uint256IdxLen]byte] *oneBl
	blockdata *os.File
	blockindx *os.File
	mutex, disk_access sync.Mutex
	max_cached_blocks int
	cache map[[btc.Uint256IdxLen]byte] *BlckCachRec

	maxidxfilepos, maxdatfilepos int64
	maxdatfileidx uint32

	blocksToWrite chan oneB2W
	datToWrite uint64

	max_data_file_size uint64
	data_files_keep uint32
}



func NewBlockDBExt(dir string, opts *BlockDBOpts) (db *BlockDB) {
	db = new(BlockDB)
	db.dirname = dir
	if db.dirname!="" && db.dirname[len(db.dirname )-1]!='/' && db.dirname[len(db.dirname )-1]!='\\' {
		db.dirname += "/"
	}
	db.blockIndex = make(map[[btc.Uint256IdxLen]byte] *oneBl)
	os.MkdirAll(db.dirname, 0770)

	db.blockindx, _ = os.OpenFile(db.dirname+"blockchain.new", os.O_RDWR|os.O_CREATE, 0660)
	if db.blockindx == nil {
		panic("Cannot open blockchain.new")
	}
	if opts.MaxCachedBlocks > 0 {
		db.max_cached_blocks = opts.MaxCachedBlocks
		db.cache = make(map[[btc.Uint256IdxLen]byte]*BlckCachRec, db.max_cached_blocks)
	}
	db.max_data_file_size = opts.MaxDataFileSize
	db.data_files_keep = opts.DataFilesKeep

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
	for len(db.cache) >= db.max_cached_blocks {
		var oldest_t time.Time
		var oldest_k [btc.Uint256IdxLen]byte
		for k, v := range db.cache {
			if oldest_t.IsZero() || v.LastUsed.Before(oldest_t) {
				if rec := db.blockIndex[k]; rec.ipos != -1 {
					// don't expire records that have not been writen to disk yet
					oldest_t = v.LastUsed
					oldest_k = k
				}
			}
		}
		if oldest_t.IsZero() {
			break
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
	var flush bool

	db.mutex.Lock()
	idx := bl.Hash.BIdx()
	if rec, ok := db.blockIndex[idx]; !ok {
		db.blockIndex[idx] = &oneBl{ipos:-1, trusted:bl.Trusted}
		db.addToCache(bl.Hash, bl.Raw, bl)
		db.datToWrite += uint64(len(bl.Raw))
		db.blocksToWrite <- oneB2W{idx:idx, h:bl.Hash.Hash, data:bl.Raw, height:height, txcount:uint32(bl.TxCount)}
		flush = len(db.blocksToWrite)>=MAX_BLOCKS_TO_WRITE || db.datToWrite>=MAX_DATA_WRITE
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

	if flush {
		//println("Too many blocksToWrite - flush the data...")
		if !db.writeAll() {
			panic("many to write but nothing stored")
		}
		//println("flush done")
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

	rec.compressed, rec.snappied = true, true
	cbts := snappy.Encode(nil, b2w.data)
	rec.blen = uint32(len(cbts))
	rec.ipos = db.maxidxfilepos

	if db.max_data_file_size != 0 && uint64(db.maxdatfilepos) + uint64(len(cbts)) > db.max_data_file_size {
		if tmpf, _ := os.Create(db.dat_fname(db.maxdatfileidx + 1, false)); tmpf != nil {
			db.blockdata.Close()
			db.blockdata = tmpf
			db.maxdatfilepos = 0
			if db.data_files_keep != 0 && db.maxdatfileidx >= db.data_files_keep {
				os.Remove(db.dat_fname(db.maxdatfileidx - db.data_files_keep, false))
			}
			db.maxdatfileidx++
		} else {
			println("Cannot create", db.dat_fname(db.maxdatfileidx, false))
		}
	}

	rec.datfileidx = db.maxdatfileidx
	rec.fpos = uint64(db.maxdatfilepos)
	fl[0] |= BLOCK_COMPRSD|BLOCK_SNAPPED // gzip compression is deprecated
	if rec.trusted {
		fl[0] |= BLOCK_TRUSTED
	}

	//copy(fl[4:32], b2w.h[:28])
	fl[0] |= BLOCK_LENGTH | BLOCK_INDEX
	binary.LittleEndian.PutUint32(fl[32:36], uint32(len(b2w.data)))
	binary.LittleEndian.PutUint32(fl[28:32], rec.datfileidx)

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
	if cur.ipos==-1 {
		// if not written yet, then never write it
		delete(db.cache, idx)
		delete(db.blockIndex, idx)
	} else {
		// write the new flag to disk
		db.setBlockFlag(cur, BLOCK_INVALID)
	}
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


func (db *BlockDB) BlockGetInternal(hash *btc.Uint256, do_not_cache bool) (cacherec *BlckCachRec, trusted bool, e error) {
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

	var f *os.File
	// we will re-open the data file, to not spoil the writting pointer
	f, e = os.Open(db.dat_fname(rec.datfileidx, false))
	if f == nil || e != nil {
		f, e = os.Open(db.dat_fname(rec.datfileidx, true))
		if f == nil || e != nil {
			db.disk_access.Unlock()
			return
		}
	}

	_, e = f.Seek(int64(rec.fpos), os.SEEK_SET)
	if e != nil {
		f.Close()
		db.disk_access.Unlock()
		return
	}

	e = btc.ReadAll(f, bl)
	f.Close()
	db.disk_access.Unlock()

	if e != nil {
		return
	}

	if rec.compressed {
		if rec.snappied {
			bl, _ = snappy.Decode(nil, bl)
			if bl==nil {
				e = errors.New("snappy.Decode() failed")
			}
		} else {
			gz, _ := gzip.NewReader(bytes.NewReader(bl))
			bl, _ = ioutil.ReadAll(gz)
			gz.Close()
		}
	}

	if rec.olen == 0 {
		rec.olen = uint32(len(bl))
	}

	if !do_not_cache {
		db.mutex.Lock()
		cacherec = db.addToCache(hash, bl, nil)
		db.mutex.Unlock()
	} else {
		cacherec = &BlckCachRec{Data:bl}
	}

	return
}

func (db *BlockDB) BlockGetExt(hash *btc.Uint256) (cacherec *BlckCachRec, trusted bool, e error) {
	return db.BlockGetInternal(hash, false)
}

func (db *BlockDB) BlockGet(hash *btc.Uint256) (bl []byte, trusted bool, e error) {
	var rec *BlckCachRec
	rec, trusted, e = db.BlockGetInternal(hash, false)
	if rec!=nil {
		bl = rec.Data
	}
	return
}

func (db *BlockDB) BlockLength(hash *btc.Uint256, decode_if_needed bool) (length uint32, e error) {
	db.mutex.Lock()
	rec, ok := db.blockIndex[hash.BIdx()]
	if !ok {
		db.mutex.Unlock()
		e = errors.New("Block not in the index")
		return
	}
	db.mutex.Unlock()

	if rec.olen != 0 {
		length = rec.olen
		return
	}

	if !rec.compressed || !decode_if_needed {
		length = rec.blen
		return
	}

	_, _, e = db.BlockGet(hash)
	if e == nil {
		length = rec.olen
	}

	return
}


func (db *BlockDB) dat_fname(idx uint32, archive bool) string {
	dir := db.dirname
	if archive {
		dir += "oldat" + string(os.PathSeparator)
	}
	if idx == 0 {
		return dir + "blockchain.dat"
	}
	return dir + fmt.Sprintf("blockchain-%08x.dat", idx)
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
		BlockHash := btc.NewSha2Hash(b[56:136])

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
		blen := binary.LittleEndian.Uint32(b[48:52])
		ob.blen = blen
		if (b[0]&BLOCK_LENGTH) != 0 {
			blen = binary.LittleEndian.Uint32(b[32:36])
			ob.olen = blen
		}
		if (b[0]&BLOCK_INDEX) != 0 {
			ob.datfileidx = binary.LittleEndian.Uint32(b[28:32])
		}
		if blen > 0 && ob.datfileidx != 0xffffffff && ob.datfileidx > db.maxdatfileidx {
			db.maxdatfileidx = ob.datfileidx
			db.maxdatfilepos = 0
		}
		txs = binary.LittleEndian.Uint32(b[52:56])
		ob.ipos = db.maxidxfilepos

		db.blockIndex[BlockHash.BIdx()] = ob

		if int64(ob.fpos)+int64(ob.blen) > db.maxdatfilepos {
			db.maxdatfilepos = int64(ob.fpos)+int64(ob.blen)
		}

		walk(ch, BlockHash.Hash[:], b[56:136], bh, blen, txs)
		db.maxidxfilepos += 136
	}
	// In case if there was some trash at the end of data or index file, this should truncate it:
	db.blockindx.Seek(db.maxidxfilepos, os.SEEK_SET)

	db.blockdata, _ = os.OpenFile(db.dat_fname(db.maxdatfileidx, false), os.O_RDWR|os.O_CREATE, 0660)
	if db.blockdata == nil {
		panic("Cannot open blockchain.dat")
	}

	db.blockdata.Seek(db.maxdatfilepos, os.SEEK_SET)
	return
}
