package main

import (
	"fmt"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/lib/utxo"
	"os"
	"strings"
)

func main() {
	var dir = ""

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

	if db.ComprssedUTXO {
		fmt.Println("UTXO.db is already compressed.")
		return
	}

	fmt.Println("Compressing UTXO records")
	for k, v := range db.HashMap {
		rec := utxo.NewUtxoRecStatic(k, v)
		db.HashMap[k] = utxo.SerializeC(rec, false, nil)
	}
	db.ComprssedUTXO = true
	db.DirtyDB.Set()
	fmt.Println("Saving new UTXO.db")
	db.Close()
	fmt.Println("Done")

}
