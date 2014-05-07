package btc

import (
	"fmt"
	"errors"
	"encoding/binary"
	"github.com/piotrnar/gocoin/qdb"
)

/*
Each unspent key is prevOutIdxLen bytes long - thats part of the tx hash xored witth vout
Eech value is variable length:
  [0:32] - TxPrevOut.Hash
  [32:36] - TxPrevOut.Vout LSB
  [36:44] - Value LSB
  [44:48] - BlockHeight LSB (where mined)
  [48:] - Pk_script (in DBfile first 4 bytes are LSB length)
*/


const (
	prevOutIdxLen = qdb.KeySize
	NumberOfUnspentSubDBs = 0x10

	SCR_OFFS = 48
)

var (
	NocacheBlocksBelow uint // Do not keep in memory blocks older than this height
	MinBrowsableOutValue uint64 = 1e6 // Zero means: browse throutgh all
)


type unspentDb struct {
	dir string
	tdb [NumberOfUnspentSubDBs] *qdb.DB
	defragIndex int
	defragCount uint64
	nosyncinprogress bool
	notifyTx TxNotifyFunc
	lastHeight uint32
}


func newUnspentDB(dir string, lasth uint32) (db *unspentDb) {
	db = new(unspentDb)
	db.dir = dir
	db.lastHeight = lasth

	for i := range db.tdb {
		fmt.Print("\rLoading unspent DB - ", 100*i/len(db.tdb), "% complete ... ")
		db.dbN(i) // Load each of the sub-DBs into memory
		if AbortNow {
			return
		}
	}
	fmt.Print("\r                                                              \r")

	return
}


func (db *unspentDb) dbN(i int) (*qdb.DB) {
	if db.tdb[i]==nil {
		db.tdb[i], _ = qdb.NewDBrowse(db.dir+fmt.Sprintf("%06d", i), func(k qdb.KeyType, v []byte) uint32 {
				if stealthIndex(v) {
					return qdb.YES_BROWSE|qdb.YES_CACHE // stealth output description
				} else {
					return 0
				}
			})
		if db.nosyncinprogress {
			db.tdb[i].NoSync()
		}
	}
	return db.tdb[i]
}


func getUnspIndex(po *TxPrevOut) (qdb.KeyType) {
	return qdb.KeyType(binary.LittleEndian.Uint64(po.Hash[:8]) ^ uint64(po.Vout))
}


func (db *unspentDb) get(po *TxPrevOut) (res *TxOut, e error) {
	ind := qdb.KeyType(po.UIdx())
	val := db.dbN(int(po.Hash[31])%NumberOfUnspentSubDBs).Get(ind)
	if val==nil {
		e = errors.New("Unspent not found")
		return
	}

	if len(val)<SCR_OFFS {
		panic(fmt.Sprint("unspent record too short:", len(val)))
	}

	res = new(TxOut)
	res.Value = binary.LittleEndian.Uint64(val[36:44])
	res.BlockHeight = binary.LittleEndian.Uint32(val[44:48])
	res.Pk_script = make([]byte, len(val)-SCR_OFFS)
	copy(res.Pk_script, val[SCR_OFFS:])
	return
}


func (db *unspentDb) add(idx *TxPrevOut, Val_Pk *TxOut) {
	if db.notifyTx!=nil {
		db.notifyTx(idx, Val_Pk)
	}
	v := make([]byte, SCR_OFFS+len(Val_Pk.Pk_script))
	copy(v[0:32], idx.Hash[:])
	binary.LittleEndian.PutUint32(v[32:36], idx.Vout)
	binary.LittleEndian.PutUint64(v[36:44], Val_Pk.Value)
	binary.LittleEndian.PutUint32(v[44:48], Val_Pk.BlockHeight)
	copy(v[SCR_OFFS:], Val_Pk.Pk_script)
	ind := qdb.KeyType(idx.UIdx())
	var flgz uint32
	if stealthIndex(v) {
		flgz = qdb.YES_CACHE|qdb.YES_BROWSE
	} else {
		if Val_Pk.Value<MinBrowsableOutValue {
			flgz = qdb.NO_CACHE | qdb.NO_BROWSE
		} else if uint(Val_Pk.BlockHeight)<NocacheBlocksBelow {
			flgz = qdb.NO_CACHE
		}
	}
	db.dbN(int(idx.Hash[31])%NumberOfUnspentSubDBs).PutExt(ind, v, flgz)
}


func (db *unspentDb) del(idx *TxPrevOut) {
	if db.notifyTx!=nil {
		db.notifyTx(idx, nil)
	}
	key := qdb.KeyType(idx.UIdx())
	db.dbN(int(idx.Hash[31])%NumberOfUnspentSubDBs).Del(key)
}


func bin2unspent(v []byte, ad *BtcAddr) (nr *OneUnspentTx) {
	nr = new(OneUnspentTx)
	copy(nr.TxPrevOut.Hash[:], v[0:32])
	nr.TxPrevOut.Vout = binary.LittleEndian.Uint32(v[32:36])
	nr.Value = binary.LittleEndian.Uint64(v[36:44])
	nr.MinedAt = binary.LittleEndian.Uint32(v[44:48])
	nr.BtcAddr = ad
	return
}


func (db *unspentDb) buildAddrMap(addr []*BtcAddr) (addrs map[uint64]*BtcAddr) {
	addrs = make(map[uint64]*BtcAddr, len(addr))
	for i := range addr {
		addrs[binary.LittleEndian.Uint64(addr[i].Hash160[0:8])] = addr[i]
	}
	return
}

func (db *unspentDb) GetAllUnspent(addr []*BtcAddr, quick bool) (res AllUnspentTx) {
	if quick {
		addrs := db.buildAddrMap(addr)
		for i := range db.tdb {
			db.dbN(i).Browse(func(k qdb.KeyType, v []byte) uint32 {
				scr := v[SCR_OFFS:]
				if len(scr)==25 && scr[0]==0x76 && scr[1]==0xa9 && scr[2]==0x14 && scr[23]==0x88 && scr[24]==0xac {
					if ad, ok := addrs[binary.LittleEndian.Uint64(scr[3:3+8])]; ok {
						res = append(res, bin2unspent(v[:SCR_OFFS], ad))
					}
				} else if len(scr)==23 && scr[0]==0xa9 && scr[1]==0x14 && scr[22]==0x87 {
					if ad, ok := addrs[binary.LittleEndian.Uint64(scr[2:2+8])]; ok {
						res = append(res, bin2unspent(v[:SCR_OFFS], ad))
					}
				}
				return 0
			})
		}
	} else {
		for i := range db.tdb {
			db.dbN(i).BrowseAll(func(k qdb.KeyType, v []byte) uint32 {
				for a := range addr {
					if addr[a].Owns(v[SCR_OFFS:]) {
						res = append(res, bin2unspent(v[:SCR_OFFS], addr[a]))
					}
				}
				return 0
			})
		}
	}
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


func (db *unspentDb) stats() (s string) {
	var tot, cnt, sum, stealth_cnt uint64
	for i := range db.tdb {
		tot += uint64(db.dbN(i).Count())
		db.dbN(i).Browse(func(k qdb.KeyType, v []byte) uint32 {
			if stealthIndex(v) {
				stealth_cnt++
			}
			val := binary.LittleEndian.Uint64(v[36:44])
			sum += val
			cnt++
			return 0
		})
	}
	s = fmt.Sprintf("UNSPENT: %.8f BTC in %d/%d outputs. %d stealth outupts\n",
		float64(sum)/1e8, cnt, tot, stealth_cnt)
	s += fmt.Sprintf(" Defrags:%d  Height:%d  NocacheBelow:%d  MinOut:%d\n",
		db.defragCount, db.lastHeight, NocacheBlocksBelow, MinBrowsableOutValue)
	return
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
		res := walk(db.dbN(i), k, NewWalkRecord(v))
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

func (r *OneWalkRecord) ToUnspent(ad *BtcAddr) (nr *OneUnspentTx) {
	nr = new(OneUnspentTx)
	copy(nr.TxPrevOut.Hash[:], r.v[0:32])
	nr.TxPrevOut.Vout = binary.LittleEndian.Uint32(r.v[32:36])
	nr.Value = binary.LittleEndian.Uint64(r.v[36:44])
	nr.MinedAt = binary.LittleEndian.Uint32(r.v[44:48])
	nr.BtcAddr = ad
	return
}
