package main

import (
	"fmt"
	"sync"
	"time"

	"github.com/piotrnar/gocoin/lib/others/memory"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/lib/utxo"
)

var (
	compress   bool
	decompress bool
	gc         int = -1
	ncpu       int = -1
	create_csv bool

	wg      sync.WaitGroup
	db      *utxo.UnspentDB
	tickets chan bool

	Memory   *memory.Allocator = memory.NewAllocator()
	MemMutex sync.Mutex
)

func one_map(i int) {
	var (
		rec      utxo.UtxoRec
		rec_outs = make([]*utxo.UtxoTxOut, utxo.MAX_OUTS_SEEN)
		rec_pool = make([]utxo.UtxoTxOut, utxo.MAX_OUTS_SEEN)
		rec_idx  int
		sta_cbs  = utxo.NewUtxoOutAllocCbs{
			OutsList: func(cnt int) (res []*utxo.UtxoTxOut) {
				if len(rec_outs) < cnt {
					println("utxo.MAX_OUTS_SEEN", len(rec_outs), "->", cnt)
					rec_outs = make([]*utxo.UtxoTxOut, cnt)
					rec_pool = make([]utxo.UtxoTxOut, cnt)
				}
				rec_idx = 0
				res = rec_outs[:cnt]
				for i := range res {
					res[i] = nil
				}
				return
			},
			OneOut: func() (res *utxo.UtxoTxOut) {
				res = &rec_pool[rec_idx]
				rec_idx++
				return
			},
		}
	)

	for k, v := range db.HashMap[i] {
		utxo.NewUtxoRecOwn(*v, &rec, &sta_cbs)
		if compress {
			db.HashMap[i][k] = utxo.SerializeC(&rec, nil)
		} else {
			db.HashMap[i][k] = utxo.SerializeU(&rec, nil)
		}
		if gc == -1 {
			utxo.Memory_Free(v)
		}
	}

	if tickets != nil {
		wg.Done()
		<-tickets
	}
}

func do_compress(dir string, compress, decompress bool, ncpu int) {
	sys.LockDatabaseDir(dir)
	defer sys.UnlockDatabaseDir()

	sta := time.Now()
	db = utxo.NewUnspentDb(&utxo.NewUnspentOpts{Dir: dir})
	if db == nil {
		println("UTXO.db (or UTXO.old) not found")
		return
	}
	fmt.Println("UTXO db open in", time.Since(sta))

	if db.ComprssedUTXO {
		fmt.Println("UTXO.db is compressed")
		compress = false
	} else {
		fmt.Println("UTXO.db is not-compressed")
		decompress = false
	}

	if !compress && !decompress {
		fmt.Println("No conversion requested or neccessary")
		db.Close()
		return
	}

	if compress {
		fmt.Print("Converting to compressed records")
	} else {
		fmt.Print("Converting to not-compressed records")
	}

	if ncpu > 1 {
		tickets := make(chan bool, ncpu)
		fmt.Println("  using", cap(tickets), "concurrent threads")
		sta = time.Now()
		for i := range db.HashMap {
			tickets <- true
			wg.Add(1)
			go one_map(i)
		}
		wg.Wait()
	} else {
		sta = time.Now()
		fmt.Println("  using single thread")
		for i := range db.HashMap {
			one_map(i)
		}
	}
	fmt.Println("Conversion took", time.Since(sta))
	db.ComprssedUTXO = compress
	db.DirtyDB.Set()
	fmt.Print("Saving new UTXO.db...")
	db.Close()
	fmt.Print("\r                      \r")
	fmt.Println("Saving UTXO.db took", time.Since(sta))
	fmt.Println("WARNING: the undo folder has not been converted")
}
