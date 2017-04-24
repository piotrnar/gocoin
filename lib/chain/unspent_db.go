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
	"sync/atomic"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
)


const (
	UTXO_RECORDS_PREALLOC = 25e6
	UTXO_WRITING_TIME_TARGET = 5*time.Minute  // Take it easy with flushing UTXO.db onto disk
)

type FunctionWalkUnspent func(*QdbRec)

// Used to pass block's changes to UnspentDB
type BlockChanges struct {
	Height uint32
	LastKnownHeight uint32  // put here zero to disable this feature
	AddList []*QdbRec
	DeledTxs map[[32]byte] []bool
	UndoData map[[32]byte] *QdbRec
}


type UnspentDB struct {
	HashMap map[UtxoKeyType][]byte
	LastBlockHash[]byte
	LastBlockHeight uint32
	dir_utxo, dir_undo string
	ch *Chain
	volatimemode bool
	UnwindBufLen uint32
	DirtyDB bool
	sync.Mutex
	WritingInProgress bool
	AbortWriting bool
	CurrentHeightOnDisk uint32
	HurryUp bool

	WritingProgress int64
}

type NewUnspentOpts struct {
	Dir string
	Chain *Chain
	Rescan bool
	VolatimeMode bool
	UnwindBufferLen uint32
}

func NewUnspentDb(opts *NewUnspentOpts) (db *UnspentDB) {
	//var maxbl_fn string
	db = new(UnspentDB)
	db.dir_utxo = opts.Dir
	db.dir_undo = db.dir_utxo + "undo"+string(os.PathSeparator)
	db.volatimemode = opts.VolatimeMode
	db.UnwindBufLen = 256

	os.MkdirAll(db.dir_undo, 0770)

	os.Remove(db.dir_undo+"tmp")
	os.Remove(db.dir_utxo+"UTXO.db.tmp")
	db.ch = opts.Chain

	db.HashMap = make(map[UtxoKeyType][]byte, UTXO_RECORDS_PREALLOC)

	if opts.Rescan {
		return
	}

	// Load data form disk
	var k UtxoKeyType
	var cnt_dwn, cnt_dwn_from int
	var le uint64
	var u64, tot_recs uint64

	of, er := os.Open(db.dir_utxo + "UTXO.db")
	if er!=nil {
		of, er = os.Open(db.dir_utxo + "UTXO.old")
		if er!=nil {
			return
		}
	}

	defer of.Close()

	rd := bufio.NewReader(of)

	er = binary.Read(rd, binary.LittleEndian, &u64)
	if er != nil {
		goto fatal_error
	}
	db.LastBlockHeight = uint32(u64)

	db.LastBlockHash = make([]byte, 32)
	_, er = rd.Read(db.LastBlockHash)
	if er != nil {
		goto fatal_error
	}
	er = binary.Read(rd, binary.LittleEndian, &u64)
	if er != nil {
		goto fatal_error
	}

	fmt.Println("Last block height", db.LastBlockHeight, "   Number of records", u64)

	cnt_dwn_from = int(u64/100)

	for !AbortNow && tot_recs<u64 {
		le, er = btc.ReadVLen(rd)
		//er = binary.Read(rd, binary.LittleEndian, &le)
		if er!=nil {
			goto fatal_error
		}

		er = binary.Read(rd, binary.LittleEndian, &k)
		if er!=nil {
			goto fatal_error
		}


		b := make([]byte, int(le)-8)
		er = btc.ReadAll(rd, b)
		if er!=nil {
			goto fatal_error
		}

		db.HashMap[k] = b
		if db.ch.CB.LoadWalk!=nil {
			db.ch.CB.LoadWalk(NewQdbRecStatic(k, b))
		}

		tot_recs++
		if cnt_dwn==0 {
			fmt.Print("\rLoading UTXO.db - ", tot_recs*100/u64, "% complete ... ")
			cnt_dwn = cnt_dwn_from
		} else {
			cnt_dwn--
		}
	}

	fmt.Print("\r                                                              \r")

	db.CurrentHeightOnDisk = db.LastBlockHeight

	return

fatal_error:
	println("Fatal error when opening UTXO.db:", er.Error(), tot_recs, u64)
	AbortNow = true
	return
}


func (db *UnspentDB) save() {
	//var cnt_dwn, cnt_dwn_from, perc int
	var abort bool

	os.Rename(db.dir_utxo + "UTXO.db", db.dir_utxo + "UTXO.old")

	of, er := os.Create(db.dir_utxo + "UTXO.db.tmp")
	if er!=nil {
		println("Create file:", er.Error())
		return
	}

	start_time := time.Now()
	total_records := int64(len(db.HashMap))
	var timewaits int
	var current_record, data_progress, time_progress int64

	db.WritingProgress = 0

	wr := bufio.NewWriter(of)
	binary.Write(wr, binary.LittleEndian, uint64(db.LastBlockHeight))
	wr.Write(db.LastBlockHash)
	binary.Write(wr, binary.LittleEndian, uint64(total_records))
	for k, v := range db.HashMap {
		if !db.HurryUp {
			current_record = atomic.AddInt64(&db.WritingProgress, 1)
			if (current_record&0xf)==0 {
				data_progress = int64((current_record<<20)/total_records)
				time_progress = int64((time.Now().Sub(start_time)<<20) / UTXO_WRITING_TIME_TARGET)
				if data_progress > time_progress {
					time.Sleep(1e6)
					timewaits++
				}
			}
		}

		if db.AbortWriting {
			//println("abort")
			abort = true
			break
		}
		btc.WriteVlen(wr, uint64(8+len(v)))
		binary.Write(wr, binary.LittleEndian, k)
		_, er = wr.Write(v)
		if er != nil {
			println("\n\007Fatal error saving UTXO:", er.Error())
			abort = true
			break
		}
	}


	if abort {
		of.Close()
		os.Remove(db.dir_utxo + "UTXO.db.tmp")
	} else {
		db.DirtyDB = false
		wr.Flush()
		of.Close()
		os.Rename(db.dir_utxo + "UTXO.db.tmp", db.dir_utxo + "UTXO.db")
		//println("utxo written OK in", time.Now().Sub(start_time).String(), timewaits)
		db.CurrentHeightOnDisk = db.LastBlockHeight
	}

	db.WritingInProgress = false
}


// Commit the given add/del transactions to UTXO and Wnwind DBs
func (db *UnspentDB) CommitBlockTxs(changes *BlockChanges, blhash []byte) (e error) {
	undo_fn := fmt.Sprint(db.dir_undo, changes.Height)

	db.Mutex.Lock()
	defer db.Mutex.Unlock()
	db.abortifwriting("commit")

	if changes.UndoData!=nil || (changes.Height%db.UnwindBufLen)==0 {
		bu := new(bytes.Buffer)
		bu.Write(blhash)
		if changes.UndoData != nil {
			for _, xx := range changes.UndoData {
				bin := xx.Serialize(true)
				btc.WriteVlen(bu, uint64(len(bin)))
				bu.Write(bin)
			}
		}
		ioutil.WriteFile(db.dir_undo+"tmp", bu.Bytes(), 0666)
		os.Rename(db.dir_undo+"tmp", undo_fn)
	}

	db.commit(changes)

	if db.LastBlockHash==nil {
		db.LastBlockHash = make([]byte, 32)
	}
	copy(db.LastBlockHash, blhash)
	db.LastBlockHeight = changes.Height

	if changes.Height>db.UnwindBufLen {
		os.Remove(fmt.Sprint(db.dir_undo, changes.Height-db.UnwindBufLen))
	}

	db.DirtyDB = true
	return
}


func (db *UnspentDB) UndoBlockTxs(bl *btc.Block, newhash []byte) {
	db.Mutex.Lock()
	defer db.Mutex.Unlock()
	db.abortifwriting("undo")

	for _, tx := range bl.Txs {
		lst := make([]bool, len(tx.TxOut))
		for i := range lst {
			lst[i] = true
		}
		db.del(tx.Hash.Hash[:], lst)
	}

	fn := fmt.Sprint(db.dir_undo, db.LastBlockHeight)
	var addback []*QdbRec

	if _, er := os.Stat(fn); er != nil {
		fn += ".tmp"
	}

	dat, er := ioutil.ReadFile(fn)
	if er!=nil {
		panic(er.Error())
	}

	off := 32  // ship the block hash
	for off < len(dat) {
		le, n := btc.VLen(dat[off:])
		off += n
		qr := FullQdbRec(dat[off:off+le])
		off += le
		addback = append(addback, qr)
	}

	for _, tx := range addback {
		if db.ch.CB.NotifyTxAdd!=nil {
			db.ch.CB.NotifyTxAdd(tx)
		}

		ind := UtxoKeyType(binary.LittleEndian.Uint64(tx.TxID[:8]))
		v := db.HashMap[ind]
		if v != nil {
			oldrec := NewQdbRec(ind, v)
			for a := range tx.Outs {
				if tx.Outs[a]==nil {
					tx.Outs[a] = oldrec.Outs[a]
				}
			}
		}
		db.HashMap[ind] = tx.Bytes()
	}

	os.Remove(fn)
	db.LastBlockHeight--
	copy(db.LastBlockHash, newhash)
	db.DirtyDB = true
}


// Call it when the main thread is idle - this will do DB defrag
func (db *UnspentDB) Idle() bool {
	if db.volatimemode {
		return false
	}

	db.Mutex.Lock()
	defer db.Mutex.Unlock()

	if db.DirtyDB && !db.WritingInProgress {
		db.WritingInProgress = true
		//println("save", db.LastBlockHeight, "now")
		go db.save()
		return true
	}

	return false
}


// Flush the data and close all the files
func (db *UnspentDB) Close() {
	db.HurryUp = true
	db.Idle()
	for db.WritingInProgress {
		time.Sleep(1e7)
	}
}


// Get ne unspent output
func (db *UnspentDB) UnspentGet(po *btc.TxPrevOut) (res *btc.TxOut, e error) {
	ind := UtxoKeyType(binary.LittleEndian.Uint64(po.Hash[:8]))
	v := db.HashMap[ind]
	if v==nil {
		e = errors.New("Unspent TX not found")
		return
	}

	rec := NewQdbRec(ind, v)
	if len(rec.Outs)<=int(po.Vout) || rec.Outs[po.Vout]==nil {
		e = errors.New("Unspent VOut not found")
		return
	}
	res = new(btc.TxOut)
	res.VoutCount = uint32(len(rec.Outs))
	res.WasCoinbase = rec.Coinbase
	res.BlockHeight = rec.InBlock
	res.Value = rec.Outs[po.Vout].Value
	res.Pk_script = rec.Outs[po.Vout].PKScr
	return
}


func (db *UnspentDB) del(hash []byte, outs []bool) {
	ind := UtxoKeyType(binary.LittleEndian.Uint64(hash[:8]))
	v := db.HashMap[ind]
	if v==nil {
		return // no such txid in UTXO (just ignorde delete request)
	}
	rec := NewQdbRec(ind, v)
	if db.ch.CB.NotifyTxDel!=nil {
		db.ch.CB.NotifyTxDel(rec, outs)
	}
	var anyout bool
	for i, rm := range outs {
		if rm {
			rec.Outs[i] = nil
		} else if rec.Outs[i] != nil {
			anyout = true
		}
	}
	if anyout {
		db.HashMap[ind] = rec.Bytes()
	} else {
		delete(db.HashMap, ind)
	}
}


func (db *UnspentDB) commit(changes *BlockChanges) {
	// Now aplly the unspent changes
	for _, rec := range changes.AddList {
		ind := UtxoKeyType(binary.LittleEndian.Uint64(rec.TxID[:8]))
		if db.ch.CB.NotifyTxAdd!=nil {
			db.ch.CB.NotifyTxAdd(rec)
		}
		db.HashMap[ind] = rec.Bytes()
	}
	for k, v := range changes.DeledTxs {
		db.del(k[:], v)
	}
}


func (db *UnspentDB) abortifwriting(why string) {
	if db.WritingInProgress {
		db.AbortWriting = true
		for db.WritingInProgress {
			time.Sleep(1e6)
		}
		db.AbortWriting = false
	}
}


// Return DB statistics
func (db *UnspentDB) GetStats() (s string) {
	var outcnt, sum, sumcb, stealth_uns, stealth_tot uint64
	var totdatasize, unspendable uint64
	for k, v := range db.HashMap {
		totdatasize += uint64(len(v)+8)
		rec := NewQdbRecStatic(k, v)
		for idx, r := range rec.Outs {
			if r!=nil {
				outcnt++
				sum += r.Value
				if rec.Coinbase {
					sumcb += r.Value
				}
				if len(r.PKScr)>0 && r.PKScr[0]==0x6a {
					unspendable++
				}
				if r.IsStealthIdx() && idx+1<len(rec.Outs) {
					if rec.Outs[idx+1]!=nil {
						stealth_uns++
					}
					stealth_tot++
				}
			}
		}
	}
	s = fmt.Sprintf("UNSPENT: %.8f BTC in %d outs from %d txs. %.8f BTC in coinbase.\n",
		float64(sum)/1e8, outcnt, len(db.HashMap), float64(sumcb)/1e8)
	s += fmt.Sprintf(" TotalData:%.1fMB  MaxTxOutCnt:%d  DirtyDB:%t  Writing:%t  Abort:%t\n",
		float64(totdatasize)/1e6, len(rec_outs), db.DirtyDB, db.WritingInProgress, db.AbortWriting)
	s += fmt.Sprintf(" Last Block : %s @ %d\n", btc.NewUint256(db.LastBlockHash).String(),
		db.LastBlockHeight)
	s += fmt.Sprintf(" Number of unspendable outputs: %d.  Number of stealth indexes: %d / %d spent\n",
		unspendable, stealth_uns, stealth_tot)
	return
}
