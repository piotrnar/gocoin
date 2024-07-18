package utxo

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
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
	UTXO_CACHE_BLOCKS_BACK          = 512
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
	LastBlockHash      []byte
	LastBlockHeight    uint32
	ComprssedUTXO      bool
	dir_utxo, dir_undo string
	volatimemode       bool
	UnwindBufLen       uint32
	sync.Mutex
	abortwritingnow     bool
	WritingInProgress   sys.SyncBool
	writingDone         sync.WaitGroup
	CurrentHeightOnDisk uint32
	DoNotWriteUndoFiles bool
	CB                  CallbackFunctions

	hurryup          chan bool
	undo_dir_created bool

	LDB   *leveldb.DB //adding a reference to the leveldb database
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

// cachedBlock represents a cached block of UTXO records.
type cachedBlock struct {
	recs   []UtxoKeyType
	hash   []byte
	height uint32
}

// utxoCache caches UTXOs for better performance.
type utxoCache struct {
	utxos  map[UtxoKeyType][]byte
	blocks chan *cachedBlock
	tosave chan *cachedBlock
}

// NewUnspentDb initializes a new instance of UnspentDB based on the provided options.
func NewUnspentDb(opts *NewUnspentOpts) (db *UnspentDB) {
	db = new(UnspentDB)
	db.dir_utxo = opts.Dir
	db.dir_undo = db.dir_utxo + "undo" + string(os.PathSeparator)
	db.volatimemode = opts.VolatimeMode
	db.UnwindBufLen = 2560
	db.CB = opts.CB
	db.cache.utxos = make(map[UtxoKeyType][]byte)
	db.cache.blocks = make(chan *cachedBlock, UTXO_CACHE_BLOCKS_BACK)
	db.cache.tosave = make(chan *cachedBlock, UTXO_CACHE_BLOCKS_BACK)

	var err error
	var dat []byte

	ldb_opts := new(opt.Options)
	ldb_opts.Filter = filter.NewBloomFilter(10)

	if opts.Rescan {
		goto start_from_scratch
	}
	// Try opening the existing LevelDB database
	println("opening", opts.Dir+"utxo.ldb", "...")
	ldb_opts.ErrorIfMissing = true
	db.LDB, err = leveldb.OpenFile(opts.Dir+"utxo.ldb", ldb_opts)
	if err != nil {
		// If opening fails, start from scratch
		fmt.Println("Failed to open LevelDB:", err.Error())
		goto start_from_scratch
	}

	// Retrieve LastBlockHash from LevelDB
	db.LastBlockHash, err = db.LDB.Get([]byte("LastBlockHash"), nil)
	if err != nil {
		fmt.Println("Failed to get LastBlockHash:", err.Error())
		goto start_from_scratch
	}

	// Retrieve LastBlockHeight from LevelDB
	dat, err = db.LDB.Get([]byte("LastBlockHeight"), nil)
	if err != nil || (len(dat) != 8 && len(dat) != 4) {
		fmt.Println("Failed to get LastBlockHeight:", err.Error())
		goto start_from_scratch
	}
	if len(dat) == 4 {
		db.LastBlockHeight = binary.LittleEndian.Uint32(dat)

	} else {
		db.LastBlockHeight = uint32(binary.BigEndian.Uint64(dat))
	}

	// Check if ComprssedUTXO flag is set in LevelDB
	dat, err = db.LDB.Get([]byte("ComprssedUTXO"), nil)
	if err == nil || len(dat) == 1 {
		db.ComprssedUTXO = dat[0] != 0
	}

	fmt.Println("LastBlockHash:", btc.NewUint256(db.LastBlockHash).String())

	goto do_compressed_check

start_from_scratch:
	// Clean up and recreate LevelDB from scratch
	if db.LDB != nil {
		db.LDB.Close()
	}
	os.RemoveAll(opts.Dir + "utxo.ldb")
	ldb_opts.ErrorIfMissing = false
	db.LDB, err = leveldb.OpenFile(opts.Dir+"utxo.ldb", ldb_opts)
	if err != nil {
		fmt.Println("Failed to open LevelDB:", err.Error())
		panic("Cannot continue")
	}
	db.LastBlockHash = nil
	db.LastBlockHeight = 0
	db.ComprssedUTXO = true
	db.LDB.Put([]byte("ComprssedUTXO"), []byte{1}, nil)

do_compressed_check:
	fmt.Println("LastBlockHeight:", db.LastBlockHeight)
	fmt.Println("ComprssedUTXO:", db.ComprssedUTXO)
	// If ComprssedUTXO is enabled, use compressed UTXO functions
	if db.ComprssedUTXO {
		FullUtxoRec = FullUtxoRecC
		NewUtxoRecStatic = NewUtxoRecStaticC
		NewUtxoRec = NewUtxoRecC
		OneUtxoRec = OneUtxoRecC
		Serialize = SerializeC
	}
	return
}

// CommitBlockTxs commits block transactions changes to the UnspentDB.
func (db *UnspentDB) CommitBlockTxs(changes *BlockChanges, blhash []byte) (e error) {
	var wg sync.WaitGroup

	undo_fn := fmt.Sprint(db.dir_undo, changes.Height)

	db.Mutex.Lock()
	defer db.Mutex.Unlock()

	if changes.UndoData != nil {
		wg.Add(1)
		go func() {
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
			os.WriteFile(db.dir_undo+"tmp", bu.Bytes(), 0666) // Write serialized undo data to a temporary file
			os.Rename(db.dir_undo+"tmp", undo_fn)             //Rename the temporary file to the final

			wg.Done()
		}()
	}

	// Apply block changes to the database
	db.commit(changes, blhash)

	if db.LastBlockHash == nil {
		db.LastBlockHash = make([]byte, 32)
	}
	copy(db.LastBlockHash, blhash)
	db.LastBlockHeight = changes.Height

	// Remove old undo files if the height exceeds db.UnwindBufLen
	if changes.Height > db.UnwindBufLen {
		os.Remove(fmt.Sprint(db.dir_undo, changes.Height-db.UnwindBufLen))
	}

	wg.Wait()
	return
}

// Undo block transactions from the UnspentDB for a given block.
func (db *UnspentDB) UndoBlockTxs(bl *btc.Block, newhash []byte) {
	db.Mutex.Lock()
	defer db.Mutex.Unlock()

	// Save any pending blocks before undoing transactions
	db.Save()
	db.cache.utxos = make(map[UtxoKeyType][]byte)
	db.cache.blocks = make(chan *cachedBlock, UTXO_CACHE_BLOCKS_BACK)

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

	// Channel to handle the UtxoRec items to be processed
	recChan := make(chan *UtxoRec, len(addback))
	// Channel to collect batch operations
	resultChan := make(chan *UtxoRec, len(addback))

	// Spawn 8 goroutines to process records in parallel
	for i := 0; i < 8; i++ {
		go func() {
			for rec := range recChan {
				var ind UtxoKeyType
				copy(ind[:], rec.TxID[:])
				if v, _ := db.LDB.Get(ind[:], nil); v != nil {
					if len(v) >= 24 {
						oldrec := NewUtxoRec(ind, v)
						for a := range rec.Outs {
							if rec.Outs[a] == nil {
								rec.Outs[a] = oldrec.Outs[a]
							}
						}
					}
				}

				// Send the processed record to resultChan
				resultChan <- rec
			}
		}()
	}

	// Send all addback records to the recChan
	for _, rec := range addback {
		if db.CB.NotifyTxAdd != nil {
			db.CB.NotifyTxAdd(rec)
		}
		recChan <- rec
	}

	// Close the recChan as we have sent all records
	close(recChan)

	// Collect all processed records from resultChan and apply them in order
	for i := 0; i < len(addback); i++ {
		rec := <-resultChan
		var ind UtxoKeyType
		copy(ind[:], rec.TxID[:])
		serializedRec := Serialize(rec, false, nil)
		batch.Put(ind[:], serializedRec)
		db.cache.utxos[ind] = serializedRec // Update the cache with recent UTXOs
	}

	close(resultChan)

	// Update cache.blocks and cache.tosave
	cb := &cachedBlock{
		recs:   make([]UtxoKeyType, len(addback)),
		hash:   db.LastBlockHash,
		height: db.LastBlockHeight,
	}
	for i, rec := range addback {
		var ind UtxoKeyType
		copy(ind[:], rec.TxID[:])
		cb.recs[i] = ind
	}

	db.cache.blocks <- cb
	db.cache.tosave <- cb

	os.Remove(fn)
	db.LastBlockHeight--
	copy(db.LastBlockHash, newhash)

	db.update_last_block(batch, db.LastBlockHash, db.LastBlockHeight)
	sta := time.Now()
	db.LDB.Write(batch, nil)
	fmt.Println("UTXO: undo block", db.LastBlockHeight+1, "done in", time.Since(sta).String())
}

// updates the LastBlockHash and LastBlockHeight in the LevelDB batch.
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
		return len(db.cache.tosave) > 0
	}
	return false
}

// Saves all pending blocks from the cache to disk.
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

// Close flushes the data and closes all the files.
func (db *UnspentDB) Close() {
	db.Save()
	db.LDB.Close()
}

// UnspentGet gets the given unspent output.
func (db *UnspentDB) UnspentGet(po *btc.TxPrevOut) (res *btc.TxOut) {

	var ind UtxoKeyType
	copy(ind[:], po.Hash[:])

	if v, ok := db.cache.utxos[ind]; ok {
		if len(v) != 1 {
			res = OneUtxoRec(ind, v, po.Vout)
		}
		return
	}

	v, err := db.LDB.Get(ind[:], nil)
	if err != nil {
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
	res, _ = db.LDB.Has(txid[:], nil)
	return
}

// del deletes specified outputs from the UnspentDB and handles deletion from both the in-memory cache (if batch is nil) and the LevelDB database.
func (db *UnspentDB) del(k UtxoKeyType, outs []bool, batch *leveldb.Batch) {
	var v []byte

	if batch == nil {
		v = db.cache.utxos[k]
	}
	// If no value found in cache or if batch is not nil, retrieve from LevelDB
	if v == nil {
		v, _ = db.LDB.Get(k[:], nil)
		if batch != nil && v == nil {
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
	var anyout bool // Flag to track if there are any remaining outputs

	// Iterate through each output to determine if it should be deleted or preserved
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

// frees up memory by removing transaction outputs from the cache for one block.
func (db *UnspentDB) freeOneBlock() {
	// If all blocks to save are pending in the cache, save one block to disk
	if len(db.cache.tosave) == len(db.cache.blocks) {
		db.saveOneBlock()
	}
	ks := <-db.cache.blocks
	for _, k := range ks.recs {
		delete(db.cache.utxos, k)
	}
}

// saveOneBlock saves transaction outputs from the cache to the LevelDB database.
func (db *UnspentDB) saveOneBlock() {
	numProcess := 8 // Number of parallel process
	var wg sync.WaitGroup
	batchCh := make(chan *leveldb.Batch, numProcess)

	// Start processes
	for i := 0; i < numProcess; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for batch := range batchCh {
				db.LDB.Write(batch, nil)
			}
		}()
	}

	// Create and send batches to the channel
	ks := <-db.cache.tosave
	for i := 0; i < len(ks.recs); i += numProcess {
		batch := new(leveldb.Batch)
		for j := 0; j < numProcess && i+j < len(ks.recs); j++ {
			k := ks.recs[i+j]
			if v, ok := db.cache.utxos[k]; ok {
				if len(v) == 1 {
					batch.Delete(k[:])
				} else {
					batch.Put(k[:], v)
				}
			}
		}
		db.update_last_block(batch, ks.hash, ks.height)
		batchCh <- batch
	}

	close(batchCh)
	wg.Wait()
}

// commit writes changes to the UnspentDB and manages caching.
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
			// Check each output for spendability
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
			db.cache.utxos[ind] = data // Store serialized transaction in cache
			ks = append(ks, ind)
		}
	}

	// Process deleted transactions
	for txid, v := range changes.DeledTxs {
		var k UtxoKeyType
		copy(k[:], txid[:])
		db.del(k, v, nil)
		ks = append(ks, k)
	}

	// Create cached block record and store it
	bl := &cachedBlock{recs: ks, hash: blhash, height: changes.Height}
	db.cache.blocks <- bl
	db.cache.tosave <- bl
}

// signals the UnspentDB to speed up processing
func (db *UnspentDB) HurryUp() {
	select {
	case db.hurryup <- true:
	default:
	}
}

// Signals to abort ongoing database writes and waits for completion.
func (db *UnspentDB) AbortWriting() {
	db.Mutex.Lock()
	defer db.Mutex.Unlock()
	if db.WritingInProgress.Get() {
		db.abortwritingnow = true // Signal to abort writing

		db.writingDone.Wait() // Wait for ongoing writes to complete

		// Clear the abort signal
		db.abortwritingnow = false
	}
}

// calculates and returns statistics about the UnspentDB.
func (db *UnspentDB) UTXOStats() (s string) {
	var outcnt, sum, sumcb uint64
	var filesize, unspendable, unspendable_recs, unspendable_bytes uint64
	var lele int

	db.Mutex.Lock()
	defer db.Mutex.Unlock()

	// Iterate through all records in the database
	iter := db.LDB.NewIterator(nil, nil)
	for iter.Next() {
		lele++
		var k UtxoKeyType
		key := iter.Key()
		copy(k[:], key[:])
		v := iter.Value()

		// Calculate record size and file size
		reclen := uint64(len(v) + UtxoIdxLen)
		filesize += uint64(btc.VLenSize(reclen))
		filesize += reclen

		// Parse UTXO record and calculate statistics
		rec := NewUtxoRecStatic(k, v)
		var spendable_found bool
		for _, r := range rec.Outs {
			if r != nil {
				outcnt++
				sum += r.Value
				if rec.Coinbase {
					sumcb += r.Value
				}
				if script.IsUnspendable(r.PKScr) {
					unspendable++
					unspendable_bytes += uint64(8 + len(r.PKScr))
				} else {
					spendable_found = true
				}
			}
		}
		if !spendable_found {
			unspendable_recs++
		}

	}
	iter.Release()
	if err := iter.Error(); err != nil {
		log.Fatal(err)
	}

	// Format statistics into a string
	s = fmt.Sprintf("UNSPENT: %.8f BTC in %d outs from %d txs. %.8f BTC in coinbase.\n",
		float64(sum)/1e8, outcnt, lele, float64(sumcb)/1e8)
	s += fmt.Sprintf(" MaxTxOutCnt: %d    Writing: %t  Abort: %t  Compressed: %t\n",
		len(rec_outs), db.WritingInProgress.Get(), db.abortwritingnow, db.ComprssedUTXO)
	s += fmt.Sprintf(" Last Block : %s @ %d\n", btc.NewUint256(db.LastBlockHash).String(),
		db.LastBlockHeight)
	s += fmt.Sprintf(" Unspendable Outputs: %d (%dKB)  txs:%d    UTXO.ldb file size: %d\n",
		unspendable, unspendable_bytes>>10, unspendable_recs, filesize)
	return
}

// GetStats returns DB statistics.
func (db *UnspentDB) GetStats() (s string) {
	return fmt.Sprintf("UTXO: Cache max len: %d,  used: %d,  to save: %d\n", UTXO_CACHE_BLOCKS_BACK,
		len(db.cache.blocks), len(db.cache.tosave))
}

// PurgeUnspendable removes unspendable transactions and optionally unspendable outputs from the database.
func (db *UnspentDB) PurgeUnspendable(all bool) {
	var unspendable_txs, unspendable_recs uint64
	db.Mutex.Lock()
	defer db.Mutex.Unlock()
	db.AbortWriting()

	// Iterate through all records in the database
	iter := db.LDB.NewIterator(nil, nil)
	for iter.Next() {
		var k UtxoKeyType
		key := iter.Key()
		copy(k[:], key[:])
		v := iter.Value()

		rec := NewUtxoRecStatic(k, v)
		var spendable_found bool
		var record_removed uint64
		for idx, r := range rec.Outs {
			if r != nil {
				if script.IsUnspendable(r.PKScr) {
					if all {
						rec.Outs[idx] = nil
						record_removed++
					}
				} else {
					spendable_found = true
				}
			}
		}
		if !spendable_found {
			Memory_Free(v)
			db.LDB.Delete(key, nil) // delete the record from the database
			unspendable_txs++
		} else if record_removed > 0 {
			newValue := Serialize(rec, false, nil) // serialize the updated record
			Memory_Free(v)
			db.LDB.Put(key, newValue, nil) // update the record in the database
			unspendable_recs += record_removed
		}
	}
	iter.Release()
	if err := iter.Error(); err != nil {
		log.Fatal(err)
	}

	fmt.Println("Purged", unspendable_txs, "transactions and", unspendable_recs, "extra records")
}
