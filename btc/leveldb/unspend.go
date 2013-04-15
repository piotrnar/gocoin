package leveldb

import (
	"fmt"
	"os"
	"bytes"
	"errors"
//    "encoding/hex"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
//	"github.com/syndtr/goleveldb/leveldb/cache"
	"github.com/syndtr/goleveldb/leveldb/storage"
)

var zeroIndex [40]byte // by default this will be filled with zeros

var DoNotUnwind bool

/*
Each unspent key is 40 bytes long:
[0:32] - TxPrevOut.Hash
[32:36] - TxPrevOut.Vout LSB
[36:40] - Block height (where the tx came from) LSB

Ech value is variable length
[0:8] - Value LSB
[8:] - Pk_script


There is also a special index containnig all zeros (zeroIndex)
and the value is 32-bytes long hash of the last block.
*/

/*
Unwind index is always 4 bytes - the block height LSB
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

	//ca := cache.NewLRUCache(1e6)
	// CompressionType:opt.NoCompression
	unspentdbase, e = leveldb.Open(unspentstorage, &opt.Options{Flag:opt.OFCreateIfMissing, 
		WriteBuffer:64<<20,
		//BlockCache:ca,
		//BlockSize:16*1024,
		//DefaultBlockCacheSize:128<<20,
		//DefaultBlockSize            = 4096
		//DefaultBlockRestartInterval = 16
		//CompressionType:opt.NoCompression,
		})
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


func getUnspSeek(po *btc.TxPrevOut) (idx []byte) {
	idx = make([]byte, 36)
	copy(idx[:32], po.Hash[:])
	binary.LittleEndian.PutUint32(idx[32:36], po.Vout)
	return
}


func getUnspIndex(po *btc.TxPrevOut, height uint32) (idx []byte) {
	idx = make([]byte, 40)
	copy(idx[:32], po.Hash[:])
	binary.LittleEndian.PutUint32(idx[32:36], po.Vout)
	binary.LittleEndian.PutUint32(idx[36:40], height)
	return
}

func (db BtcDB) UnspentGet(po *btc.TxPrevOut) (res *btc.TxOut, e error) {
	//fmt.Println(" ?", po.String())
	filt := getUnspSeek(po)
	it := unspentdbase.NewIterator(&opt.ReadOptions{})
	if it.Seek(filt) && bytes.Equal(it.Key()[:36], filt[:]) {
		val := it.Value()
		res = new(btc.TxOut)
		res.Value = binary.LittleEndian.Uint64(val[0:8])
		//fmt.Println("Got", idx2str(it.Key()), res.Value)
		res.Pk_script = make([]byte, len(val)-8)
		copy(res.Pk_script[:], val[8:])
	} else {
		e = errors.New("unspent not found")
	}
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

func idx2str(idx []byte) string {
	return fmt.Sprintf("%s-%03d @ %d", btc.NewUint256(idx[:32]).String(),
		binary.LittleEndian.Uint32(idx[32:36]),
		binary.LittleEndian.Uint32(idx[36:40]))
}


func addUnspent(batch *leveldb.Batch, idx []byte, Val_Pk *btc.TxOut) {
	//fmt.Println(" + ", idx2str(idx), Val_Pk.Value)
	val := make([]byte, 8+len(Val_Pk.Pk_script))
	binary.LittleEndian.PutUint64(val[0:8], Val_Pk.Value)
	copy(val[8:], Val_Pk.Pk_script[:])
	batch.Put(idx, val)
}

func delUnspent(batch *leveldb.Batch, idx []byte) {
	//println("del", hex.EncodeToString(idx[:]))
	it := unspentdbase.NewIterator(&opt.ReadOptions{})
	if it.Seek(idx) && bytes.Equal(it.Key()[:36], idx[:]) {
		batch.Delete(it.Key())
	} else {
		panic("Unspent not found")
	}
}

func setLastBlock(batch *leveldb.Batch, hash []byte) {
	batch.Put(zeroIndex[:], hash)
}

func (db BtcDB) GetLastBlockHash() (val []byte) {
	val, _ = unspentdbase.Get(zeroIndex[:], &opt.ReadOptions{})
	return
}

func (db BtcDB) CommitBlockTxs(changes *btc.BlockChanges, blhash []byte) (e error) {
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
	if !DoNotUnwind {
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

		binary.LittleEndian.PutUint32(buf[0:4], changes.Height)
		batch.Put(buf[0:4], val)
		if changes.Height >= btc.UnwindBufferMaxHistory {
			binary.LittleEndian.PutUint32(buf[0:4], changes.Height-btc.UnwindBufferMaxHistory)
			batch.Delete(buf[0:4])
		}
		unwinddbase.Write(&batch, &opt.WriteOptions{})
		batch.Reset()
	}

	// Now ally the unspent changes
	for i := range changes.AddedTxs {
		if changes.AddedTxs[i]!=nil {
			addUnspent(&batch, getUnspIndex(changes.AddedTxs[i].Tx_Adr, changes.Height), changes.AddedTxs[i].Val_Pk)
		}
	}
	for i := range changes.DeledTxs {
		if changes.DeledTxs[i]!=nil {
			delUnspent(&batch, getUnspSeek(changes.DeledTxs[i].Tx_Adr))
		}
	}
	setLastBlock(&batch, blhash)
	unspentdbase.Write(&batch, &opt.WriteOptions{})
	
	return
}

func (db BtcDB) UndoBlockTransactions(height uint32, blhash []byte) error {
	if DoNotUnwind {
		panic("Unwinding is disabled")
	}
	var idx [4]byte
	binary.LittleEndian.PutUint32(idx[:], height)
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
			delUnspent(&batch, buf[1:37])
		}
	}
	setLastBlock(&batch, blhash)
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
		if bytes.Equal(v[8:], scr[:]) {
			value := binary.LittleEndian.Uint64(v[0:8])
			var po btc.TxPrevOut
			copy(po.Hash[:], k[0:32])
			po.Vout = binary.LittleEndian.Uint32(k[32:36])
			res = append(res, btc.OneUnspentTx{Value:value, Output: po})
		}
	}
	return
}

