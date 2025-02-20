package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/lib/utxo"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/opt"
)

func main() {
	var dir = ""
	var dat [4]byte

	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	if dir != "" && !strings.HasSuffix(dir, string(os.PathSeparator)) {
		dir += string(os.PathSeparator)
	}

	sys.LockDatabaseDir(dir)
	defer sys.UnlockDatabaseDir()

	db := utxo.NewUnspentDb(&utxo.NewUnspentOpts{Dir: dir})
	if db == nil {
		println("UTXO.db not found.")
		return
	}

	fmt.Println("Converting UTXO.db to LevelDB")

	os.RemoveAll("utxo.ldb")
	ldb, err := leveldb.OpenFile("utxo.ldb", &opt.Options{NoSync:true})
	if err != nil {
		panic(err)
	}

	var cnt uint
	sta := time.Now()
	for i := range db.HashMap {
		fmt.Print("\rConverting map ", i+1, "/", len(db.HashMap), "...")
		for k, v := range db.HashMap[i] {
			err = ldb.Put(k[:], v, nil)
			if err != nil {
				panic(err)
			}
			cnt++
		}
	}
	fmt.Print("\r                                                               \r")

	//stores lastblockhash and lastblockheight
	ldb.Put([]byte("LastBlockHash"), db.LastBlockHash[:], nil)

	//lastblockheight
	binary.LittleEndian.PutUint32(dat[:], db.LastBlockHeight)
	ldb.Put([]byte("LastBlockHeight"), dat[:4], nil)
	
	var cp [1]byte
	if db.ComprssedUTXO {
		cp[0] = 1
	}
	ldb.Put([]byte("ComprssedUTXO"), cp[:], nil)
	
	ldb.Close()

	fmt.Println("Done", cnt, "records in", time.Since(sta).String())
}
