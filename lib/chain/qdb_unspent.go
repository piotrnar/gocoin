package chain

import (
	"fmt"
	"errors"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/qdb"
)

/*
Each unspent key is prevOutIdxLen bytes long - thats part of the tx hash xored witth vout
Eech value is variable length:
  [0:32] - btc.TxPrevOut.Hash
  [32:36] - btc.TxPrevOut.Vout LSB
  [36:44] - Value LSB  (If COINBASE_VALBIT is set => it was coinbase TX, ignore it for the value)
  [44:48] - BlockHeight LSB (where mined)
  [48:] - Pk_script (in DBfile first 4 bytes are LSB length)
*/

const COINBASE_VALBIT = uint64(0x8000000000000000)


type unspentDb struct {
	dir string
	tdb [NumberOfUnspentSubDBs] *qdb.DB
	defragIndex int
	defragCount uint64
	nosyncinprogress bool
	lastHeight uint32
	noCacheBefore uint32
	ch *Chain
}


func newUnspentDB(dir string, lasth uint32, ch *Chain) (db *unspentDb) {
	db = new(unspentDb)
	db.dir = dir
	db.ch = ch
	db.setHeight(lasth)

	for i := range db.tdb {
		fmt.Print("\rLoading unspent DB - ", 100*i/len(db.tdb), "% complete ... ")
		db.dbN(i) // Load each of the sub-DBs into memory
		if AbortNow {
			return
		}
		if db.ch.CB.LoadFlush!=nil {
			db.ch.CB.LoadFlush()
		}
	}
	fmt.Print("\r                                                              \r")

	return
}

func (db *unspentDb) setHeight(lasth uint32) {
	db.lastHeight = lasth
	if NocacheBlocksBelow < 0 {
		res := int(db.lastHeight) + NocacheBlocksBelow + 1
		if res > 0 {
			db.noCacheBefore = uint32(res)
		} else {
			db.noCacheBefore = 0
		}
	} else {
		db.noCacheBefore = uint32(NocacheBlocksBelow)
	}
}


func (db *unspentDb) dbN(i int) (*qdb.DB) {
	if db.tdb[i]==nil {
		qdb.NewDBrowse(&db.tdb[i], db.dir+fmt.Sprintf("%06d", i), func(k qdb.KeyType, v []byte) uint32 {
			if db.ch.CB.LoadWalk!=nil {
				db.ch.CB.LoadWalk(db.tdb[i], k, NewWalkRecord(v))
			}
			if stealthIndex(v) {
				return qdb.YES_BROWSE|qdb.YES_CACHE // stealth output description
			} else if binary.LittleEndian.Uint32(v[44:48]) <= db.noCacheBefore {
				return qdb.NO_CACHE | qdb.NO_BROWSE
			} else if binary.LittleEndian.Uint64(v[36:44]) < MinBrowsableOutValue {
				return qdb.NO_CACHE | qdb.NO_BROWSE
			} else if btc.IsUsefullOutScript(v[SCR_OFFS:]) {
				return qdb.YES_BROWSE|qdb.YES_CACHE // if it was non-browsable, make it such now
			} else {
				return qdb.NO_CACHE | qdb.NO_BROWSE // useluess output - do not waste mem for it
			}
		}, SingeIndexSize)

		if db.nosyncinprogress {
			db.tdb[i].NoSync()
		}
	}
	return db.tdb[i]
}


func getUnspIndex(po *btc.TxPrevOut) (qdb.KeyType) {
	return qdb.KeyType(binary.LittleEndian.Uint64(po.Hash[:8]) ^ uint64(po.Vout))
}


func (db *unspentDb) get(po *btc.TxPrevOut) (res *btc.TxOut, e error) {
	ind := qdb.KeyType(po.UIdx())
	val := db.dbN(int(po.Hash[31])%NumberOfUnspentSubDBs).Get(ind)
	if val==nil {
		e = errors.New("Unspent not found")
		return
	}

	if len(val)<SCR_OFFS {
		panic(fmt.Sprint("unspent record too short:", len(val)))
	}

	res = new(btc.TxOut)
	res.Value = binary.LittleEndian.Uint64(val[36:44])
	if (res.Value&COINBASE_VALBIT)!=0 {
		res.Value &= ^COINBASE_VALBIT
		res.WasCoinbase = true
	}
	res.BlockHeight = binary.LittleEndian.Uint32(val[44:48])
	res.Pk_script = make([]byte, len(val)-SCR_OFFS)
	copy(res.Pk_script, val[SCR_OFFS:])
	return
}


func (db *unspentDb) add(idx *btc.TxPrevOut, Val_Pk *btc.TxOut) {
	v := make([]byte, SCR_OFFS+len(Val_Pk.Pk_script))
	copy(v[0:32], idx.Hash[:])
	binary.LittleEndian.PutUint32(v[32:36], idx.Vout)
	val := Val_Pk.Value
	if Val_Pk.WasCoinbase {
		val |= COINBASE_VALBIT
	}
	binary.LittleEndian.PutUint64(v[36:44], val)
	binary.LittleEndian.PutUint32(v[44:48], Val_Pk.BlockHeight)
	copy(v[SCR_OFFS:], Val_Pk.Pk_script)
	k := qdb.KeyType(idx.UIdx())
	var flgz uint32
	dbN := db.dbN(int(idx.Hash[31]) % NumberOfUnspentSubDBs)
	if stealthIndex(v) {
		if db.ch.CB.NotifyStealthTx!=nil {
			db.ch.CB.NotifyStealthTx(dbN, k, NewWalkRecord(v))
		}
		flgz = qdb.YES_CACHE|qdb.YES_BROWSE
	} else {
		if db.ch.CB.NotifyTx!=nil {
			db.ch.CB.NotifyTx(idx, Val_Pk)
		}
		if Val_Pk.Value < MinBrowsableOutValue {
			flgz = qdb.NO_CACHE | qdb.NO_BROWSE
		} else if NocacheBlocksBelow==-1 {
			flgz = qdb.NO_CACHE | qdb.NO_BROWSE
		}
	}
	dbN.PutExt(k, v, flgz)
}


func (db *unspentDb) del(idx *btc.TxPrevOut) {
	if db.ch.CB.NotifyTx!=nil {
		db.ch.CB.NotifyTx(idx, nil)
	}
	key := qdb.KeyType(idx.UIdx())
	db.dbN(int(idx.Hash[31])%NumberOfUnspentSubDBs).Del(key)
}


func bin2unspent(v []byte, ad *btc.BtcAddr) (nr *OneUnspentTx) {
	nr = new(OneUnspentTx)
	copy(nr.TxPrevOut.Hash[:], v[0:32])
	nr.TxPrevOut.Vout = binary.LittleEndian.Uint32(v[32:36])
	nr.Value = binary.LittleEndian.Uint64(v[36:44])
	nr.MinedAt = binary.LittleEndian.Uint32(v[44:48])
	nr.BtcAddr = ad
	return
}


func (db *unspentDb) commit(changes *BlockChanges) {
	// Now ally the unspent changes
	for k, v := range changes.AddedTxs {
		db.add(&k, v)
	}
	for k, _ := range changes.DeledTxs {
		db.del(&k)
	}
}


func (db *unspentDb) sync() {
	db.nosyncinprogress = false
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].Sync()
		}
	}
}

func (db *unspentDb) nosync() {
	db.nosyncinprogress = true
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].NoSync()
		}
	}
}

func (db *unspentDb) save() {
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].Flush()
		}
	}
}

func (db *unspentDb) close() {
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].Close()
			db.tdb[i] = nil
		}
	}
}

func (db *unspentDb) idle() bool {
	for _ = range db.tdb {
		db.defragIndex++
		if db.defragIndex >= len(db.tdb) {
			db.defragIndex = 0
		}
		if db.tdb[db.defragIndex]!=nil && db.tdb[db.defragIndex].Defrag() {
			db.defragCount++
			return true
		}
	}
	return false
}

func stealthIndex(v []byte) bool {
	return len(v)==SCR_OFFS+40 && v[SCR_OFFS]==0x6a && v[49]==0x26 && v[50]==0x06
}


func (db *unspentDb) browse(walk FunctionWalkUnspent, quick bool) {
	var i int
	brfn := func(k qdb.KeyType, v []byte) (fl uint32) {
		res := walk(db.tdb[i], k, NewWalkRecord(v))
		if (res&WALK_ABORT)!=0 {
			fl |= qdb.BR_ABORT
		}
		if (res&WALK_NOMORE)!=0 {
			fl |= qdb.NO_CACHE|qdb.NO_BROWSE
		}
		return
	}

	if quick {
		for i = range db.tdb {
			db.dbN(i).Browse(brfn)
		}
	} else {
		for i = range db.tdb {
			db.dbN(i).BrowseAll(brfn)
		}
	}
}

type OneWalkRecord struct {
	v []byte
}

func NewWalkRecord(v []byte) (r *OneWalkRecord) {
	r = new(OneWalkRecord)
	r.v = v
	return
}

func (r *OneWalkRecord) IsStealthIdx() bool {
	return len(r.v)==SCR_OFFS+40 &&
		r.v[SCR_OFFS]==0x6a && r.v[49]==0x26 && r.v[50]==0x06
}

func (r *OneWalkRecord) IsP2KH() bool {
	return len(r.v)==SCR_OFFS+25 &&
		r.v[SCR_OFFS+0]==0x76 && r.v[SCR_OFFS+1]==0xa9 && r.v[SCR_OFFS+2]==0x14 &&
		r.v[SCR_OFFS+23]==0x88 && r.v[SCR_OFFS+24]==0xac
}

func (r *OneWalkRecord) IsP2SH() bool {
	return len(r.v)==SCR_OFFS+23 && r.v[SCR_OFFS+0]==0xa9 && r.v[SCR_OFFS+1]==0x14 && r.v[SCR_OFFS+22]==0x87
}

func (r *OneWalkRecord) Script() []byte {
	return r.v[SCR_OFFS:]
}

func (r *OneWalkRecord) VOut() uint32 {
	return binary.LittleEndian.Uint32(r.v[32:36])
}

func (r *OneWalkRecord) TxID() []byte {
	return r.v[0:32]
}

func (r *OneWalkRecord) BlockHeight() uint32 {
	return binary.LittleEndian.Uint32(r.v[44:48])
}

func (r *OneWalkRecord) Value() uint64 {
	return binary.LittleEndian.Uint64(r.v[36:44])
}

func (r *OneWalkRecord) TxPrevOut() (res *btc.TxPrevOut) {
	res = new(btc.TxPrevOut)
	copy(res.Hash[:], r.v[0:32])
	res.Vout = binary.LittleEndian.Uint32(r.v[32:36])
	return
}

func (r *OneWalkRecord) ToUnspent(ad *btc.BtcAddr) (nr *OneUnspentTx) {
	nr = new(OneUnspentTx)
	copy(nr.TxPrevOut.Hash[:], r.v[0:32])
	nr.TxPrevOut.Vout = binary.LittleEndian.Uint32(r.v[32:36])
	nr.Value = binary.LittleEndian.Uint64(r.v[36:44])
	nr.MinedAt = binary.LittleEndian.Uint32(r.v[44:48])
	nr.BtcAddr = ad
	nr.destString = ad.String()
	return
}


func (db *unspentDb) stats() (s string) {
	var tot, brcnt, sum, stealth_cnt uint64
	var mincnt, maxcnt, totdatasize uint64
	for i := range db.tdb {
		dbcnt := uint64(db.dbN(i).Count())
		if i==0 {
			mincnt, maxcnt = dbcnt, dbcnt
		} else if dbcnt < mincnt {
			mincnt = dbcnt
		} else if dbcnt > maxcnt {
			maxcnt = dbcnt
		}
		tot += dbcnt
		db.dbN(i).Browse(func(k qdb.KeyType, v []byte) uint32 {
			if stealthIndex(v) {
				stealth_cnt++
			}
			val := binary.LittleEndian.Uint64(v[36:44])
			sum += val
			totdatasize += uint64(len(v))
			brcnt++
			return 0
		})
	}
	s = fmt.Sprintf("UNSPENT: %.8f BTC in %d/%d outputs. %d stealth outupts\n",
		float64(sum)/1e8, brcnt, tot, stealth_cnt)
	s += fmt.Sprintf(" Defrags:%d  Height:%d  NoCacheBelow:%d/%d  MinBrowsableVal:%d\n",
		db.defragCount, db.lastHeight, NocacheBlocksBelow, db.noCacheBefore, MinBrowsableOutValue)
	s += fmt.Sprintf(" Records per index : %d..%d   (config:%d)   TotalData:%.1fMB  AgedRecs:%d\n",
		mincnt, maxcnt, SingeIndexSize, float64(totdatasize)/1e6, UTXOAgedCount)
	return
}


func (db *UnspentDB) PrintCoinAge() {
	const chunk = 10000
	var maxbl uint32
	type rec struct {
		cnt, bts, val uint64
	}
	age := make(map[uint32] *rec)
	for i := range db.unspent.tdb {
		db.unspent.dbN(i).BrowseAll(func(k qdb.KeyType, v []byte) uint32 {
			a := binary.LittleEndian.Uint32(v[44:48])
			if a>maxbl {
				maxbl = a
			}
			a /= chunk
			tmp := age[a]
			if tmp==nil {
				tmp = new(rec)
			}
			tmp.val += binary.LittleEndian.Uint64(v[36:44])
			tmp.cnt++
			tmp.bts += uint64(len(v))
			age[a] = tmp
			return 0
		})
	}
	for i:=uint32(0); i<=(maxbl/chunk); i++ {
		tb := (i+1)*chunk-1
		if tb>maxbl {
			tb = maxbl
		}
		cnt := uint64(tb-i*chunk)+1
		fmt.Printf(" Blocks  %6d ... %6d: %9d records, %5d MB, %18s BTC.  Per block:%7.1f records,%8d,%15s BTC\n",
			i*chunk, tb, age[i].cnt, age[i].bts>>20, btc.UintToBtc(age[i].val),
			float64(age[i].cnt)/float64(cnt), (age[i].bts/cnt), btc.UintToBtc(age[i].val/cnt))
	}
}
