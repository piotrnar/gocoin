package utxo

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

var Memory_Malloc = func(le int) *[]byte {
	p := make([]byte, le)
	return &p
}

var Memory_Free = func(*[]byte) {
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
	HashMap         [256](map[UtxoKeyType]*[]byte)
	DeletedRecords  [256]int // used to decide whether to defragment the map
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

	mag2defrag      int
	mapDefragsCnt   int
	mapNoDefragsCnt int
	mapDefragsTime  time.Duration
	recDefragsCnt   int
	recDefragsTot   int
}

type NewUnspentOpts struct {
	CB              CallbackFunctions
	AbortNow        *bool
	Dir             string
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
			db.HashMap[i] = make(map[UtxoKeyType]*[]byte, 100e3)
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
		b *[]byte
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
		db.HashMap[i] = make(map[UtxoKeyType]*[]byte, int(u64)/256)
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
		rec.b = Memory_Malloc(int(le))
		if _, er = io.ReadFull(rd, *rec.b); er != nil {
			goto fatal_error
		}
		copy(rec.k[:], (*rec.b)[:])

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
		db.HashMap[i] = make(map[UtxoKeyType]*[]byte, 100e3)
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
		for _, v := range db.HashMap[_i] {
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

			btc.WriteVlen(buf, uint64(len(*v)))
			buf.Write(*v)
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
					bin := Serialize(xx, tmp[:])
					btc.WriteVlen(bu, uint64(len(*bin)))
					bu.Write(*bin)
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

func (db *UnspentDB) Defrag(recs []*[]byte) {
	for _, r := range recs {
		var ind UtxoKeyType
		copy(ind[:], *r)
		db.MapMutex[ind[0]].Lock()
		db.HashMap[ind[0]][ind] = r
		db.MapMutex[ind[0]].Unlock()
	}
	db.recDefragsCnt++
	db.recDefragsTot += len(recs)
}

// Only call it from the main thread
func (db *UnspentDB) DefragMap(force bool) {
	if db.WritingInProgress.Get() {
		return
	}
	//db.Mutex.Lock()
	sta := time.Now()
	db.MapMutex[db.mag2defrag].Lock()
	if force || (len(db.HashMap[db.mag2defrag]) > 100e3 && 2*db.DeletedRecords[db.mag2defrag] > len(db.HashMap[db.mag2defrag])) {
		new_map := make(map[UtxoKeyType]*[]byte, len(db.HashMap[db.mag2defrag]))
		for k, v := range db.HashMap[db.mag2defrag] {
			new_map[k] = v
		}
		db.HashMap[db.mag2defrag] = new_map
		db.DeletedRecords[db.mag2defrag] = 0
		db.mapDefragsCnt++
	} else {
		db.mapNoDefragsCnt++
	}
	db.MapMutex[db.mag2defrag].Unlock()
	db.mag2defrag = (db.mag2defrag + 1) % len(db.HashMap)
	db.mapDefragsTime += time.Since(sta)
	//db.Mutex.Unlock()
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
			db.DeletedRecords[ind[0]]++
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
			oldrec := NewUtxoRec(*v)
			for a := range rec.Outs {
				if rec.Outs[a] == nil {
					rec.Outs[a] = oldrec.Outs[a]
				}
			}
		}
		db.MapMutex[ind[0]].Lock()
		db.HashMap[ind[0]][ind] = Serialize(rec, nil)
		db.MapMutex[ind[0]].Unlock()
	}

	//os.Remove(fn) - it may crash while test-doing the undo, so we keep this file just in case
	db.LastBlockHeight--
	copy(db.LastBlockHash, newhash)
	db.DirtyDB.Set()
}

// Idle should be called when the main thread is idle to trigger UTXO.db saving
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
	var v *[]byte
	copy(ind[:], po.Hash[:])

	db.MapMutex[ind[0]].RLock()
	v = db.HashMap[ind[0]][ind]
	db.MapMutex[ind[0]].RUnlock()
	if v != nil {
		res = OneUtxoRec(*v, po.Vout)
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
	rec := NewUtxoRec(*v)
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
		db.HashMap[ind[0]][ind] = Serialize(rec, nil)
	} else {
		delete(db.HashMap[ind[0]], ind)
		db.DeletedRecords[ind[0]]++

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
				v := Serialize(rec, nil)
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

func (db *UnspentDB) UTXOStats() string {
	var outcnt, sum, sumcb, lele uint64
	var filesize, unspendable, unspendable_recs, unspendable_bytes uint64
	o := new(bytes.Buffer)

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
			for _, v := range db.HashMap[_i] {
				reclen := uint64(len(*v))
				atomic.AddUint64(&filesize, uint64(btc.VLenSize(reclen))+reclen)
				NewUtxoRecOwn(*v, rec, &sta_cbs)
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

	fmt.Fprintf(o, "UNSPENT: %.8f BTC in %d outs from %d txs. %.8f BTC in coinbase.\n",
		float64(sum)/1e8, outcnt, lele, float64(sumcb)/1e8)
	fmt.Fprintf(o, " MaxTxOutCnt: %d  DirtyDB: %t  Writing: %t  Abort: %t  Compressed: %t\n",
		len(rec_outs), db.DirtyDB.Get(), db.WritingInProgress.Get(), len(db.abortwritingnow) > 0,
		db.ComprssedUTXO)
	fmt.Fprintf(o, " Last Block : %s @ %d\n", btc.NewUint256(db.LastBlockHash).String(),
		db.LastBlockHeight)
	fmt.Fprintf(o, " Unspendable Outputs: %d (%dKB)  txs:%d    UTXO.db file size: %d\n",
		unspendable, unspendable_bytes>>10, unspendable_recs, filesize)

	return o.String()
}

// GetStats returns DB statistics.
func (db *UnspentDB) GetStats() (s string) {
	var hml, dels int
	for i := range db.HashMap {
		db.MapMutex[i].RLock()
		hml += len(db.HashMap[i])
		dels += db.DeletedRecords[i]
		db.MapMutex[i].RUnlock()
	}

	s = fmt.Sprintf("UNSPENT: %d txs.  MaxCnt:%d  Dirt:%t  Writ:%t  Abort:%t  Compr:%t\n",
		hml, len(rec_outs), db.DirtyDB.Get(), db.WritingInProgress.Get(),
		len(db.abortwritingnow) > 0, db.ComprssedUTXO)
	s += fmt.Sprintf(" Last Block : %s @ %d\n", btc.NewUint256(db.LastBlockHash).String(),
		db.LastBlockHeight)
	s += fmt.Sprintf(" Defrags:  Maps:%d (%d no) in %s (%d%% dels)   Records: %d recs (in %d rounds)\n",
		db.mapDefragsCnt, db.mapNoDefragsCnt, db.mapDefragsTime.String(), 100*dels/hml,
		db.recDefragsTot, db.recDefragsCnt)
	return
}

func (db *UnspentDB) PurgeUnspendable(all bool) {
	var unspendable_txs, unspendable_recs uint64
	db.Mutex.Lock()
	db.abortWriting()

	for _i := range db.HashMap {
		db.MapMutex[_i].Lock()
		for k, v := range db.HashMap[_i] {
			rec := NewUtxoRecStatic(*v)
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
				db.DeletedRecords[k[0]]++
				unspendable_txs++
			} else if record_removed > 0 {
				db.HashMap[k[0]][k] = Serialize(rec, nil)
				Memory_Free(v)
				unspendable_recs += record_removed
			}
		}
		db.MapMutex[_i].Unlock()
	}

	db.Mutex.Unlock()

	fmt.Println("Purged", unspendable_txs, "transactions and", unspendable_recs, "extra records")
}
