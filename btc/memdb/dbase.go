package memdb

import (
	"os"
	"fmt"
	"time"
	"encoding/hex"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
)

var (
	lastBlockHash []byte
	dirname string
)


type UnspentDB struct {
}


func NewDb(dir string, init bool) btc.UnspentDB {
	var db UnspentDB
	dirname = dir+"unspent/"
	if init {
		os.RemoveAll(dirname)
	}
	os.MkdirAll(dirname, 0770)
	sta := time.Now().Unix()
	e := loadDataFromDisk()
	if e != nil {
		println("Error reading UnspentDB from disk:", e.Error())
		delUnspDbFiles()
		unspentInit()
	}
	sto := time.Now().Unix()
	fmt.Printf("loadDataFromDisk took %d seconds - %d records\n", sto-sta, len(unspentdbase))
	
	appendDataFromLog()
	fmt.Printf("appendDataFromLog took %d seconds - %d records\n", time.Now().Unix()-sto, len(unspentdbase))
	return &db
}


func setLastBlock(hash []byte) {
	lastBlockHash = make([]byte, 32)
	copy(lastBlockHash[:], hash[:32])
}

func (db UnspentDB) GetLastBlockHash() (val []byte) {
	val = lastBlockHash[:]
	return
}

func (db UnspentDB) CommitBlockTxs(changes *btc.BlockChanges, blhash []byte) (e error) {
	btc.ChSta("CommitBlockTxs")
	
	// First the unwind data
	unwindCommit(changes)

	unspentCommit(changes, blhash)

	btc.ChSto("CommitBlockTxs")

	return
}

func (db UnspentDB) GetStats() (s string) {
	s+= fmt.Sprintln("Best block:", btc.NewUint256(lastBlockHash[:]).String())
	
	var cnt, sum uint64
	var chsum [prevOutIdxLen]byte
	for k, v := range unspentdbase {
		sum += v.TxOut.Value
		cnt++
		for i := range k {
			chsum[i] ^= k[i]
		}
	}
	s += fmt.Sprintf("UNSPENT: %.8f BTC in %d outputs. Checksum:%s\n", 
		float64(sum)/1e8, cnt, hex.EncodeToString(chsum[:]))
	return
}


// Flush all the data to files
func (db UnspentDB) Sync() {
	unwindSync()
	unspentSync()
}


func (db UnspentDB) Close() {
	unwindSync()
	unspentClose()
}


func (db UnspentDB) Save() {
	db_version_seq++
	f, e := os.Create(fmt.Sprint(dirname, "unspent.", db_version_seq&1))
	if e != nil {
		panic(e.Error())
	}
	
	// unspent txs data
	for _, v := range unspentdbase {
		v.SaveTo(f)
	}
	
	// last block data
	f.Write(zeroHash)  // 32 bytes of zero
	f.Write(lastBlockHash[:])
	println("Saving Lash Block", btc.NewUint256(lastBlockHash[:]).String())
	
	// db_version_seq
	f.Write([]byte{0xff,0xff,0xff,0xff})  // 4 bytes of 0xff
	binary.Write(f, binary.LittleEndian, db_version_seq)
	f.Write([]byte("FINI"))  // mark file as completed
	f.Close()
	
	os.Remove(fmt.Sprint(dirname, "unspent.", db_version_seq&1^1))
	if logfile != nil {
		logfile.Close()
		logfile = nil
		os.Remove(dirname+"unspent.log")
	}
}



func init() {
	btc.NewUnspentDb = NewDb
}

