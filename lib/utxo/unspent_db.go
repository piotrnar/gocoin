package utxo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/lib/script"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/filter"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

var (
	UTXO_WRITING_TIME_TARGET        = 5 * time.Minute // Take it easy with flushing UTXO.db onto disk
	UTXO_SKIP_SAVE_BLOCKS    uint32 = 0
	UTXO_PURGE_UNSPENDABLE   bool   = false
	UTXO_CACHE_BLOCKS_BACK          = 6
)

var Memory_Malloc = func(le int) []byte {
	return make([]byte, le)
}

var Memory_Free = func([]byte) {
}

type FunctionWalkUnspent func(*UtxoRec)

type CallbackFunctions struct {
	// If NotifyTx is set, it will be called each time a new unspent
	// output is being added or removed. When being removed, btc.TxOut is nil.
	NotifyTxAdd func(*UtxoRec)
	NotifyTxDel func(*UtxoRec, []bool)
}

// BlockChanges is used to pass block's changes to UnspentDB.
type BlockChanges struct {
	Height          uint32
	LastKnownHeight uint32 // put here zero to disable this feature
	AddList         []*UtxoRec
	DeledTxs        map[[32]byte][]bool
	UndoData        map[[32]byte]*UtxoRec
}

type UnspentDB struct {
	HashMap  [256](map[UtxoKeyType][]byte)
	MapMutex [256]sync.RWMutex // used to access HashMap

	LastBlockHash      []byte
	LastBlockHeight    uint32
	ComprssedUTXO      bool
	dir_utxo, dir_undo string
	volatimemode       bool
	UnwindBufLen       uint32
	DirtyDB            sys.SyncBool // deprecated
	sync.Mutex

	WritingInProgress   sys.SyncBool
	CurrentHeightOnDisk uint32
	DoNotWriteUndoFiles bool
	CB                  CallbackFunctions

	undo_dir_created bool

	ldb *leveldb.DB //adding a reference to the leveldb database

	cache utxoCache
}

type NewUnspentOpts struct {
	Dir             string
	Rescan          bool
	VolatimeMode    bool
	UnwindBufferLen uint32
	CB              CallbackFunctions
	AbortNow        *bool
	CompressRecords bool
	RecordsPrealloc uint
}

type cachedBlock struct {
	recs   []UtxoKeyType
	hash   []byte
	height uint32
}

type utxoCache struct {
	utxos  map[UtxoKeyType][]byte
	blocks chan *cachedBlock
	tosave chan *cachedBlock
}

func NewUnspentDb(opts *NewUnspentOpts) (db *UnspentDB) {
	db = new(UnspentDB)
	db.dir_utxo = opts.Dir
	db.dir_undo = db.dir_utxo + "undo" + string(os.PathSeparator)
	db.volatimemode = opts.VolatimeMode
	db.UnwindBufLen = 256
	db.CB = opts.CB
	for i := range db.HashMap {
		db.HashMap[i] = make(map[UtxoKeyType][]byte)
	}
	db.cache.utxos = make(map[UtxoKeyType][]byte)
	db.cache.blocks = make(chan *cachedBlock, UTXO_CACHE_BLOCKS_BACK)
	db.cache.tosave = make(chan *cachedBlock, UTXO_CACHE_BLOCKS_BACK)

	var err error
	var dat []byte

	ldb_opts := new(opt.Options)
	//ldb_opts.Strict = opt.DefaultStrict
	//ldb_opts.Compression = opt.NoCompression
	ldb_opts.Filter = filter.NewBloomFilter(10)

	if opts.Rescan {
		goto start_from_scratch
	}
	println("opening", opts.Dir+"utxo.ldb", "...")
	ldb_opts.ErrorIfMissing = true
	db.ldb, err = leveldb.OpenFile(opts.Dir+"utxo.ldb", ldb_opts)
	if err != nil {
		fmt.Println("Failed to open LevelDB:", err.Error())
		goto start_from_scratch
	}

	db.LastBlockHash, err = db.ldb.Get([]byte("LastBlockHash"), nil)
	if err != nil {
		fmt.Println("Failed to get LastBlockHash:", err.Error())
		goto start_from_scratch
	}

	dat, err = db.ldb.Get([]byte("LastBlockHeight"), nil)
	if err != nil || (len(dat) != 8 && len(dat) != 4) {
		fmt.Println("Failed to get LastBlockHeight:", err.Error())
		goto start_from_scratch
	}
	if len(dat) == 4 {
		db.LastBlockHeight = binary.LittleEndian.Uint32(dat)

	} else {
		db.LastBlockHeight = uint32(binary.BigEndian.Uint64(dat))
	}

	dat, err = db.ldb.Get([]byte("ComprssedUTXO"), nil)
	if err == nil || len(dat) == 1 {
		db.ComprssedUTXO = dat[0] != 0
	}

	fmt.Println("LastBlockHash:", btc.NewUint256(db.LastBlockHash).String())

	goto do_compressed_check

start_from_scratch:
	if db.ldb != nil {
		db.ldb.Close()
	}
	os.RemoveAll(opts.Dir + "utxo.ldb")
	ldb_opts.ErrorIfMissing = false
	db.ldb, err = leveldb.OpenFile(opts.Dir+"utxo.ldb", ldb_opts)
	if err != nil {
		fmt.Println("Failed to open LevelDB:", err.Error())
		panic("Cannot continue")
	}
	db.LastBlockHash = nil
	db.LastBlockHeight = 0
	db.ComprssedUTXO = true
	db.ldb.Put([]byte("ComprssedUTXO"), []byte{1}, nil)

do_compressed_check:
	fmt.Println("LastBlockHeight:", db.LastBlockHeight)
	fmt.Println("ComprssedUTXO:", db.ComprssedUTXO)
	if db.ComprssedUTXO {
		NewUtxoRecOwn = NewUtxoRecOwnC
		OneUtxoRec = OneUtxoRecC
		Serialize = SerializeC
	}
	return
}

// CommitBlockTxs commits the given add/del transactions to UTXO and Unwind DBs.
func (db *UnspentDB) CommitBlockTxs(changes *BlockChanges, blhash []byte) (e error) {
	//fmt.Println("CommitBlockTxs", changes.Height)

	var wg sync.WaitGroup

	undo_fn := fmt.Sprint(db.dir_undo, changes.Height)

	db.Mutex.Lock()
	defer db.Mutex.Unlock()

	if changes.UndoData != nil {
		//fmt.Println("store undo in", undo_fn, " unwind:", db.UnwindBufLen)
		wg.Add(1)
		go func() {
			//fmt.Println(" undo...", changes.UndoData, db.undo_dir_created)
			var tmp [0x100000]byte // static record for Serialize to serialize to
			bu := new(bytes.Buffer)
			bu.Write(blhash)
			if changes.UndoData != nil {
				for _, xx := range changes.UndoData {
					bin := Serialize(xx, true, tmp[:])
					btc.WriteVlen(bu, uint64(len(bin)))
					bu.Write(bin)
				}
			}
			if !db.undo_dir_created { // (try to) create undo folder before writing the first file
				os.MkdirAll(db.dir_undo, 0770)
				db.undo_dir_created = true
			}
			os.WriteFile(db.dir_undo+"tmp", bu.Bytes(), 0666)
			os.Rename(db.dir_undo+"tmp", undo_fn)

			wg.Done()
		}()
	}

	db.commit(changes, blhash)

	if db.LastBlockHash == nil {
		db.LastBlockHash = make([]byte, 32)
	}
	copy(db.LastBlockHash, blhash)
	db.LastBlockHeight = changes.Height

	if changes.Height > db.UnwindBufLen {
		os.Remove(fmt.Sprint(db.dir_undo, changes.Height-db.UnwindBufLen))
	}

	wg.Wait()
	return
}

func (db *UnspentDB) UndoBlockTxs(bl *btc.Block, newhash []byte) {
	db.Mutex.Lock()
	defer db.Mutex.Unlock()

	db.Save()

	batch := new(leveldb.Batch)
	for _, tx := range bl.Txs {
		lst := make([]bool, len(tx.TxOut))
		for i := range lst {
			lst[i] = true
		}
		var key UtxoKeyType
		copy(key[:], tx.Hash.Hash[:])
		db.del(key, lst, batch)
	}

	fn := fmt.Sprint(db.dir_undo, db.LastBlockHeight)
	var addback []*UtxoRec

	if _, er := os.Stat(fn); er != nil {
		fn += ".tmp"
	}

	dat, er := os.ReadFile(fn)

	if er != nil {
		panic(er.Error())
	}

	off := 32 // skip the block hash

	for off < len(dat) {
		le, n := btc.VLen(dat[off:])
		off += n
		qr := FullUtxoRec(dat[off : off+le])
		off += le
		addback = append(addback, qr)
	}

	for _, rec := range addback {
		if db.CB.NotifyTxAdd != nil {
			db.CB.NotifyTxAdd(rec)
		}

		var ind UtxoKeyType
		copy(ind[:], rec.TxID[:])
		if v, _ := db.ldb.Get(ind[:], nil); v != nil {
			oldrec := NewUtxoRec(ind, v)
			for a := range rec.Outs {
				if rec.Outs[a] == nil {
					rec.Outs[a] = oldrec.Outs[a]
				}
			}
		}
		batch.Put(ind[:], Serialize(rec, false, nil))
	}

	os.Remove(fn)
	db.LastBlockHeight--
	copy(db.LastBlockHash, newhash)

	db.update_last_block(batch, db.LastBlockHash, db.LastBlockHeight)
	sta := time.Now()
	db.ldb.Write(batch, nil)
	fmt.Println("UTXO: SLOW undo block", db.LastBlockHeight+1, "done in", time.Since(sta).String())

	// TODO: We shoukd not be needing it, but only for now...
	db.cache.utxos = make(map[UtxoKeyType][]byte)
	db.cache.blocks = make(chan *cachedBlock, UTXO_CACHE_BLOCKS_BACK)
	db.cache.tosave = make(chan *cachedBlock, UTXO_CACHE_BLOCKS_BACK)
}

func (db *UnspentDB) update_last_block(batch *leveldb.Batch, hash []byte, height uint32) {
	// Write the updated LastBlockHash and LastBlockHeight to the LevelDB database
	batch.Put([]byte("LastBlockHash"), hash)
	var da [4]byte
	binary.LittleEndian.PutUint32(da[:], height)
	batch.Put([]byte("LastBlockHeight"), da[:])
}

// Idle should be called when the main thread is idle.
func (db *UnspentDB) Idle() bool {
	if len(db.cache.tosave) > 0 {
		db.saveOneBlock()
		return true
	}
	return false
}

func (db *UnspentDB) Save() {
	if len(db.cache.tosave) == 0 {
		return
	}
	fmt.Println("UTXO: saving", len(db.cache.tosave), "/", len(db.cache.blocks), "blocks")
	for len(db.cache.tosave) > 0 {
		db.saveOneBlock()
	}
	fmt.Println("Saving done")
}

func (db *UnspentDB) HurryUp() {
}

// Close flushes the data and closes all the files.
func (db *UnspentDB) Close() {
	db.Save()
	db.ldb.Close()
}

// UnspentGet gets the given unspent output.
func (db *UnspentDB) UnspentGet(po *btc.TxPrevOut) (res *btc.TxOut) {

	var ind UtxoKeyType
	copy(ind[:], po.Hash[:])
	//fmt.Println("UnspentGet called", hex.EncodeToString(ind[:]))
	if v, ok := db.cache.utxos[ind]; ok {
		if len(v) != 1 {
			res = OneUtxoRec(ind, v, po.Vout)
		}
		return
	}

	v, err := db.ldb.Get(ind[:], nil)
	if err != nil {
		//fmt.Println("UnspentGet Failed to get record:", err, hex.EncodeToString(ind[:]), po.String())
		//panic("aaa")
		return nil
	}

	if v != nil {
		res = OneUtxoRec(ind, v, po.Vout)
	} else {
		fmt.Println("Record not found")
	}

	return
}

// TxPresent returns true if gived TXID is in UTXO.
func (db *UnspentDB) TxPresent(id *btc.Uint256) (res bool) {
	var txid UtxoKeyType
	copy(txid[:], id.Hash[:])
	res, _ = db.ldb.Has(txid[:], nil)
	return
}

func (db *UnspentDB) del(k UtxoKeyType, outs []bool, batch *leveldb.Batch) {
	var v []byte
	if batch == nil {
		v = db.cache.utxos[k]
	}
	if v == nil {
		v, _ = db.ldb.Get(k[:], nil)
		if batch != nil && v == nil {
			//println("undo from non-existing record!")
			return
		}
	}
	if len(v) == 1 {
		println("deleting from deleted record")
		return
	}
	rec := NewUtxoRec(k, v)
	if db.CB.NotifyTxDel != nil {
		db.CB.NotifyTxDel(rec, outs[:]) // converting outs to a slice
	}
	var anyout bool
	for i, rm := range outs {
		if rm || UTXO_PURGE_UNSPENDABLE && rec.Outs[i] != nil && script.IsUnspendable(rec.Outs[i].PKScr) {
			rec.Outs[i] = nil
		} else if !anyout && rec.Outs[i] != nil {
			anyout = true
		}
	}
	if anyout {
		data := Serialize(rec, false, nil)
		if batch != nil {
			batch.Put(k[:], data)
		} else {
			db.cache.utxos[k] = data
		}
	} else {
		if batch != nil {
			batch.Delete(k[:])
		} else {
			db.cache.utxos[k] = []byte{1}
		}
	}
}

func (db *UnspentDB) freeOneBlock() {
	if len(db.cache.tosave) == len(db.cache.blocks) {
		db.saveOneBlock()
	}
	ks := <-db.cache.blocks
	for _, k := range ks.recs {
		delete(db.cache.utxos, k)
	}
}

func (db *UnspentDB) saveOneBlock() {
	sta := time.Now()
	batch := new(leveldb.Batch)
	ks := <-db.cache.tosave
	for _, k := range ks.recs {
		if v, ok := db.cache.utxos[k]; ok {
			if len(v) == 1 {
				batch.Delete(k[:])
			} else {
				batch.Put(k[:], v)
			}
		}
	}
	db.update_last_block(batch, ks.hash, ks.height)
	db.ldb.Write(batch, nil)
	fmt.Println("UTXO: cached block", ks.height, "saved in", time.Since(sta).String())
}

func (db *UnspentDB) commit(changes *BlockChanges, blhash []byte) {
	if len(db.cache.blocks) == UTXO_CACHE_BLOCKS_BACK {
		db.freeOneBlock()
	}
	// var wg sync.WaitGroup
	// Now apply the unspent changes
	ks := make([]UtxoKeyType, 0, len(changes.AddList)+len(changes.DeledTxs))
	for _, rec := range changes.AddList {
		var ind UtxoKeyType
		copy(ind[:], rec.TxID[:])
		if db.CB.NotifyTxAdd != nil {
			db.CB.NotifyTxAdd(rec)
		}
		var add_this_tx bool
		if UTXO_PURGE_UNSPENDABLE {
			for idx, r := range rec.Outs {
				if r != nil {
					if script.IsUnspendable(r.PKScr) {
						rec.Outs[idx] = nil
					} else {
						add_this_tx = true
					}
				}
			}
		} else {
			add_this_tx = true
		}
		if add_this_tx {
			data := Serialize(rec, false, nil)
			//db.batch.Put(ind[:], data)
			db.cache.utxos[ind] = data
			ks = append(ks, ind)
		}
	}
	for txid, v := range changes.DeledTxs {
		var k UtxoKeyType
		copy(k[:], txid[:])
		db.del(k, v, nil)
		ks = append(ks, k)
	}
	bl := &cachedBlock{recs: ks, hash: blhash, height: changes.Height}
	db.cache.blocks <- bl
	db.cache.tosave <- bl
}

func (db *UnspentDB) AbortWriting() {

}

func (db *UnspentDB) UTXOStats() (s string) {
	return ""
}

func (db *UnspentDB) GetByKey(k []byte) (v []byte) {
	v, _ = db.ldb.Get(k[:], nil)
	return
}

func (db *UnspentDB) Browse(cb func(k, v []byte) bool) {
	iter := db.ldb.NewIterator(nil, nil)
	defer iter.Release()
	for iter.Next() {
		if k := iter.Key(); len(k) == UtxoIdxLen {
			if cb(k, iter.Value()) {
				return
			}
		}
	}
}

// GetStats returns DB statistics.
func (db *UnspentDB) GetStats() (s string) {
	return fmt.Sprintf("UTXO: Cache max len: %d,  used: %d,  to save: %d\n", UTXO_CACHE_BLOCKS_BACK,
		len(db.cache.blocks), len(db.cache.tosave))
}

func (db *UnspentDB) PurgeUnspendable(all bool) {}
