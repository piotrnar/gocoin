package main

import (
	"fmt"
	"os"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/piotrnar/gocoin/lib/others/memory"

	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/lib/utxo"
)

func print_help() {
	fmt.Println("Use this tool to compress or decompress UTXO database")
	fmt.Println("Possible arguments are:")
	fmt.Println("You have to specify at least one argument")
	fmt.Println("   * Path to UTXO.db file - to see basic info about it")
	fmt.Println("   * Path to dir (may be .) with UTXO db - (de)compress UTXO records")
	fmt.Println("   * -bench : to benchmark UTXO db")
	fmt.Println("   * -compr or -decompr : to request DB conversion")
	fmt.Println("   * -gc[<perc>] : to use native Go heap [with the given GC target percentage]")
	fmt.Println("   * -n[<num>] : to use concurrent threads [number of threads]")
	os.Exit(1)
}

func main() {
	var dir = ""
	var compress, decompress bool
	var benchmark bool
	var gc int = -1
	var ncpu int = -1

	if len(os.Args) < 2 {
		print_help()
	}

	for _, arg := range os.Args[1:] {
		arg = strings.ToLower(arg)
		if strings.HasPrefix(arg, "-h") {
			print_help()
		} else if strings.HasPrefix(arg, "-b") {
			if benchmark {
				println("ERROR: benchmark specified more than once")
				print_help()
			}
			benchmark = true
		} else if strings.HasPrefix(arg, "-com") {
			if compress {
				println("ERROR: compress specified more than once")
				print_help()
			}
			compress = true
		} else if strings.HasPrefix(arg, "-dec") {
			if decompress {
				println("ERROR: decompress specified more than once")
				print_help()
			}
			decompress = true
		} else if strings.HasPrefix(arg, "-gc") {
			if gc != -1 {
				println("ERROR: gc specified more than once")
				print_help()
			}
			if n, er := strconv.ParseUint(arg[3:], 10, 32); er == nil && n > 0 {
				gc = int(n)
			} else {
				gc = 30
				println("WARNING: Using default GC value")
			}
		} else if strings.HasPrefix(arg, "-n") {
			if ncpu != -1 {
				println("ERROR: n specified more than once")
				print_help()
			}
			if n, er := strconv.ParseUint(arg[2:], 10, 32); er == nil && n > 0 && n < 1e6 {
				ncpu = int(n)
			} else {
				ncpu = runtime.NumCPU()
				println("WARNING: Using default NumCPU value")
			}
		} else {
			if dir != "" {
				println("ERROR: db directory specified more than once")
				print_help()
			}
			dir = arg
		}
	}

	if ncpu == -1 {
		ncpu = 1
	}

	if dir != "" {
		if fi, er := os.Stat(dir); er == nil && !fi.IsDir() {
			decode_utxo_header(dir)
		}
	}

	if compress && decompress {
		println("ERROR: requested both; compress and decompress")
		print_help()
	}

	if dir != "" && !strings.HasSuffix(dir, string(os.PathSeparator)) {
		dir += string(os.PathSeparator)
	}

	if gc == -1 {
		var Memory memory.Allocator
		var MemMutex sync.Mutex

		fmt.Println("Using designated memory allocator")
		utxo.Memory_Malloc = func(le int) (res []byte) {
			MemMutex.Lock()
			res, _ = Memory.Malloc(le)
			MemMutex.Unlock()
			return
		}
		utxo.Memory_Free = func(ptr []byte) {
			MemMutex.Lock()
			Memory.Free(ptr)
			MemMutex.Unlock()
		}
	} else {
		fmt.Println("Using native Go heap with GC target of", gc)
		debug.SetGCPercent(gc)
	}
	sys.LockDatabaseDir(dir)
	defer sys.UnlockDatabaseDir()

	if benchmark {
		utxo_benchmark(dir)
		return
	}

	sta := time.Now()
	db := utxo.NewUnspentDb(&utxo.NewUnspentOpts{Dir: dir})
	if db == nil {
		println("UTXO.db (or UTXO.old) not found.")
		return
	}
	fmt.Println("UTXO db open in", time.Since(sta))

	if db.ComprssedUTXO {
		fmt.Println("UTXO.db is compressed")
	} else {
		fmt.Println("UTXO.db is not compressed")
	}

	if !compress && !decompress {
		fmt.Println("No conversion requested")
		db.Close()
		return
	}

	if compress {
		fmt.Println("Compressing UTXO records")
	} else {
		fmt.Println("Decompressing UTXO records")
	}

	var wg sync.WaitGroup
	tickets := make(chan bool, ncpu)
	fmt.Println("Using", cap(tickets), "concurrent threads")
	sta = time.Now()
	for i := range db.HashMap {
		tickets <- true
		wg.Add(1)
		go func(i int) {
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
				utxo.NewUtxoRecOwn(k, v, &rec, &sta_cbs)
				if compress {
					db.HashMap[i][k] = utxo.SerializeC(&rec, false, nil)
				} else {
					db.HashMap[i][k] = utxo.SerializeU(&rec, false, nil)
				}
				utxo.Memory_Free(v)
			}
			defer wg.Done()
			<-tickets
		}(i)
	}
	wg.Wait()
	fmt.Println("Done in", time.Since(sta))
	db.ComprssedUTXO = compress
	db.DirtyDB.Set()
	fmt.Println("Saving new UTXO.db...")
	db.Close()
	fmt.Println("Done in", time.Since(sta))
	fmt.Println("WARNING: the undo folder has not been converted")
}
