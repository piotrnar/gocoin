package btc

import (
	"io"
	"fmt"
	"bytes"
	"encoding/binary"
	"github.com/piotrnar/gocoin/qdb"
)


/*
Ine block record:
 * Tndex is the block height (64 bits)
 * 32 bytes of block's hash
 * Number of spent record:
   [0] - 1-added / 0 - deleted
   [1:33] - TxPrevOut.Hash
   [33:37] - TxPrevOut.Vout LSB
   Now optional ([0]==0):
     [37:45] - Value
     [45:49] - PK_Script length
     [49:] - PK_Script
*/


const (
	UnwindBufferMaxHistory = 5000  // Let's keep unwind history for so may last blocks
	NumberOfUnwindSubDBs = 10
)

type unwindDb struct {
	dir string
	tdb [NumberOfUnwindSubDBs] *qdb.DB
	lastBlockHeight uint32
	lastBlockHash [32]byte
	defragIndex int
	defragCount uint64
	nosyncinprogress bool
}

func (db *unwindDb) dbH(i int) (*qdb.DB) {
	if db.tdb[i]==nil {
		db.tdb[i], _ = qdb.NewDBCnt(db.dir+fmt.Sprintf("unw%03d", i), true,
			(UnwindBufferMaxHistory+NumberOfUnwindSubDBs-1)/NumberOfUnwindSubDBs)
		if db.nosyncinprogress {
			db.tdb[i].NoSync()
		}
	}
	return db.tdb[i]
}


func newUnwindDB(dir string) (db *unwindDb) {
	db = new(unwindDb)
	db.dir = dir
	for i := range db.tdb {
		// Load each of the sub-DBs into memory and try to find the highest block
		db.dbH(i).BrowseAll(func(k qdb.KeyType, v []byte) uint32 {
			h := uint32(k)
			if h > db.lastBlockHeight {
				db.lastBlockHeight = h
				copy(db.lastBlockHash[:], v[:32])
			}
			return qdb.NO_CACHE
		})
		if AbortNow {
			return
		}
	}
	return
}


func unwindFromReader(f io.Reader, unsp *unspentDb) {
	for {
		po, to := readSpent(f)
		if po == nil {
			break
		}
		if to != nil {
			// record deleted - so add it
			unsp.add(po, to)
		} else {
			// record added - so delete it
			unsp.del(po)
		}
	}
}


func (db *unwindDb) del(height uint32) {
	db.tdb[height%NumberOfUnwindSubDBs].Del(qdb.KeyType(height))
}


func (db *unwindDb) sync() {
	db.nosyncinprogress = false
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].Sync()
		}
	}
}

func (db *unwindDb) nosync() {
	db.nosyncinprogress = true
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].NoSync()
		}
	}
}

func (db *unwindDb) save() {
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].Flush()
		}
	}
}

func (db *unwindDb) close() {
	for i := range db.tdb {
		if db.tdb[i]!=nil {
			db.tdb[i].Close()
			db.tdb[i] = nil
		}
	}
}

func (db *unwindDb) idle() bool {
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

func (db *unwindDb) undo(height uint32, unsp *unspentDb) {
	if height != db.lastBlockHeight {
		panic("Unexpected height")
	}

	v := db.dbH(int(height)%NumberOfUnwindSubDBs).Get(qdb.KeyType(height))
	if v == nil {
		panic("Unwind data not found")
	}

	unwindFromReader(bytes.NewReader(v[32:]), unsp)
	db.del(height)

	db.lastBlockHeight--
	v = db.dbH(int(db.lastBlockHeight)%NumberOfUnwindSubDBs).Get(qdb.KeyType(db.lastBlockHeight))
	if v == nil {
		panic("Parent data not found")
	}
	copy(db.lastBlockHash[:], v[:32])
	return
}


func (db *unwindDb) commit(changes *BlockChanges, blhash []byte) {
	if db.lastBlockHeight+1 != changes.Height {
		println(db.lastBlockHeight+1, changes.Height)
		panic("Unexpected height")
	}
	db.lastBlockHeight++
	copy(db.lastBlockHash[:], blhash[0:32])

	f := new(bytes.Buffer)
	f.Write(blhash[0:32])
	// cast uin32 to int to properly discover negative diffs:
	if int(changes.LastKnownHeight) - int(changes.Height) < UnwindBufferMaxHistory {
		for k, _ := range changes.AddedTxs {
			writeSpent(f, &k, nil)
		}
		for k, v := range changes.DeledTxs {
			writeSpent(f, &k, v)
		}
	}
	db.dbH(int(changes.Height)%NumberOfUnwindSubDBs).PutExt(qdb.KeyType(changes.Height), f.Bytes(), qdb.NO_CACHE)
	if changes.Height >= UnwindBufferMaxHistory {
		db.del(changes.Height-UnwindBufferMaxHistory)
	}
}


func (db *unwindDb) GetLastBlockHash() (val []byte) {
	if db.lastBlockHeight != 0 {
		val = make([]byte, 32)
		copy(val, db.lastBlockHash[:])
	}
	return
}


func (db *unwindDb) stats() (s string) {
	var cnt int
	for i := range db.tdb {
		cnt += db.dbH(i).Count()
	}
	s = fmt.Sprintf("UNWIND: len:%d  last:%d  defrags:%d/%d\n",
		cnt, db.lastBlockHeight, db.defragCount, db.defragIndex)
	s += "Last block: " + NewUint256(db.lastBlockHash[:]).String() + "\n"
	return
}

func writeSpent(f io.Writer, po *TxPrevOut, to *TxOut) {
	if to == nil {
		// added
		f.Write([]byte{1})
		f.Write(po.Hash[:])
		binary.Write(f, binary.LittleEndian, uint32(po.Vout))
	} else {
		// deleted
		f.Write([]byte{0})
		f.Write(po.Hash[:])
		binary.Write(f, binary.LittleEndian, uint32(po.Vout))
		binary.Write(f, binary.LittleEndian, uint64(to.Value))
		binary.Write(f, binary.LittleEndian, uint32(len(to.Pk_script)))
		f.Write(to.Pk_script[:])
	}
}


func readSpent(f io.Reader) (po *TxPrevOut, to *TxOut) {
	var buf [49]byte
	n, e := f.Read(buf[:37])
	if n!=37 || e!=nil || buf[0]>1 {
		return
	}
	po = new(TxPrevOut)
	copy(po.Hash[:], buf[1:33])
	po.Vout = binary.LittleEndian.Uint32(buf[33:37])
	if buf[0]==0 {
		n, e = f.Read(buf[37:49])
		if n!=12 || e!=nil {
			panic("Unexpected end of file")
		}
		to = new(TxOut)
		to.Value = binary.LittleEndian.Uint64(buf[37:45])
		to.Pk_script = make([]byte, binary.LittleEndian.Uint32(buf[45:49]))
		f.Read(to.Pk_script[:])
	}
	return
}
