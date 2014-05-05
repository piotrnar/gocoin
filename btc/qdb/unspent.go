package qdb

import (
	"fmt"
	"errors"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
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
	notifyTx btc.TxNotifyFunc
	lastHeight uint32
	stealthOuts map[qdb.KeyType] *stealthRec
}

func newUnspentDB(dir string, lasth uint32) (db *unspentDb) {
	db = new(unspentDb)
	db.dir = dir
	db.lastHeight = lasth
	db.stealthOuts = make(map[qdb.KeyType] *stealthRec)

	for i := range db.tdb {
		fmt.Print("\rLoading unspent DB - ", 100*i/len(db.tdb), "% complete ... ")
		db.dbN(i) // Load each of the sub-DBs into memory
		if btc.AbortNow {
			return
		}
	}
	fmt.Print("\r                                                              \r")

	return
}


type stealthRec struct {
	key qdb.KeyType
	dbidx int
	pkey []byte
	txid []byte
	vout uint32
}


func stealthIndexTo(k qdb.KeyType, v []byte) (res *stealthRec) {
	if len(v)==48+40 && v[48]==0x6a && v[49]==0x26 && v[50]==0x06 {
		res = new(stealthRec)
		vo := binary.LittleEndian.Uint32(v[32:36])
		res.key = qdb.KeyType(uint64(k) ^ uint64(vo) ^ uint64(vo+1))
		res.dbidx = int(v[31]) % NumberOfUnspentSubDBs
		res.pkey = v[55:]
		res.txid = v[:32]
		res.vout = vo
	}
	return
}


func (db UnspentDB) ScanStealth(walk func([]byte,[]byte,uint32,[]byte,uint64)bool) {
	var remd, rem2, okd uint
	for k, v := range db.unspent.stealthOuts {
		tx := db.unspent.dbN(v.dbidx).Get(v.key)
		if tx==nil {
			delete(db.unspent.stealthOuts, k)
			remd++
		} else {
			if walk(v.pkey, v.txid, v.vout, tx[48:],binary.LittleEndian.Uint64(tx[36:44])) {
				okd++
			} else {
				delete(db.unspent.stealthOuts, k)
				rem2++
			}
		}
	}
	if remd>0 {
		fmt.Println(okd, "stealth outputs")
		fmt.Println(remd, "+", rem2, "obsolete outputs have been removed")
	}
}


func (db *unspentDb) dbN(i int) (*qdb.DB) {
	if db.tdb[i]==nil {
		db.tdb[i], _ = qdb.NewDBrowse(db.dir+fmt.Sprintf("%06d", i), func(k qdb.KeyType, v []byte) uint32 {
				idx := stealthIndexTo(k, v)
				if idx != nil {
					db.stealthOuts[k] = idx
				}
				return 0
			})
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

	if len(val)<48 {
		panic(fmt.Sprint("unspent record too short:", len(val)))
	}

	res = new(btc.TxOut)
	res.Value = binary.LittleEndian.Uint64(val[36:44])
	res.BlockHeight = binary.LittleEndian.Uint32(val[44:48])
	res.Pk_script = make([]byte, len(val)-48)
	copy(res.Pk_script, val[48:])
	return
}


func (db *unspentDb) add(idx *btc.TxPrevOut, Val_Pk *btc.TxOut) {
	if db.notifyTx!=nil {
		db.notifyTx(idx, Val_Pk)
	}
	v := make([]byte, 48+len(Val_Pk.Pk_script))
	copy(v[0:32], idx.Hash[:])
	binary.LittleEndian.PutUint32(v[32:36], idx.Vout)
	binary.LittleEndian.PutUint64(v[36:44], Val_Pk.Value)
	binary.LittleEndian.PutUint32(v[44:48], Val_Pk.BlockHeight)
	copy(v[48:], Val_Pk.Pk_script)
	ind := qdb.KeyType(idx.UIdx())
	var flgz uint32
	sidx := stealthIndexTo(ind, v)
	if sidx != nil {
		db.stealthOuts[ind] = sidx
		flgz = qdb.NO_CACHE | qdb.NO_BROWSE
	} else {
		if Val_Pk.Value<MinBrowsableOutValue {
			flgz = qdb.NO_CACHE | qdb.NO_BROWSE
		} else if uint(Val_Pk.BlockHeight)<NocacheBlocksBelow {
			flgz = qdb.NO_CACHE
		}
	}
	db.dbN(int(idx.Hash[31])%NumberOfUnspentSubDBs).PutExt(ind, v, flgz)
}


func (db *unspentDb) del(idx *btc.TxPrevOut) {
	if db.notifyTx!=nil {
		db.notifyTx(idx, nil)
	}
	key := qdb.KeyType(idx.UIdx())
	delete(db.stealthOuts, key)
	db.dbN(int(idx.Hash[31])%NumberOfUnspentSubDBs).Del(key)
}


func bin2unspent(v []byte, ad *btc.BtcAddr) (nr *btc.OneUnspentTx) {
	nr = new(btc.OneUnspentTx)
	copy(nr.TxPrevOut.Hash[:], v[0:32])
	nr.TxPrevOut.Vout = binary.LittleEndian.Uint32(v[32:36])
	nr.Value = binary.LittleEndian.Uint64(v[36:44])
	nr.MinedAt = binary.LittleEndian.Uint32(v[44:48])
	nr.BtcAddr = ad
	return
}


func (db *unspentDb) buildAddrMap(addr []*btc.BtcAddr) (addrs map[uint64]*btc.BtcAddr) {
	addrs = make(map[uint64]*btc.BtcAddr, len(addr))
	for i := range addr {
		addrs[binary.LittleEndian.Uint64(addr[i].Hash160[0:8])] = addr[i]
	}
	return
}

func (db *unspentDb) GetAllUnspent(addr []*btc.BtcAddr, quick bool) (res btc.AllUnspentTx) {
	if quick {
		addrs := db.buildAddrMap(addr)
		for i := range db.tdb {
			db.dbN(i).Browse(func(k qdb.KeyType, v []byte) uint32 {
				scr := v[48:]
				if len(scr)==25 && scr[0]==0x76 && scr[1]==0xa9 && scr[2]==0x14 && scr[23]==0x88 && scr[24]==0xac {
					if ad, ok := addrs[binary.LittleEndian.Uint64(scr[3:3+8])]; ok {
						res = append(res, bin2unspent(v[:48], ad))
					}
				} else if len(scr)==23 && scr[0]==0xa9 && scr[1]==0x14 && scr[22]==0x87 {
					if ad, ok := addrs[binary.LittleEndian.Uint64(scr[2:2+8])]; ok {
						res = append(res, bin2unspent(v[:48], ad))
					}
				}
				return 0
			})
		}
	} else {
		for i := range db.tdb {
			db.dbN(i).BrowseAll(func(k qdb.KeyType, v []byte) uint32 {
				for a := range addr {
					if addr[a].Owns(v[48:]) {
						res = append(res, bin2unspent(v[:48], addr[a]))
					}
				}
				return 0
			})
		}
	}
	return
}


func (db *unspentDb) commit(changes *btc.BlockChanges) {
	// Now ally the unspent changes
	for k, v := range changes.AddedTxs {
		db.add(&k, v)
	}
	for k, _ := range changes.DeledTxs {
		db.del(&k)
	}
}


func (db *unspentDb) stats() (s string) {
	var tot, cnt, sum uint64
	for i := range db.tdb {
		tot += uint64(db.dbN(i).Count())
		db.dbN(i).Browse(func(k qdb.KeyType, v []byte) uint32 {
			val := binary.LittleEndian.Uint64(v[36:44])
			/*
			if val>=10e3*1e8 { // Look for outputs with over 10k BTC on them
				fmt.Println(val/1e8, "BTC in", binary.LittleEndian.Uint32(v[44:48]), "at",
					btc.NewUint256(v[0:32]).String(), binary.LittleEndian.Uint32(v[32:36]))
			}
			*/
			sum += val
			cnt++
			return 0
		})
	}
	s = fmt.Sprintf("UNSPENT: %.8f BTC in %d/%d outputs. %d stealth outupts\n",
		float64(sum)/1e8, cnt, tot, len(db.stealthOuts))
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
