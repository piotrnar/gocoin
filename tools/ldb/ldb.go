package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/syndtr/goleveldb/leveldb"
)

func countRecords(db *leveldb.DB) (int) {
	iter := db.NewIterator(nil, nil)
	defer iter.Release()

	count := 0
	for iter.Next() {
		count++
	}

	return count
}

func main() {
	var dir = "utxo.ldb"

	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	//fmt.Println("Opening LevelDB in", dir, "...")
	ldb, err := leveldb.OpenFile(dir, nil)
	if err != nil {
		fmt.Println(err.Error())
		os.Exit(1)
	}
	defer ldb.Close()
	
	var dif int

	compressedDB, err := ldb.Get([]byte("ComprssedUTXO"), nil)
	if err == nil {
		fmt.Println("ComprssedUTXO:", hex.EncodeToString(compressedDB))
		dif++
	}
	
	lastBlockHeightData, err := ldb.Get([]byte("LastBlockHeight"), nil)
	if err != nil {
		fmt.Println("Failed to get LastBlockHeight:", err)
	} else {
		if len(lastBlockHeightData)==8 {
			fmt.Println("Last Block Height:", binary.BigEndian.Uint64(lastBlockHeightData), "(8 bytes)")
		} else if len(lastBlockHeightData)==4 {
			fmt.Println("Last Block Height:", binary.LittleEndian.Uint32(lastBlockHeightData), "(4 bytes)")
		} else {
			fmt.Println("Last Block Height:", len(lastBlockHeightData), "bytes ***")
		}
		dif++
	}

	lastBlockHash, err := ldb.Get([]byte("LastBlockHash"), nil)
	if err != nil {
		fmt.Println("Failed to get LastBlockHash:", err)
	} else {
		fmt.Println("Last Block Hash:", btc.NewUint256(lastBlockHash).String())
		dif++
	}

	fmt.Print("counting records...")	
	fmt.Println("\rNumber of UTXO records:", countRecords(ldb)-dif)	
}
