package utxo

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

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
	DirtyDB            sys.SyncBool
	sync.Mutex

	abortwritingnow   chan bool
	WritingInProgress sys.SyncBool
	writingDone       sync.WaitGroup
	lastFileClosed    sync.WaitGroup

	CurrentHeightOnDisk uint32
	hurryup             chan bool
	DoNotWriteUndoFiles bool
	CB                  CallbackFunctions

	undo_dir_created bool
}

type NewUnspentOpts struct {
	Dir             string
	Rescan          bool
	VolatimeMode    bool
	UnwindBufferLen uint32
	CB              CallbackFunctions
	AbortNow        *bool
	UseGoHeap       bool
}

func NewUnspentDb(opts *NewUnspentOpts) (db *UnspentDB) {
	//var maxbl_fn string
	db = new(UnspentDB)
	db.dir_utxo = opts.Dir
	db.dir_undo = db.dir_utxo + "undo" + string(os.PathSeparator)
	db.volatimemode = opts.VolatimeMode
	db.UnwindBufLen = 256
	db.CB = opts.CB
	db.abortwritingnow = make(chan bool, 1)
	db.hurryup = make(chan bool, 1)

	os.Remove(db.dir_undo + "tmp") // Remove unfinished undo file
	if files, er := filepath.Glob(db.dir_utxo + "*.db.tmp"); er == nil {
		for _, f := range files {
			os.Remove(f) // Remove unfinished *.db.tmp files
		}
	}

	if opts.Rescan {
		for i := range db.HashMap {
			db.HashMap[i] = make(map[UtxoKeyType][]byte, int(UTXO_RECORDS_PREALLOC)/256)
		}
		return
	}

	// Load data from disk
	var k UtxoKeyType
	var cnt_dwn, cnt_dwn_from, perc int
	var le uint64
	var u64, tot_recs uint64
	var info string
	var rd *bufio.Reader
	var of *os.File

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

	for tot_recs = 0; tot_recs < u64; tot_recs++ {
		if opts.AbortNow != nil && *opts.AbortNow {
			break
		}
		le, er = btc.ReadVLen(rd)
		if er != nil {
			goto fatal_error
		}

		_, er = io.ReadFull(rd, k[:])
		if er != nil {
			goto fatal_error
		}

		b := Memory_Malloc(int(le) - UtxoIdxLen)
		_, er = io.ReadFull(rd, b)
		if er != nil {
			goto fatal_error
		}

		// we don't lock RWMutex here as this code is only used during init phase, when no other routines are running
		db.HashMap[int(k[0])][k] = b

		if cnt_dwn == 0 {
			fmt.Print(info, perc, "% complete ... ")
			perc++
			cnt_dwn = cnt_dwn_from
		} else {
			cnt_dwn--
		}
	}
	of.Close()

	fmt.Print("\r                                                                 \r")

	atomic.StoreUint32(&db.CurrentHeightOnDisk, db.LastBlockHeight)
	if db.ComprssedUTXO {
		FullUtxoRec = FullUtxoRecC
		NewUtxoRecStatic = NewUtxoRecStaticC
		NewUtxoRec = NewUtxoRecC
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
		db.HashMap[i] = make(map[UtxoKeyType][]byte, int(UTXO_RECORDS_PREALLOC)/256)
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
		for k, v := range db.HashMap[_i] {
			if check_time {
				check_time = false
				data_progress = int64(current_record<<20) / int64(total_records)
				time_progress = int64(time.Now().Sub(start_time)<<20) / int64(UTXO_WRITING_TIME_TARGET)
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
		db.MapMutex[_i].RUnlock()
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
			ioutil.WriteFile(db.dir_undo+"tmp", bu.Bytes(), 0666)
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

	for _, tx := range bl.Txs {
		lst := make([]bool, len(tx.TxOut))
		for i := range lst {
			lst[i] = true
		}
		db.del(tx.Hash.Hash[:], lst)
	}

	fn := fmt.Sprint(db.dir_undo, db.LastBlockHeight)
	var addback []*UtxoRec

	if _, er := os.Stat(fn); er != nil {
		fn += ".tmp"
	}

	dat, er := ioutil.ReadFile(fn)
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

	os.Remove(fn)
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

func (db *UnspentDB) del(hash []byte, outs []bool) {
	var ind UtxoKeyType
	copy(ind[:], hash)
	db.MapMutex[ind[0]].RLock()
	v := db.HashMap[ind[0]][ind]
	db.MapMutex[ind[0]].RUnlock()
	if v == nil {
		return // no such txid in UTXO (just ignorde delete request)
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
	var wg sync.WaitGroup
	/*
		var add_lists [256][]*UtxoRec
		type one_delist struct {
			k [32]byte
			v []bool
		}
		var del_lists [256][]one_delist

		for _, rec := range changes.AddList {
			if db.CB.NotifyTxAdd != nil {
				db.CB.NotifyTxAdd(rec)
			}
			idx := int(rec.TxID[0])
			add_lists[idx] = append(add_lists[idx], rec)
		}
		for k, v := range changes.DeledTxs {
			idx := int(k[0])
			del_lists[idx] = append(del_lists[idx], one_delist{k: k, v: v})
		}

		for i := 0; i < 256; i++ {
			if len(add_lists[i]) > 0 || len(del_lists[i]) > 0 {
				wg.Add(1)
				go func(idx int) {
					for _, rec := range add_lists[idx] {
						var ind UtxoKeyType
						copy(ind[:], rec.TxID[:])
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
							db.MapMutex[idx].Lock()
							db.HashMap[ind[0]][ind] = Serialize(rec, false, nil)
							db.MapMutex[idx].Unlock()
						}
					}
					for _, r := range del_lists[idx] {
						db.del(r.k[:], r.v)
					}
					wg.Done()
				}(i)
			}
		}
		wg.Wait()
	*/
	// Now aplly the unspent changes
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
			wg.Add(1)
			go func(k UtxoKeyType, r *UtxoRec) {
				v := Serialize(r, false, nil)
				db.MapMutex[int(k[0])].Lock()
				db.HashMap[int(k[0])][k] = v
				db.MapMutex[int(k[0])].Unlock()
				wg.Done()
			}(ind, rec)
		}
	}
	for k, v := range changes.DeledTxs {
		wg.Add(1)
		go func(ke [32]byte, vv []bool) {
			db.del(ke[:], vv)
			wg.Done()
		}(k, v)
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

func (db *UnspentDB) UTXOStats() (s string) {
	var outcnt, sum, sumcb uint64
	var filesize, unspendable, unspendable_recs, unspendable_bytes uint64

	filesize = 8 + 32 + 8 // UTXO.db: block_no + block_hash + rec_cnt

	var lele int

	for _i := range db.HashMap {
		db.MapMutex[_i].RLock()
		lele += len(db.HashMap)
		for k, v := range db.HashMap[_i] {
			reclen := uint64(len(v) + UtxoIdxLen)
			filesize += uint64(btc.VLenSize(reclen))
			filesize += reclen
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
		db.MapMutex[_i].RUnlock()
	}

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
				delete(db.HashMap[int(k[0])], k)
				unspendable_txs++
			} else if record_removed > 0 {
				db.HashMap[int(k[0])][k] = Serialize(rec, false, nil)
				Memory_Free(v)
				unspendable_recs += record_removed
			}
		}
		db.MapMutex[_i].Unlock()
	}

	db.Mutex.Unlock()

	fmt.Println("Purged", unspendable_txs, "transactions and", unspendable_recs, "extra records")
}
