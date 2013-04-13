package leveldb

import (
	"fmt"
//	"errors"
	"bytes"
	"github.com/piotrnar/gocoin/btc"
	//"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

/*
unwind index is always 5 bytes
[0:4] - block height MSB
[4] - 1-added / 0 - deleted
*/


func (db BtcDB) UnwindNewRecord(height uint32, added bool, po *btc.TxPrevOut, rec *btc.TxOut) (e error) {
	//fmt.Println("UnwindNewRecord", added, po.String())
	var idx [5+32+4]byte
	
	idx[0] = byte(height>>24)
	idx[1] = byte(height>>16)
	idx[2] = byte(height>>8)
	idx[3] = byte(height)
	if added {
		idx[4] = 1
	} else {
		idx[4] = 0
	}

	copy(idx[5:37], po.Hash[:])
	idx[37] = byte(po.Vout>>24)
	idx[38] = byte(po.Vout>>16)
	idx[39] = byte(po.Vout>>8)
	idx[40] = byte(po.Vout)

	val := make([]byte, 8+len(rec.Pk_script))
	for i:=0; i<8; i++ {
		val[i] = byte(rec.Value>>(uint(i)*8))
	}
	copy(val[8:], rec.Pk_script[:])
	
	e = unwinddbase.Put(idx[:], val, &opt.WriteOptions{})
	return
}

func (db BtcDB) UnwindDel(height uint32) (e error) {
	var idx [4]byte
	idx[0] = byte(height>>24)
	idx[1] = byte(height>>16)
	idx[2] = byte(height>>8)
	idx[3] = byte(height)

	it := unwinddbase.NewIterator(&opt.ReadOptions{})
	it.Seek(idx[:])
	for it.Valid() {
		key := it.Key()
		if !bytes.Equal(key[:4], idx[:]) {
			break
		}
		unwinddbase.Delete(key, &opt.WriteOptions{})
		it.Next()
	}
	return
}


func (db BtcDB) UnwindBlock(height uint32) (e error) {
	fmt.Println("UnwindBlock", height)
	var idx [4]byte
	idx[0] = byte(height>>24)
	idx[1] = byte(height>>16)
	idx[2] = byte(height>>8)
	idx[3] = byte(height)

	it := unwinddbase.NewIterator(&opt.ReadOptions{})
	it.Seek(idx[:])
	for it.Valid() {
		key := it.Key()
		if !bytes.Equal(key[:4], idx[:]) {
			break
		}
		if key[4]==0 {
			// Deleted record - add it back to unspent
			unspentdbase.Put(key[5:], it.Value(), &opt.WriteOptions{})
		} else {
			// Added record - delete it from unspent
			unspentdbase.Delete(key[5:], &opt.WriteOptions{})
		}
		
		// Now delete this record after it has been applied
		unwinddbase.Delete(key, &opt.WriteOptions{})
		it.Next()
	}
	return
}

