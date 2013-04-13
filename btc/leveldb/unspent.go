package leveldb

import (
	"os"
	"bytes"
	"github.com/piotrnar/gocoin/btc"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
)


/*
Each key is 36 bytes long:
[0:32] - TxPrevOut.Hash
[32:36] - TxPrevOut.Vout LSB

Ech value is variable length
[0:8] - Value LSB
[8:] - Pk_script
*/


var (
	unspentstorage *storage.FileStorage
	unspentdbase *leveldb.DB
	
	unwindstorage *storage.FileStorage
	unwinddbase *leveldb.DB
)


type oneUnspent struct {
	prv *btc.TxPrevOut
	out *btc.TxOut
	cnt int
}

func unspentOpen() {
	var e error
	unspentstorage, e = storage.OpenFile(dirname+"/coinstate")
	if e != nil {
		panic(e.Error())
	}

	unspentdbase, e = leveldb.Open(unspentstorage, &opt.Options{Flag: opt.OFCreateIfMissing})
	if e != nil {
		panic(e.Error())
	}

	unwindstorage, e = storage.OpenFile(dirname+"/coinstate/unwind")
	if e != nil {
		panic(e.Error())
	}
	unwinddbase, e = leveldb.Open(unwindstorage, &opt.Options{Flag: opt.OFCreateIfMissing})
	if e != nil {
		panic(e.Error())
	}
}


func unspentClose() {
	unspentdbase.Close()
	unwinddbase.Close()
	unspentstorage.Close()
}

func (db BtcDB) UnspentPurge() {
	println("UnspentPurge()")
	unspentClose()
	os.RemoveAll(dirname+"/coinstate")
	unspentOpen()
}


func getUnspIdx(po *btc.TxPrevOut) (idx []byte) {
	idx = make([]byte, 36)
	copy(idx[:32], po.Hash[:])
	idx[32] = byte(po.Vout)
	idx[33] = byte(po.Vout>>8)
	idx[34] = byte(po.Vout>>16)
	idx[35] = byte(po.Vout>>24)
	return
}


func (db BtcDB) UnspentAdd(po *btc.TxPrevOut, rec *btc.TxOut) (e error) {
	//fmt.Println(" +", po.String())
	val := make([]byte, 8+len(rec.Pk_script))
	for i:=0; i<8; i++ {
		val[i] = byte(rec.Value>>(uint(i)*8))
	}
	copy(val[8:], rec.Pk_script[:])
	unspentdbase.Put(getUnspIdx(po), val[:], &opt.WriteOptions{})
	return
}


func (db BtcDB) UnspentGet(po *btc.TxPrevOut) (res *btc.TxOut, e error) {
	//fmt.Println(" ?", po.String())
	var rec []byte
	rec, e = unspentdbase.Get(getUnspIdx(po), &opt.ReadOptions{})
	if e!=nil {
		return
	}
	res = new(btc.TxOut)
	for i:=0; i<8; i++ {
		res.Value |= uint64(rec[i])<<(uint(i)*8)
	}
	res.Pk_script = make([]byte, len(rec)-8)
	copy(res.Pk_script[:], rec[8:])
	return
}

func (db BtcDB) UnspentDel(po *btc.TxPrevOut) (e error) {
	//fmt.Println(" -", po.String())
	e = unspentdbase.Delete(getUnspIdx(po), &opt.WriteOptions{})
	return
}

func (db BtcDB) GetUnspentFromPkScr(scr []byte) (list []btc.OneUnspentTx) {
	//fmt.Println("Looking for", hex.EncodeToString(scr[:]))
	
	it := unspentdbase.NewIterator(&opt.ReadOptions{})
	for it.Next() {
		val := it.Value()
		if bytes.Equal(val[8:], scr[:]) {
			//fmt.Println(" YES!!", hex.EncodeToString(val[:]))
			key := it.Key()
			rec := new(btc.OneUnspentTx)
			
			copy(rec.Output.Hash[:], key[0:32])
			rec.Output.Vout = uint32(key[32]) | (uint32(key[33])<<8) |
				(uint32(key[34])<<16) | (uint32(key[35])<<24)
			
			for i:=0; i<8; i++ {
				rec.Value |= uint64(val[i])<<(uint(i)*8)
			}

			list = append(list, *rec)
		} else {
			//fmt.Println(" no", hex.EncodeToString(val[:]))
		}
	}
	return
}

