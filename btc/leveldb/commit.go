package leveldb

import (
//	"fmt"
	"os"
	"bytes"
	"encoding/binary"
    "encoding/hex"
	"github.com/piotrnar/gocoin/btc"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

/*
Each unspent key is 36 bytes long:
[0:32] - TxPrevOut.Hash
[32:36] - TxPrevOut.Vout LSB

Ech value is variable length
[0] - How many times
[1:9] - Value LSB
[9:] - Pk_script

------------------------------------
Unwind index is always 4 bytes - the block height MSB
Inside the value there is a set of records
 [0] - 1-added / 0 - deleted
 [1:33] - TxPrevOut.Hash
 [33:37] - TxPrevOut.Vout LSB
 These only for delted:
  [37:45] - Value
  [45:49] - PK_Script length
  [49:] - PK_Script
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

	// CompressionType:opt.NoCompression
	unspentdbase, e = leveldb.Open(unspentstorage, &opt.Options{Flag:opt.OFCreateIfMissing, WriteBuffer:64<<20})
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
	binary.LittleEndian.PutUint32(idx[32:36], po.Vout)
	return
}

func getUndoIdx(height uint32, add bool, po *btc.TxPrevOut) (idx []byte) {
	idx = make([]byte, 42)
	binary.LittleEndian.PutUint32(idx[0:4], height)
	if add {
		idx[4] = 1
	} else {
		idx[4] = 0
	}
	copy(idx[5:], po.Hash[:])
	binary.LittleEndian.PutUint32(idx[38:42], po.Vout)
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
	res.Value = binary.LittleEndian.Uint64(rec[1:9])
	res.Pk_script = make([]byte, len(rec)-9)
	copy(res.Pk_script[:], rec[9:])
	return
}


func addUnspent(batch *leveldb.Batch, idx []byte, Val_Pk *btc.TxOut) {
	//println("add", hex.EncodeToString(idx[:]))
	val, _ := unspentdbase.Get(idx, &opt.ReadOptions{})
	if val != nil {
		val[0]++
		println("add:", hex.EncodeToString(idx[:]), "->", val[0])
	} else {
		val = make([]byte, 1+8+len(Val_Pk.Pk_script))
		val[0] = 1
		binary.LittleEndian.PutUint64(val[1:9], Val_Pk.Value)
		copy(val[9:], Val_Pk.Pk_script[:])
	}
	batch.Put(idx, val)
}

func delUnspent(batch *leveldb.Batch, idx []byte) {
	//println("del", hex.EncodeToString(idx[:]))
	val, _ := unspentdbase.Get(idx, &opt.ReadOptions{})
	if val == nil {
		println(hex.EncodeToString(idx[:]))
		panic("del what??")
	}
	if val[0]>1 {
		val[0]--
		println("del:", hex.EncodeToString(idx[:]), "->", val[0])
		batch.Put(idx, val)
	} else {
		batch.Delete(idx)
	}
}


func (db BtcDB) CommitBlockTxs(changes *btc.BlockChanges) (e error) {
	var batch leveldb.Batch
	
	// Remove all the adds that are also in the del list
	for i := range changes.AddedTxs {
		if changes.AddedTxs[i]==nil {
			continue
		}
		for j := range changes.DeledTxs {
			if changes.DeledTxs[j]==nil {
				continue
			}
			if !bytes.Equal(changes.AddedTxs[i].Tx_Adr.Hash[:], changes.DeledTxs[j].Tx_Adr.Hash[:]) {
				continue
			}
			if changes.AddedTxs[i].Tx_Adr.Vout != changes.DeledTxs[j].Tx_Adr.Vout {
				continue
			}
			//println("add", i, "removes del", j)
			changes.AddedTxs[i] = nil
			changes.DeledTxs[j] = nil
			break
		}
	}
	
	// First the unwind data
	var val []byte
	var buf [49]byte // [0]-add, [1:33]-TxPrevOut.Hash, [33:37]-TxPrevOut.Vout LSB
	buf[0] = 1
	for i := range changes.AddedTxs {
		if changes.AddedTxs[i]==nil {
			continue
		}
		copy(buf[1:33], changes.AddedTxs[i].Tx_Adr.Hash[:])
		binary.LittleEndian.PutUint32(buf[33:37], changes.AddedTxs[i].Tx_Adr.Vout)
		val = append(val, buf[0:37]...)
	}
	
	buf[0] = 0
	for i := range changes.DeledTxs {
		if changes.DeledTxs[i]==nil {
			continue
		}
		copy(buf[1:33], changes.DeledTxs[i].Tx_Adr.Hash[:])
		binary.LittleEndian.PutUint32(buf[33:37], changes.DeledTxs[i].Tx_Adr.Vout)
		binary.LittleEndian.PutUint64(buf[37:45], changes.DeledTxs[i].Val_Pk.Value)
		binary.LittleEndian.PutUint32(buf[45:49], uint32(len(changes.DeledTxs[i].Val_Pk.Pk_script)))
		val = append(val, buf[0:49]...)
		val = append(val, changes.DeledTxs[i].Val_Pk.Pk_script...)
	}

	binary.BigEndian.PutUint32(buf[0:4], changes.Height)
	batch.Put(buf[0:4], val)
	if changes.Height >= btc.UnwindBufferMaxHistory {
		binary.BigEndian.PutUint32(buf[0:4], changes.Height-btc.UnwindBufferMaxHistory)
		batch.Delete(buf[0:4])
	}
	unwinddbase.Write(&batch, &opt.WriteOptions{})

	// Now ally the unspent changes
	batch.Reset()
	for i := range changes.AddedTxs {
		if changes.AddedTxs[i]!=nil {
			addUnspent(&batch, getUnspIdx(changes.AddedTxs[i].Tx_Adr), changes.AddedTxs[i].Val_Pk)
		}
	}
	for i := range changes.DeledTxs {
		if changes.DeledTxs[i]!=nil {
			delUnspent(&batch, getUnspIdx(changes.DeledTxs[i].Tx_Adr))
		}
	}
	unspentdbase.Write(&batch, &opt.WriteOptions{})
	
	return
}

func (db BtcDB) UndoBlockTransactions(height uint32) error {
	//panic("UndoBlockTransactions not implemented")
	var idx [4]byte
	binary.BigEndian.PutUint32(idx[:], height)
	val, e := unwinddbase.Get(idx[:], &opt.ReadOptions{})
	if e != nil {
		println(height)
		panic(e.Error())
		return e
	}
	
	var batch leveldb.Batch
	rd := bytes.NewReader(val[:])
	var buf [49]byte
	for {
		_, e = rd.Read(buf[:37])
		if e!=nil {
			break
		}
		if buf[0]==0 {
			// record deleted - so add it
			rd.Read(buf[37:49])
			var to btc.TxOut
			to.Value = binary.LittleEndian.Uint64(buf[37:45])
			to.Pk_script = make([]byte, binary.LittleEndian.Uint32(buf[45:49]))
			rd.Read(to.Pk_script[:])
			addUnspent(&batch, buf[1:37], &to)
		} else {
			// record added - so delete it
			batch.Delete(buf[1:37])
		}
	}
	unspentdbase.Write(&batch, &opt.WriteOptions{})
	unwinddbase.Delete(idx[:], &opt.WriteOptions{})
	//println(height, "unrolled")

	return nil
}


func (db BtcDB) GetUnspentFromPkScr(scr []byte) (res []btc.OneUnspentTx) {
	it := unspentdbase.NewIterator(&opt.ReadOptions{})
	for it.Next() {
		k := it.Key()
		v := it.Value()
		if bytes.Equal(v[9:], scr[:]) {
			value := binary.LittleEndian.Uint64(v[1:9]) * uint64(v[0])
			var po btc.TxPrevOut
			copy(po.Hash[:], k[0:32])
			po.Vout = binary.LittleEndian.Uint32(k[32:36])
			res = append(res, btc.OneUnspentTx{Value:value, Output: po})
		}
	}
	return
}

