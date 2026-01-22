package utxo

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/lib/script"
)

var (
	UTXO_WRITING_TIME_TARGET        = 5 * time.Minute // Take it easy with flushing UTXO.db onto disk
	UTXO_SKIP_SAVE_BLOCKS    uint32 = 0
	UTXO_PURGE_UNSPENDABLE   bool   = false
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
	DeledTxs        map[[32]byte][]bool
	UndoData        map[[32]byte]*UtxoRec
	AddList         []*UtxoRec
	Height          uint32
	LastKnownHeight uint32 // put here zero to disable this feature
}

type UnspentDB struct {
	HashMap         [256](map[UtxoKeyType][]byte)
	CB              CallbackFunctions
	hurryup         chan bool
	abortwritingnow chan bool
	dir_utxo        string
	dir_undo        string
	LastBlockHash   []byte
	lastFileClosed  sync.WaitGroup
	writingDone     sync.WaitGroup
	MapMutex        [256]sync.RWMutex // used to access HashMap
	sync.Mutex
	DirtyDB             sys.SyncBool
	WritingInProgress   sys.SyncBool
	UnwindBufLen        uint32
	CurrentHeightOnDisk uint32
	LastBlockHeight     uint32
	volatimemode        bool
	ComprssedUTXO       bool
	DoNotWriteUndoFiles bool
	undo_dir_created    bool
}

type NewUnspentOpts struct {
	CB              CallbackFunctions
	AbortNow        *bool
	Dir             string
	RecordsPrealloc uint
	Rescan          bool
	VolatimeMode    bool
	CompressRecords bool
}

func NewUnspentDb(opts *NewUnspentOpts) (db *UnspentDB) {
	//var maxbl_fn string
	db = new(UnspentDB)
	db.dir_utxo = opts.Dir
	db.dir_undo = db.dir_utxo + "undo" + string(os.PathSeparator)
	db.volatimemode = opts.VolatimeMode
	db.UnwindBufLen = 2560
	db.CB = opts.CB
	db.abortwritingnow = make(chan bool, 1)
	db.hurryup = make(chan bool, 1)

	os.Remove(db.dir_undo + "tmp") // Remove unfinished undo file
	if files, er := filepath.Glob(db.dir_utxo + "*.db.tmp"); er == nil {
		for _, f := range files {
			os.Remove(f) // Remove unfinished *.db.tmp files
		}
	}

	db.ComprssedUTXO = opts.CompressRecords
	if opts.Rescan {
		for i := range db.HashMap {
			db.HashMap[i] = make(map[UtxoKeyType][]byte, opts.RecordsPrealloc/256)
		}
		return
	}

	// Load data from disk
	var cnt_dwn, cnt_dwn_from, perc int
	var le uint64
	var u64, tot_recs uint64
	var info string
	var rd *bufio.Reader
	var of *os.File

	const BUFFERS_CNT = 6
	const CHANNEL_SIZE = BUFFERS_CNT - 2
	const RECS_PACK_SIZE = 0x10000
	var wg sync.WaitGroup
	type one_rec struct {
		b []byte
		k UtxoKeyType
	}
	//var rec *one_rec
	var rec_idx, pool_idx int
	var recpool [BUFFERS_CNT][RECS_PACK_SIZE]one_rec
	var ch chan []one_rec
	var recs []one_rec

	fname := "UTXO.db"

redo:
	of, er := os.Open(db.dir_utxo + fname)
	if er != nil {
		goto fatal_error
	}

	rd = bufio.NewReaderSize(of, 0x40000) // read ahed buffer size

	er = binary.Read(rd, binary.LittleEndian, &u64)
	if er != nil {
		goto fatal_error
	}
	db.LastBlockHeight = uint32(u64)

	// If the highest bit of the block number is set, the UTXO records are compressed
	db.ComprssedUTXO = (u64 & 0x8000000000000000) != 0

	db.LastBlockHash = make([]byte, 32)
	_, er = rd.Read(db.LastBlockHash)
	if er != nil {
		goto fatal_error
	}
	er = binary.Read(rd, binary.LittleEndian, &u64)
	if er != nil {
		goto fatal_error
	}

	//fmt.Println("Last block height", db.LastBlockHeight, "   Number of records", u64)
	cnt_dwn_from = int(u64 / 100)
	perc = 0

	for i := range db.HashMap {
		db.HashMap[i] = make(map[UtxoKeyType][]byte, int(u64)/256)
	}
	if db.ComprssedUTXO {
		info = fmt.Sprint("\rLoading ", u64, " compressed txs from ", fname, " - ")
	} else {
		info = fmt.Sprint("\rLoading ", u64, " plain txs from ", fname, " - ")
	}

	// use background routine for map updates
	ch = make(chan []one_rec, CHANNEL_SIZE)
	recs = recpool[pool_idx][:]
	wg.Add(1)
	go func() {
		for {
			if recs := <-ch; recs == nil {
				wg.Done()
				return
			} else {
				for _, r := range recs {
					db.HashMap[r.k[0]][r.k] = r.b
				}
			}
		}
	}()

	for tot_recs = 0; tot_recs < u64; tot_recs++ {
		if opts.AbortNow != nil && *opts.AbortNow {
			break
		}
		le, er = btc.ReadVLen(rd)
		if er != nil {
			goto fatal_error
		}

		rec := &recs[rec_idx]
		if _, er = io.ReadFull(rd, rec.k[:]); er != nil {
			goto fatal_error
		}

		rec.b = Memory_Malloc(int(le) - UtxoIdxLen)
		if _, er = io.ReadFull(rd, rec.b); er != nil {
			goto fatal_error
		}

		if rec_idx == len(recs)-1 {
			ch <- recs
			rec_idx = 0
			pool_idx = (pool_idx + 1) % BUFFERS_CNT
			recs = recpool[pool_idx][:]
		} else {
			rec_idx++
		}

		if cnt_dwn == 0 {
			fmt.Print(info, perc, "% complete ... ")
			perc++
			cnt_dwn = cnt_dwn_from
		} else {
			cnt_dwn--
		}
	}
	if rec_idx > 0 {
		ch <- recs[:rec_idx]
	}
	ch <- nil
	wg.Wait()
	of.Close()

	fmt.Print("\r                                                                 \r")

	atomic.StoreUint32(&db.CurrentHeightOnDisk, db.LastBlockHeight)
	if db.ComprssedUTXO {
		NewUtxoRecOwn = NewUtxoRecOwnC
		OneUtxoRec = OneUtxoRecC
		Serialize = SerializeC
	}

	return

fatal_error:
	if of != nil {
		of.Close()
	}

	println(er.Error())
	if fname != "UTXO.old" {
		fname = "UTXO.old"
		goto redo
	}
	db.LastBlockHeight = 0
	db.LastBlockHash = nil
	for i := range db.HashMap {
		db.HashMap[i] = make(map[UtxoKeyType][]byte, opts.RecordsPrealloc/256)
	}

	return
}

func (db *UnspentDB) save() {
	//var cnt_dwn, cnt_dwn_from, perc int
	var abort, hurryup, check_time bool
	var total_records, current_record, data_progress, time_progress int64

	const save_buffer_min = 0x10000 // write in chunks of ~64KB
	const save_buffer_cnt = 100

	os.Rename(db.dir_utxo+"UTXO.db", db.dir_utxo+"UTXO.old")
	data_channel := make(chan []byte, save_buffer_cnt)
	exit_channel := make(chan bool, 1)

	start_time := time.Now()

	for _i := range db.HashMap {
		total_records += int64(len(db.HashMap[_i]))
	}

	buf := bytes.NewBuffer(make([]byte, 0, save_buffer_min+0x1000)) // add 4K extra for the last record (it will still be able to grow over it)
	u64 := uint64(db.LastBlockHeight)
	if db.ComprssedUTXO {
		u64 |= 0x8000000000000000
	}
	binary.Write(buf, binary.LittleEndian, u64)
	buf.Write(db.LastBlockHash)
	binary.Write(buf, binary.LittleEndian, uint64(total_records))

	// The data is written in a separate process
	// so we can abort without waiting for disk.
	db.lastFileClosed.Add(1)
	go func(fname string) {
		of_, er := os.Create(fname)
		if er != nil {
			println("Create file:", er.Error())
			return
		}

		of := bufio.NewWriter(of_)

		var dat []byte
		var abort, exit bool

		for !exit || len(data_channel) > 0 {
			select {

			case dat = <-data_channel:
				if len(exit_channel) > 0 {
					if abort = <-exit_channel; abort {
						goto exit
					} else {
						exit = true
					}
				}
				of.Write(dat)

			case abort = <-exit_channel:
				if abort {
					goto exit
				} else {
					exit = true
				}
			}
		}
	exit:
		if abort {
			of_.Close() // abort
			os.Remove(fname)
		} else {
			of.Flush()
			of_.Close()
			os.Rename(fname, db.dir_utxo+"UTXO.db")
		}
		db.lastFileClosed.Done()
	}(db.dir_utxo + btc.NewUint256(db.LastBlockHash).String() + ".db.tmp")

	if UTXO_WRITING_TIME_TARGET == 0 {
		hurryup = true
	}
	for _i := range db.HashMap {
		db.MapMutex[_i].RLock()
		defer db.MapMutex[_i].RUnlock()
		for k, v := range db.HashMap[_i] {
			if check_time {
				check_time = false
				data_progress = int64(current_record<<20) / int64(total_records)
				time_progress = int64(time.Since(start_time)<<20) / int64(UTXO_WRITING_TIME_TARGET)
				if data_progress > time_progress {
					select {
					case <-db.abortwritingnow:
						abort = true
						goto finito
					case <-db.hurryup:
						hurryup = true
					case <-time.After((time.Duration(data_progress-time_progress) * UTXO_WRITING_TIME_TARGET) >> 20):
					}
				}
			}

			for len(data_channel) >= cap(data_channel) {
				select {
				case <-db.abortwritingnow:
					abort = true
					goto finito
				case <-db.hurryup:
					hurryup = true
				case <-time.After(time.Millisecond):
				}
			}

			btc.WriteVlen(buf, uint64(UtxoIdxLen+len(v)))
			buf.Write(k[:])
			buf.Write(v)
			if buf.Len() >= save_buffer_min {
				data_channel <- buf.Bytes()
				if !hurryup {
					check_time = true
				}
				buf = bytes.NewBuffer(make([]byte, 0, save_buffer_min+0x1000)) // add 4K extra for the last record
			}

			current_record++
		}
	}
finito:

	if !abort && buf.Len() > 0 {
		data_channel <- buf.Bytes()
	}
	exit_channel <- abort

	if !abort {
		db.DirtyDB.Clr()
		//println("utxo written OK in", time.Now().Sub(start_time).String(), timewaits)
		atomic.StoreUint32(&db.CurrentHeightOnDisk, db.LastBlockHeight)
	}
	db.WritingInProgress.Clr()
	db.writingDone.Done()
}

// CommitBlockTxs commits the given add/del transactions to UTXO and Unwind DBs.
func (db *UnspentDB) CommitBlockTxs(changes *BlockChanges, blhash []byte) (e error) {
	var wg sync.WaitGroup

	undo_fn := fmt.Sprint(db.dir_undo, changes.Height)

	db.Mutex.Lock()
	defer db.Mutex.Unlock()
	db.abortWriting()

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
			os.WriteFile(db.dir_undo+"tmp", bu.Bytes(), 0666)
			os.Rename(db.dir_undo+"tmp", undo_fn)
			wg.Done()
		}()
	}

	db.commit(changes)

	if db.LastBlockHash == nil {
		db.LastBlockHash = make([]byte, 32)
	}
	copy(db.LastBlockHash, blhash)
	db.LastBlockHeight = changes.Height

	if changes.Height > db.UnwindBufLen {
		os.Remove(fmt.Sprint(db.dir_undo, changes.Height-db.UnwindBufLen))
	}

	db.DirtyDB.Set()
	wg.Wait()
	return
}

func (db *UnspentDB) UndoBlockTxs(bl *btc.Block, newhash []byte) {
	db.Mutex.Lock()
	defer db.Mutex.Unlock()
	db.abortWriting()

	// first we have to delete all bl.Txs from our set
	if db.CB.NotifyTxDel == nil {
		// if we don't need to notify the wallet, we can do it quicker
		for _, tx := range bl.Txs {
			var ind UtxoKeyType
			copy(ind[:], tx.Hash.Hash[:])
			db.MapMutex[ind[0]].Lock()
			delete(db.HashMap[ind[0]], ind)
			db.MapMutex[ind[0]].Unlock()
		}
	} else {
		// otherwise do it the slow way, using db.del()
		outs := make([]bool, 0, 0x10000)
		var ind UtxoKeyType
		for _, tx := range bl.Txs {
			for len(outs) < len(tx.TxOut) {
				outs = append(outs, true)
			}
			copy(ind[:], tx.Hash.Hash[:])
			db.del(ind, outs[:len(tx.TxOut)])
		}
	}

	fn := fmt.Sprint(db.dir_undo, db.LastBlockHeight)
	var addback []*UtxoRec

	dat, er := os.ReadFile(fn)
	if er != nil {
		panic(er.Error())
	}

	off := 32 // ship the block hash
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
		db.MapMutex[ind[0]].RLock()
		v := db.HashMap[ind[0]][ind]
		db.MapMutex[ind[0]].RUnlock()
		if v != nil {
			oldrec := NewUtxoRec(ind, v)
			for a := range rec.Outs {
				if rec.Outs[a] == nil {
					rec.Outs[a] = oldrec.Outs[a]
				}
			}
		}
		db.MapMutex[ind[0]].Lock()
		db.HashMap[ind[0]][ind] = Serialize(rec, false, nil)
		db.MapMutex[ind[0]].Unlock()
	}

	//os.Remove(fn) - it may crash while test-doing the undo, so we keep this file just in case
	db.LastBlockHeight--
	copy(db.LastBlockHash, newhash)
	db.DirtyDB.Set()
}

// Idle should be called when the main thread is idle.
func (db *UnspentDB) Idle() bool {
	if db.volatimemode {
		return false
	}

	db.Mutex.Lock()
	defer db.Mutex.Unlock()

	if db.DirtyDB.Get() && db.LastBlockHeight-atomic.LoadUint32(&db.CurrentHeightOnDisk) > UTXO_SKIP_SAVE_BLOCKS {
		return db.Save()
	}

	return false
}

func (db *UnspentDB) Save() bool {
	if db.WritingInProgress.Get() {
		return false
	}
	db.WritingInProgress.Set()
	db.writingDone.Add(1)
	go db.save() // this one will call db.writingDone.Done()
	return true
}

func (db *UnspentDB) HurryUp() {
	select {
	case db.hurryup <- true:
	default:
	}
}

// Close flushes the data and closes all the files.
func (db *UnspentDB) Close() {
	db.volatimemode = false
	if db.DirtyDB.Get() {
		db.HurryUp()
		db.Save()
	}
	db.writingDone.Wait()
	db.lastFileClosed.Wait()
}

// UnspentGet gets the given unspent output.
func (db *UnspentDB) UnspentGet(po *btc.TxPrevOut) (res *btc.TxOut) {
	var ind UtxoKeyType
	var v []byte
	copy(ind[:], po.Hash[:])

	db.MapMutex[ind[0]].RLock()
	v = db.HashMap[ind[0]][ind]
	db.MapMutex[ind[0]].RUnlock()
	if v != nil {
		res = OneUtxoRec(ind, v, po.Vout)
	}

	return
}

// TxPresent returns true if gived TXID is in UTXO.
func (db *UnspentDB) TxPresent(id *btc.Uint256) (res bool) {
	var ind UtxoKeyType
	copy(ind[:], id.Hash[:])
	db.MapMutex[ind[0]].RLock()
	_, res = db.HashMap[ind[0]][ind]
	db.MapMutex[ind[0]].RUnlock()
	return
}

func (db *UnspentDB) del(ind UtxoKeyType, outs []bool) {
	db.MapMutex[ind[0]].RLock()
	v := db.HashMap[ind[0]][ind]
	db.MapMutex[ind[0]].RUnlock()
	if v == nil {
		return // no such txid in UTXO (just ignore delete request)
	}
	rec := NewUtxoRec(ind, v)
	if db.CB.NotifyTxDel != nil {
		db.CB.NotifyTxDel(rec, outs)
	}
	var anyout bool
	for i, rm := range outs {
		if rm || UTXO_PURGE_UNSPENDABLE && rec.Outs[i] != nil && script.IsUnspendable(rec.Outs[i].PKScr) {
			rec.Outs[i] = nil
		} else if !anyout && rec.Outs[i] != nil {
			anyout = true
		}
	}
	db.MapMutex[ind[0]].Lock()
	if anyout {
		db.HashMap[ind[0]][ind] = Serialize(rec, false, nil)
	} else {
		delete(db.HashMap[ind[0]], ind)
	}
	db.MapMutex[ind[0]].Unlock()
	Memory_Free(v)
}

func (db *UnspentDB) commit(changes *BlockChanges) {
	const OPS_AT_ONCE = 32
	var wg sync.WaitGroup

	type one_del_rec struct {
		v []bool
		k UtxoKeyType
	}
	do_del := func(list []one_del_rec) {
		for _, rec := range list {
			db.del(rec.k, rec.v)
		}
		wg.Done()
	}

	do_add := func(list []*UtxoRec) {
		for _, rec := range list {
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
				ind := *(*UtxoKeyType)(unsafe.Pointer(&rec.TxID[0]))
				v := Serialize(rec, false, nil)
				db.MapMutex[ind[0]].Lock()
				db.HashMap[ind[0]][ind] = v
				db.MapMutex[ind[0]].Unlock()
			}
		}
		wg.Done()
	}

	if len(changes.DeledTxs) > 0 {
		thelist := make([]one_del_rec, 0, len(changes.DeledTxs))
		var offs, cnt int
		for k, v := range changes.DeledTxs {
			ind := *(*UtxoKeyType)(unsafe.Pointer(&k[0]))
			thelist = append(thelist, one_del_rec{k: ind, v: v})
			if cnt == OPS_AT_ONCE-1 {
				wg.Add(1)
				go do_del(thelist[offs : offs+OPS_AT_ONCE])
				offs += OPS_AT_ONCE
				cnt = 0
			} else {
				cnt++
			}
		}
		if cnt > 0 {
			wg.Add(1)
			go do_del(thelist[offs:])
		}
	}

	if len(changes.AddList) != 0 {
		var offs int
		for {
			wg.Add(1)
			if offs+OPS_AT_ONCE >= len(changes.AddList) {
				go do_add(changes.AddList[offs:])
				break
			} else {
				go do_add(changes.AddList[offs : offs+OPS_AT_ONCE])
				offs += OPS_AT_ONCE
			}
		}
	}

	wg.Wait()
}

func (db *UnspentDB) AbortWriting() {
	db.Mutex.Lock()
	db.abortWriting()
	db.Mutex.Unlock()
}

func (db *UnspentDB) abortWriting() {
	if db.WritingInProgress.Get() {
		db.abortwritingnow <- true
		db.writingDone.Wait()
		select {
		case <-db.abortwritingnow:
		default:
		}
	}
}

// UTXOStatsDetailed provides comprehensive statistical analysis of UTXO record sizes
// This is the enhanced version for memory optimization analysis
func (db *UnspentDB) UTXOStatsDetailed() (s string) {
	var outcnt, sum, sumcb, lele uint64
	var filesize, unspendable, unspendable_recs, unspendable_bytes uint64

	filesize = 8 + 32 + 8 // UTXO.db: block_no + block_hash + rec_cnt

	// Size distribution tracking
	type SizeStats struct {
		sync.Mutex
		histogram    map[int]uint64 // exact size -> count
		totalRecords uint64
		totalBytes   uint64
		minSize      int
		maxSize      int
		sizes        []int // for percentile calculation
	}

	sizeStats := &SizeStats{
		histogram: make(map[int]uint64),
		minSize:   int(^uint(0) >> 1), // max int
		maxSize:   0,
	}

	var wg sync.WaitGroup
	for _i := range db.HashMap {
		wg.Add(1)
		go func(_i int) {
			var (
				sta_rec  UtxoRec
				rec_outs = make([]*UtxoTxOut, MAX_OUTS_SEEN)
				rec_pool = make([]UtxoTxOut, MAX_OUTS_SEEN)
				rec_idx  int
				sta_cbs  = NewUtxoOutAllocCbs{
					OutsList: func(cnt int) (res []*UtxoTxOut) {
						if len(rec_outs) < cnt {
							println("utxo.MAX_OUTS_SEEN", len(rec_outs), "->", cnt)
							rec_outs = make([]*UtxoTxOut, cnt)
							rec_pool = make([]UtxoTxOut, cnt)
						}
						rec_idx = 0
						res = rec_outs[:cnt]
						for i := range res {
							res[i] = nil
						}
						return
					},
					OneOut: func() (res *UtxoTxOut) {
						res = &rec_pool[rec_idx]
						rec_idx++
						return
					},
				}
			)

			// Local histogram for this goroutine
			localHistogram := make(map[int]uint64)
			localSizes := make([]int, 0, len(db.HashMap[_i]))
			localMin := int(^uint(0) >> 1)
			localMax := 0

			rec := &sta_rec
			db.MapMutex[_i].RLock()
			atomic.AddUint64(&lele, uint64(len(db.HashMap[_i])))

			for k, v := range db.HashMap[_i] {
				// THIS IS THE KEY PART: len(v) is the allocation size
				recordSize := len(v)

				// Track in local histogram
				localHistogram[recordSize]++
				localSizes = append(localSizes, recordSize)
				if recordSize < localMin {
					localMin = recordSize
				}
				if recordSize > localMax {
					localMax = recordSize
				}

				reclen := uint64(len(v) + UtxoIdxLen)
				atomic.AddUint64(&filesize, uint64(btc.VLenSize(reclen))+reclen)
				NewUtxoRecOwn(k, v, rec, &sta_cbs)
				var spendable_found bool
				for _, r := range rec.Outs {
					if r != nil {
						atomic.AddUint64(&outcnt, 1)
						atomic.AddUint64(&sum, r.Value)
						if rec.Coinbase {
							atomic.AddUint64(&sumcb, r.Value)
						}
						if script.IsUnspendable(r.PKScr) {
							atomic.AddUint64(&unspendable, 1)
							atomic.AddUint64(&unspendable_bytes, uint64(8+len(r.PKScr)))
						} else {
							spendable_found = true
						}
					}
				}
				if !spendable_found {
					atomic.AddUint64(&unspendable_recs, 1)
				}
			}
			db.MapMutex[_i].RUnlock()

			// Merge local stats into global
			sizeStats.Lock()
			for size, count := range localHistogram {
				sizeStats.histogram[size] += count
			}
			sizeStats.sizes = append(sizeStats.sizes, localSizes...)
			if localMin < sizeStats.minSize {
				sizeStats.minSize = localMin
			}
			if localMax > sizeStats.maxSize {
				sizeStats.maxSize = localMax
			}
			sizeStats.Unlock()

			wg.Done()
		}(_i)
	}
	wg.Wait()

	// Calculate statistics
	sizeStats.totalRecords = uint64(len(sizeStats.sizes))
	for size, count := range sizeStats.histogram {
		sizeStats.totalBytes += uint64(size) * count
	}

	// Sort sizes for percentile calculation
	sort.Ints(sizeStats.sizes)

	// Calculate percentiles
	getPercentile := func(p float64) int {
		if len(sizeStats.sizes) == 0 {
			return 0
		}
		idx := int(float64(len(sizeStats.sizes)-1) * p)
		return sizeStats.sizes[idx]
	}

	p1 := getPercentile(0.01)
	p5 := getPercentile(0.05)
	p10 := getPercentile(0.10)
	p25 := getPercentile(0.25)
	p50 := getPercentile(0.50)
	p75 := getPercentile(0.75)
	p90 := getPercentile(0.90)
	p95 := getPercentile(0.95)
	p99 := getPercentile(0.99)

	avgSize := float64(sizeStats.totalBytes) / float64(sizeStats.totalRecords)

	// Build report
	s = fmt.Sprintf("================================================================================\n")
	s += fmt.Sprintf("UTXO DETAILED SIZE ANALYSIS\n")
	s += fmt.Sprintf("================================================================================\n\n")

	s += fmt.Sprintf("BASIC STATS:\n")
	s += fmt.Sprintf("  Total Records:     %d\n", sizeStats.totalRecords)
	s += fmt.Sprintf("  Total Bytes:       %d (%.2f MB)\n", sizeStats.totalBytes, float64(sizeStats.totalBytes)/1024/1024)
	s += fmt.Sprintf("  Average Size:      %.2f bytes\n", avgSize)
	s += fmt.Sprintf("  Min Size:          %d bytes\n", sizeStats.minSize)
	s += fmt.Sprintf("  Max Size:          %d bytes\n\n", sizeStats.maxSize)

	s += fmt.Sprintf("PERCENTILES:\n")
	s += fmt.Sprintf("  P1  (1%%):          %d bytes\n", p1)
	s += fmt.Sprintf("  P5  (5%%):          %d bytes\n", p5)
	s += fmt.Sprintf("  P10 (10%%):         %d bytes\n", p10)
	s += fmt.Sprintf("  P25 (25%%):         %d bytes\n", p25)
	s += fmt.Sprintf("  P50 (50%%, median): %d bytes\n", p50)
	s += fmt.Sprintf("  P75 (75%%):         %d bytes\n", p75)
	s += fmt.Sprintf("  P90 (90%%):         %d bytes\n", p90)
	s += fmt.Sprintf("  P95 (95%%):         %d bytes\n", p95)
	s += fmt.Sprintf("  P99 (99%%):         %d bytes\n\n", p99)

	// Find most common sizes
	type SizeCount struct {
		Size  int
		Count uint64
	}
	var sizeList []SizeCount
	for size, count := range sizeStats.histogram {
		sizeList = append(sizeList, SizeCount{Size: size, Count: count})
	}
	sort.Slice(sizeList, func(i, j int) bool {
		return sizeList[i].Count > sizeList[j].Count
	})

	s += fmt.Sprintf("TOP 30 MOST COMMON SIZES:\n")
	s += fmt.Sprintf("  Size (bytes)    Count         Percentage    Cumulative%%\n")
	s += fmt.Sprintf("  ------------    ---------     ----------    -----------\n")
	var cumulative uint64
	for i := 0; i < 30 && i < len(sizeList); i++ {
		sc := sizeList[i]
		cumulative += sc.Count
		pct := float64(sc.Count) * 100.0 / float64(sizeStats.totalRecords)
		cumPct := float64(cumulative) * 100.0 / float64(sizeStats.totalRecords)
		s += fmt.Sprintf("  %-12d    %-9d     %6.2f%%        %6.2f%%\n",
			sc.Size, sc.Count, pct, cumPct)
	}
	s += "\n"

	// Size ranges for optimization planning
	s += fmt.Sprintf("SIZE RANGE DISTRIBUTION:\n")
	ranges := []struct {
		min, max int
		name     string
	}{
		{0, 32, "0-32 bytes"},
		{33, 64, "33-64 bytes"},
		{65, 80, "65-80 bytes"},
		{81, 96, "81-96 bytes"},
		{97, 112, "97-112 bytes"},
		{113, 128, "113-128 bytes"},
		{129, 144, "129-144 bytes"},
		{145, 160, "145-160 bytes"},
		{161, 192, "161-192 bytes"},
		{193, 256, "193-256 bytes"},
		{257, 512, "257-512 bytes"},
		{513, 1024, "513-1024 bytes"},
		{1025, 2048, "1025-2048 bytes"},
		{2049, 999999, "2049+ bytes"},
	}

	for _, r := range ranges {
		var count uint64
		var bytes uint64
		for size, cnt := range sizeStats.histogram {
			if size >= r.min && size <= r.max {
				count += cnt
				bytes += cnt * uint64(size)
			}
		}
		if count > 0 {
			pct := float64(count) * 100.0 / float64(sizeStats.totalRecords)
			avgInRange := float64(bytes) / float64(count)
			s += fmt.Sprintf("  %-20s: %9d records (%6.2f%%)  avg: %.1f bytes\n",
				r.name, count, pct, avgInRange)
		}
	}
	s += "\n"

	// Memory allocation overhead analysis
	s += fmt.Sprintf("MEMORY ALLOCATION ANALYSIS (Power-of-2 allocator):\n")
	var totalWaste uint64
	nextPowerOf2 := func(n int) int {
		if n <= 0 {
			return 0
		}
		// Round up to next power of 2
		p := 1
		for p < n {
			p *= 2
		}
		return p
	}

	for size, count := range sizeStats.histogram {
		allocated := nextPowerOf2(size)
		waste := allocated - size
		totalWaste += uint64(waste) * count
	}

	actualMemory := sizeStats.totalBytes
	allocatedMemory := actualMemory + totalWaste
	wastePercent := float64(totalWaste) * 100.0 / float64(allocatedMemory)

	s += fmt.Sprintf("  Actual data size:         %d bytes (%.2f MB)\n", actualMemory, float64(actualMemory)/1024/1024)
	s += fmt.Sprintf("  Allocated (power-of-2):   %d bytes (%.2f MB)\n", allocatedMemory, float64(allocatedMemory)/1024/1024)
	s += fmt.Sprintf("  Internal fragmentation:   %d bytes (%.2f MB)\n", totalWaste, float64(totalWaste)/1024/1024)
	s += fmt.Sprintf("  Waste percentage:         %.2f%%\n\n", wastePercent)

	// Original stats
	s += fmt.Sprintf("================================================================================\n")
	s += fmt.Sprintf("ORIGINAL UTXO STATS:\n")
	s += fmt.Sprintf("================================================================================\n")
	s += fmt.Sprintf("UNSPENT: %.8f BTC in %d outs from %d txs. %.8f BTC in coinbase.\n",
		float64(sum)/1e8, outcnt, lele, float64(sumcb)/1e8)
	s += fmt.Sprintf(" MaxTxOutCnt: %d  DirtyDB: %t  Writing: %t  Abort: %t  Compressed: %t\n",
		len(rec_outs), db.DirtyDB.Get(), db.WritingInProgress.Get(), len(db.abortwritingnow) > 0,
		db.ComprssedUTXO)
	s += fmt.Sprintf(" Last Block : %s @ %d\n", btc.NewUint256(db.LastBlockHash).String(),
		db.LastBlockHeight)
	s += fmt.Sprintf(" Unspendable Outputs: %d (%dKB)  txs:%d    UTXO.db file size: %d\n",
		unspendable, unspendable_bytes>>10, unspendable_recs, filesize)

	return
}

func (db *UnspentDB) UTXOStats() (s string) {
	var outcnt, sum, sumcb, lele uint64
	var filesize, unspendable, unspendable_recs, unspendable_bytes uint64

	filesize = 8 + 32 + 8 // UTXO.db: block_no + block_hash + rec_cnt

	var wg sync.WaitGroup
	for _i := range db.HashMap {
		wg.Add(1)
		go func(_i int) {
			var (
				sta_rec  UtxoRec
				rec_outs = make([]*UtxoTxOut, MAX_OUTS_SEEN)
				rec_pool = make([]UtxoTxOut, MAX_OUTS_SEEN)
				rec_idx  int
				sta_cbs  = NewUtxoOutAllocCbs{
					OutsList: func(cnt int) (res []*UtxoTxOut) {
						if len(rec_outs) < cnt {
							println("utxo.MAX_OUTS_SEEN", len(rec_outs), "->", cnt)
							rec_outs = make([]*UtxoTxOut, cnt)
							rec_pool = make([]UtxoTxOut, cnt)
						}
						rec_idx = 0
						res = rec_outs[:cnt]
						for i := range res {
							res[i] = nil
						}
						return
					},
					OneOut: func() (res *UtxoTxOut) {
						res = &rec_pool[rec_idx]
						rec_idx++
						return
					},
				}
			)
			rec := &sta_rec
			db.MapMutex[_i].RLock()
			atomic.AddUint64(&lele, uint64(len(db.HashMap[_i])))
			for k, v := range db.HashMap[_i] {
				reclen := uint64(len(v) + UtxoIdxLen)
				atomic.AddUint64(&filesize, uint64(btc.VLenSize(reclen))+reclen)
				NewUtxoRecOwn(k, v, rec, &sta_cbs)
				var spendable_found bool
				for _, r := range rec.Outs {
					if r != nil {
						atomic.AddUint64(&outcnt, 1)
						atomic.AddUint64(&sum, r.Value)
						if rec.Coinbase {
							atomic.AddUint64(&sumcb, r.Value)
						}
						if script.IsUnspendable(r.PKScr) {
							atomic.AddUint64(&unspendable, 1)
							atomic.AddUint64(&unspendable_bytes, uint64(8+len(r.PKScr)))
						} else {
							spendable_found = true
						}
					}
				}
				if !spendable_found {
					atomic.AddUint64(&unspendable_recs, 1)
				}
			}
			db.MapMutex[_i].RUnlock()
			wg.Done()
		}(_i)
	}
	wg.Wait()

	s = fmt.Sprintf("UNSPENT: %.8f BTC in %d outs from %d txs. %.8f BTC in coinbase.\n",
		float64(sum)/1e8, outcnt, lele, float64(sumcb)/1e8)
	s += fmt.Sprintf(" MaxTxOutCnt: %d  DirtyDB: %t  Writing: %t  Abort: %t  Compressed: %t\n",
		len(rec_outs), db.DirtyDB.Get(), db.WritingInProgress.Get(), len(db.abortwritingnow) > 0,
		db.ComprssedUTXO)
	s += fmt.Sprintf(" Last Block : %s @ %d\n", btc.NewUint256(db.LastBlockHash).String(),
		db.LastBlockHeight)
	s += fmt.Sprintf(" Unspendable Outputs: %d (%dKB)  txs:%d    UTXO.db file size: %d\n",
		unspendable, unspendable_bytes>>10, unspendable_recs, filesize)

	return
}

// GetStats returns DB statistics.
func (db *UnspentDB) GetStats() (s string) {
	var hml int
	for i := range db.HashMap {
		db.MapMutex[i].RLock()
		hml += len(db.HashMap[i])
		db.MapMutex[i].RUnlock()
	}

	s = fmt.Sprintf("UNSPENT: %d txs.  MaxCnt:%d  Dirt:%t  Writ:%t  Abort:%t  Compr:%t\n",
		hml, len(rec_outs), db.DirtyDB.Get(), db.WritingInProgress.Get(),
		len(db.abortwritingnow) > 0, db.ComprssedUTXO)
	s += fmt.Sprintf(" Last Block : %s @ %d\n", btc.NewUint256(db.LastBlockHash).String(),
		db.LastBlockHeight)
	return
}

func (db *UnspentDB) PurgeUnspendable(all bool) {
	var unspendable_txs, unspendable_recs uint64
	db.Mutex.Lock()
	db.abortWriting()

	for _i := range db.HashMap {
		db.MapMutex[_i].Lock()
		for k, v := range db.HashMap[_i] {
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
				delete(db.HashMap[k[0]], k)
				unspendable_txs++
			} else if record_removed > 0 {
				db.HashMap[k[0]][k] = Serialize(rec, false, nil)
				Memory_Free(v)
				unspendable_recs += record_removed
			}
		}
		db.MapMutex[_i].Unlock()
	}

	db.Mutex.Unlock()

	fmt.Println("Purged", unspendable_txs, "transactions and", unspendable_recs, "extra records")
}
