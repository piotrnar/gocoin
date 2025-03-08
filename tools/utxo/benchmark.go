package main

import (
	"encoding/binary"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/lib/utxo"
)

func do_the_thing(db *utxo.UnspentDB, i int, tmp *uint32) {
	var (
		sta_rec  utxo.UtxoRec
		rec_outs = make([]*utxo.UtxoTxOut, utxo.MAX_OUTS_SEEN)
		rec_pool = make([]utxo.UtxoTxOut, utxo.MAX_OUTS_SEEN)
		rec_idx  int
		sta_cbs  = utxo.NewUtxoOutAllocCbs{
			OutsList: func(cnt int) []*utxo.UtxoTxOut {
				if len(rec_outs) < cnt {
					rec_outs = make([]*utxo.UtxoTxOut, cnt)
					rec_pool = make([]utxo.UtxoTxOut, cnt)
				}
				rec_idx = 0
				return rec_outs[:cnt]
			},
			OneOut: func() (res *utxo.UtxoTxOut) {
				res = &rec_pool[rec_idx]
				rec_idx++
				return
			},
		}
	)
	for k, vv := range db.HashMap[i] {
		v := db.GetData(vv)
		utxo.NewUtxoRecOwn(k, v, &sta_rec, &sta_cbs)
		atomic.AddUint32(tmp, sta_rec.InBlock)
	}
}

func utxo_benchmark(dir string) {
	var tmp uint32

	println("UtxoIdxLen:", utxo.UtxoIdxLen)

	sta := time.Now()
	db := utxo.NewUnspentDb(&utxo.NewUnspentOpts{Dir: dir})
	if db == nil {
		println("place UTXO.db or UTXO.old in the current folder")
		return
	}

	var cnt int
	for i := range db.HashMap {
		cnt += len(db.HashMap[i])
	}
	println(cnt, "UTXO records/txs loaded in", time.Since(sta).String())
	al, sy := sys.MemUsed()
	println("Mem Used:", al>>20, "/", sy>>20, "/", Memory.Bytes>>20)

	print("Going through the map...")
	sta = time.Now()
	for i := range db.HashMap {
		for k, v := range db.HashMap[i] {
			if v != nil {
				tmp += binary.LittleEndian.Uint32(k[:])
			}
		}
	}
	tim := time.Since(sta)
	println("\rGoing through the map done in", tim.String(), tmp)

	print("Going through the map for the slice...")
	tmp = 0
	sta = time.Now()
	for i := range db.HashMap {
		for _, vv := range db.HashMap[i] {
			v := db.GetData(vv)
			tmp += binary.LittleEndian.Uint32(v)
		}
	}
	println("\rGoing through the map for the slice done in", time.Since(sta).String(), tmp)

	var wg sync.WaitGroup
	print("Decoding all records in static parallel 256 ...")
	tmp = 0
	sta = time.Now()
	for i := range db.HashMap {
		wg.Add(1)
		go func(i int) {
			do_the_thing(db, i, &tmp)
			wg.Done()
		}(i)
	}
	wg.Wait()
	println("\rDecoding all records in static parallel 256 mode done in", time.Since(sta).String(), tmp)

	ticket := make(chan bool, runtime.NumCPU())
	print("Decoding all records in static parallel ", cap(ticket), " ...")
	tmp = 0
	sta = time.Now()
	for i := range db.HashMap {
		ticket <- true
		wg.Add(1)
		go func(i int) {
			do_the_thing(db, i, &tmp)
			wg.Done()
			<-ticket
		}(i)
	}
	wg.Wait()
	println("\rDecoding all records in static parallel", cap(ticket), "mode done in", time.Since(sta).String(), tmp)

	print("Decoding all records in static mode ...")
	tmp = 0
	sta = time.Now()
	for i := range db.HashMap {
		for k, vv := range db.HashMap[i] {
			v := db.GetData(vv)
			tmp += utxo.NewUtxoRecStatic(k, v).InBlock
		}
	}
	println("\rDecoding all records in static mode done in", time.Since(sta).String(), tmp)

	print("Decoding all records in dynamic mode ...")
	tmp = 0
	sta = time.Now()
	for i := range db.HashMap {
		for k, vv := range db.HashMap[i] {
			v := db.GetData(vv)
			tmp += utxo.NewUtxoRec(k, v).InBlock
		}
	}
	println("\rDecoding all records in dynamic mode done in", time.Since(sta).String(), tmp)

	al, sy = sys.MemUsed()
	println("Mem Used:", al>>20, "/", sy>>20, "/", Memory.Bytes>>20)
	db.Close()
}
