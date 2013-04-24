package memdb

import (
	"os"
	"fmt"
	"bytes"
	"errors"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
)

type oneUnspent struct {
	btc.TxPrevOut
	btc.TxOut
}

/*
Each unspent key is prevOutIdxLen bytes long - thats part of the tx hash xored witth vout
Eech value is variable length:
  [0:32] - TxPrevOut.Hash
  [32:36] - TxPrevOut.Vout LSB
  [36:44] - Value LSB
  [44:] - Pk_script (in DBfile first 4 bytes are LSB length)


There is also a special index containnig all zeros (zeroIndex)
and the value is 32-bytes long hash of the last block.
*/


const prevOutIdxLen = 8


var (
	unspentdbase map[[prevOutIdxLen]byte] *oneUnspent
	
	db_version_seq uint32

	logfile *os.File
	logfile_pos int64
	
	zeroHash = []byte{0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0}
)



func checkLogfile() bool {
	if logfile==nil {
		logfile, _ = os.Create(dirname+"unspent.log")
		logfile_pos = 0
		if logfile!=nil {
			binary.Write(logfile, binary.LittleEndian, db_version_seq)
			println("stored log sequence", db_version_seq)
			logfile_pos = 4
		}
	}
	return logfile!=nil
}


func unspentSync() {
	if logfile!=nil {
		logfile.Sync()
	}
}

func unspentInit() {
	unspentdbase = make(map[[prevOutIdxLen]byte] *oneUnspent)
}


func delUnspDbFiles() {
	os.Remove(dirname+"unspent.0")
	os.Remove(dirname+"unspent.1")
}

func unspentClose() {
	if logfile!=nil {
		logfile.Close()
	}
	unspentdbase = nil
}


func getUnspIndex(po *btc.TxPrevOut) (idx [prevOutIdxLen]byte) {
	copy(idx[:], po.Hash[:prevOutIdxLen])
	idx[0] ^= byte(po.Vout)
	idx[1] ^= byte(po.Vout>>8)
	idx[2] ^= byte(po.Vout>>16)
	idx[3] ^= byte(po.Vout>>32)
	return
}


func (db UnspentDB) UnspentGet(po *btc.TxPrevOut) (res *btc.TxOut, e error) {
	//btc.ChSta("DB.UnspentGet")
	//fmt.Println(" ?", po.String())
	val, ok := unspentdbase[getUnspIndex(po)]
	if ok {
		res = &val.TxOut
	} else {
		e = errors.New("unspent not found")
	}
	//btc.ChSto("DB.UnspentGet")
	return
}


func addUnspent(idx *btc.TxPrevOut, Val_Pk *btc.TxOut) {
	unspentdbase[getUnspIndex(idx)] = &oneUnspent{TxPrevOut:*idx, TxOut:*Val_Pk}
	if checkLogfile() {
		logfile.Seek(logfile_pos, os.SEEK_SET)
		writeSpent(logfile, idx, Val_Pk)
		logfile_pos, _ = logfile.Seek(0, os.SEEK_CUR)
	}
}


func delUnspent(idx *btc.TxPrevOut) {
	delete(unspentdbase, getUnspIndex(idx))
	if checkLogfile() {
		logfile.Seek(logfile_pos, os.SEEK_SET)
		writeSpent(logfile, idx, nil)
		logfile_pos, _ = logfile.Seek(0, os.SEEK_CUR)
	}
}

func loadUnspent(f *os.File) (*oneUnspent, bool) {
	var b [48]byte

	// Read 32 bytes
	n, _ := f.Read(b[:32])
	if n != 32 {
		return nil, true
	}
	
	// Of all bytes are zero - this is lastBlockHash
	if bytes.Equal(b[:32], zeroHash[:]) {
		// lastBlockHash
		lastBlockHash = make([]byte, 32)
		n, _ = f.Read(lastBlockHash[:])
		if n != 32 {
			panic("Unexpexted end of file")
		}
		println("LastBlockHash:", btc.NewUint256(lastBlockHash).String())
		return nil, false
	}

	n, _ = f.Read(b[32:48])
	if n != 16 {
		panic("Unexpexted end of file")
	}

	//println(fpos, "-", hex.EncodeToString(b[0:32]), hex.EncodeToString(b[32:36]), hex.EncodeToString(b[36:44]))
	slen := binary.LittleEndian.Uint32(b[44:48])

	o := new(oneUnspent)
	copy(o.TxPrevOut.Hash[:], b[0:32])
	o.TxPrevOut.Vout = binary.LittleEndian.Uint32(b[32:36])
	o.TxOut.Value = binary.LittleEndian.Uint64(b[36:44])
	o.TxOut.Pk_script = make([]byte, slen)

	n, _ = f.Read(o.TxOut.Pk_script[:])
	if n != len(o.TxOut.Pk_script) {
		panic("Unexpexted end of file")
	}
	
	return o, false
}


func (o *oneUnspent) SaveTo(wr *os.File) {
	wr.Write(o.TxPrevOut.Hash[:])
	binary.Write(wr, binary.LittleEndian, uint32(o.TxPrevOut.Vout))
	binary.Write(wr, binary.LittleEndian, uint64(o.TxOut.Value))
	binary.Write(wr, binary.LittleEndian, uint32(len(o.TxOut.Pk_script)))
	wr.Write(o.TxOut.Pk_script[:])
	return
}


func (db UnspentDB) GetAllUnspent(addr *btc.BtcAddr) (res []btc.OneUnspentTx) {
	for _, v := range unspentdbase {
		if addr.Owns(v.TxOut.Pk_script) {
			var nr btc.OneUnspentTx
			nr.Output = v.TxPrevOut
			nr.Value = v.TxOut.Value
			res = append(res, nr)
		}
	}
	return
}


// Opens file and checks the ffffffff-sequence-FINI marker at the end
func openAndGetSeq(fn string) (f *os.File, seq uint32) {
	var b [12]byte
	var e error
	
	if f, e = os.Open(fn); e != nil {
		return
	}
	
	if _, e = f.Seek(-12, os.SEEK_END); e != nil {
		f.Close()
		f = nil
		return
	}

	if _, e = f.Read(b[:]); e != nil {
		f.Close()
		f = nil
		return
	}

	if binary.LittleEndian.Uint32(b[0:4])!=0xffffffff || string(b[8:12])!="FINI" {
		f.Close()
		f = nil
		return
	}

	seq = binary.LittleEndian.Uint32(b[4:8])
	return
}


func loadDataFromDisk() (e error) {
	// Try to read the database from the disk
	f, seq := openAndGetSeq(dirname+"unspent.0")
	f1, seq1 := openAndGetSeq(dirname+"unspent.1")
	
	if f == nil && f1 == nil {
		e = errors.New("No unspent database")
		return
	}

	if f!=nil && f1!=nil {
		// Both files are valid - take the one with higher sequence
		if int32(seq-seq1) > 0 {
			f1.Close()
			os.Remove(dirname+"unspent.1")
		} else {
			f.Close()
			f = f1
			os.Remove(dirname+"unspent.0")
		}
	} else if f==nil {
		f = f1
		seq = seq1
	}
	
	db_version_seq = seq
	
	// at this point we should have the db storage open in "f"
	fmt.Printf("Restoring unspent database from disk seq=%08x...\n", db_version_seq)
	
	// unspent txs data
	f.Seek(0, os.SEEK_SET)
	for {
		uns, eof := loadUnspent(f)
		if eof {
			break
		}
		if uns != nil {
			unspentdbase[getUnspIndex(&uns.TxPrevOut)] = uns
		}
	}
	f.Close()
	return
}


func appendDataFromLog() (e error) {
	fn := fmt.Sprintf(dirname+"unspent.log")
	logfile, e = os.OpenFile(fn, os.O_RDWR, 0660)
	if e == nil {
		var buf [36]byte
		var u32 uint32
		
		markerpos, _ := logfile.Seek(-36, os.SEEK_END)
		n, e := logfile.Read(buf[:])
		if n==36 && e==nil && string(buf[:4])=="MARK" {
			fmt.Println("Last block hash from log:", btc.NewUint256(buf[4:36]).String(), markerpos)
			setLastBlock(buf[4:36])
			logfile.Seek(0, os.SEEK_SET)
			
			e = binary.Read(logfile, binary.LittleEndian, &u32)
			if e != nil {
				println("logfile get sequence ", u32, e.Error())
				goto discard_log_file
			}
			
			if u32 != db_version_seq {
				println("logfile sequence mismatch", u32, db_version_seq)
				goto discard_log_file
			}
			
			var lastokpos, ad, de int64
			for {
				lastokpos, _ = logfile.Seek(0, os.SEEK_CUR)
				if lastokpos >= markerpos {
					break
				}

				po, to := readSpent(logfile)
				if po == nil {
					println("break at", lastokpos)
					break
				}
				idx := getUnspIndex(po)
				if to == nil {
					delete(unspentdbase, idx)
					de++
				} else {
					unspentdbase[idx] = &oneUnspent{TxPrevOut:*po, TxOut:*to}
					ad++
				}
			}
			logfile.Seek(lastokpos, os.SEEK_SET)
			fmt.Println(ad, "adds and", de, "dels, from the logfile ->", lastokpos)
		} else {
			goto discard_log_file
		}
	} else {
		fmt.Println("Log file not found", e.Error())
	}
	return

discard_log_file:
	fmt.Println("The log file does not look good - discard it")
	logfile.Close()
	logfile = nil
	os.Remove(fn)
	return
}


func unspentCommit(changes *btc.BlockChanges, blhash []byte) {
	// Now ally the unspent changes
	for k, v := range changes.AddedTxs {
		addUnspent(&k, v)
	}
	for k, _ := range changes.DeledTxs {
		delUnspent(&k)
	}
	setLastBlock(blhash)
	if logfile!=nil {
		logfile.Write([]byte("MARK"))
		logfile.Write(blhash[:])
	}
}


func init() {
	unspentInit()
}


