package memdb

import (
	"fmt"
	"errors"
	"encoding/hex"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/qdb"
)

/*
Each unspent key is prevOutIdxLen bytes long - thats part of the tx hash xored witth vout
Eech value is variable length:
  [0:32] - TxPrevOut.Hash
  [32:36] - TxPrevOut.Vout LSB
  [36:44] - Value LSB
  [44:] - Pk_script (in DBfile first 4 bytes are LSB length)
*/


const prevOutIdxLen = qdb.KeySize


type unspentDb struct {
	dir string
	tdb [0x100] *qdb.DB
	defragIndex int
	defragCount uint64
}

func newUnspentDB(dir string) (db *unspentDb) {
	db = new(unspentDb)
	db.dir = dir
	return
}


func (db *unspentDb) dbN(i int) (*qdb.DB) {
	if db.tdb[i]==nil {
		db.tdb[i], _ = qdb.NewDB(db.dir+fmt.Sprintf("%02x/", i))
		db.tdb[i].Load()
	}
	return db.tdb[i]
}


func getUnspIndex(po *btc.TxPrevOut) (idx [prevOutIdxLen]byte) {
	copy(idx[:], po.Hash[:prevOutIdxLen])
	idx[0] ^= byte(po.Vout)
	idx[1] ^= byte(po.Vout>>8)
	idx[2] ^= byte(po.Vout>>16)
	idx[3] ^= byte(po.Vout>>32)
	return
}


func (db *unspentDb) get(po *btc.TxPrevOut) (res *btc.TxOut, e error) {
	ind := getUnspIndex(po)
	val, _ := db.dbN(int(po.Hash[31])).Get(ind)
	if val==nil {
		//println(po.Hash[31], len(db.tdb[po.Hash[31]].Cache), hex.EncodeToString(ind[:]))
		//panic("Unspent not found")
		e = errors.New("Unspent not found")
		return
	}

	if len(val)<44 {
		panic(fmt.Sprint("unspent record too short:", len(val)))
	}
	
	res = new(btc.TxOut)
	res.Value = binary.LittleEndian.Uint64(val[36:44])
	res.Pk_script = make([]byte, len(val)-44)
	copy(res.Pk_script, val[44:])
	return
}


func (db *unspentDb) add(idx *btc.TxPrevOut, Val_Pk *btc.TxOut) {
	v := make([]byte, 44+len(Val_Pk.Pk_script))
	copy(v[0:32], idx.Hash[:])
	binary.LittleEndian.PutUint32(v[32:36], idx.Vout)
	binary.LittleEndian.PutUint64(v[36:44], Val_Pk.Value)
	copy(v[44:], Val_Pk.Pk_script)
	ind := getUnspIndex(idx)
	db.dbN(int(idx.Hash[31])).Put(ind, v)
	/*
	if idx.Hash[31]==169 {
		println("dodalem", len(db.tdb[idx.Hash[31]].Cache), hex.EncodeToString(ind[:]))
		for k, _ := range db.tdb[idx.Hash[31]].Cache {
			println(" *", hex.EncodeToString(k[:]))
		}
	}
	*/
}


func (db *unspentDb) idle() bool {
	for _ = range db.tdb {
		db.defragIndex++
		if db.defragIndex >= len(db.tdb) {
			db.defragIndex = 0
		}
		if db.tdb[db.defragIndex]!=nil && db.tdb[db.defragIndex].Defrag() {
			db.defragCount++
			//println(db.defragIndex, "defragmented")
			return true
		}
	}
	return false
}


func (db *unspentDb) del(idx *btc.TxPrevOut) {
	db.dbN(int(idx.Hash[31])).Del(getUnspIndex(idx))
}


func (db *unspentDb) GetAllUnspent(addr *btc.BtcAddr) (res []btc.OneUnspentTx) {
	for i := range db.tdb {
		for _, v := range db.dbN(i).Cache {
			if addr.Owns(v[8:]) {
				var nr btc.OneUnspentTx
				copy(nr.Output.Hash[:], v[0:32])
				nr.Output.Vout = binary.LittleEndian.Uint32(v[32:36])
				nr.Value = binary.LittleEndian.Uint64(v[36:44])
				res = append(res, nr)
			}
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
	var cnt, sum uint64
	var chsum [prevOutIdxLen]byte
	for i := range db.tdb {
		for k, v := range db.dbN(i).Cache {
			sum += binary.LittleEndian.Uint64(v[36:44])
			cnt++
			for i := range k {
				chsum[i] ^= k[i]
			}
		}
	}
	return fmt.Sprintf("UNSPENT: %.8f BTC in %d outputs. Checksum:%s  defrgs:%d\n", 
		float64(sum)/1e8, cnt, hex.EncodeToString(chsum[:]), db.defragCount)
}


func (db *unspentDb) sync() {
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].Sync()
		}
	}
}

func (db *unspentDb) nosync() {
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].NoSync()
		}
	}
}


func (db *unspentDb) save() {
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].Defrag()
		}
	}
}

func (db *unspentDb) close() {
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].Close()
		}
		db.tdb[i] = nil
	}
}


