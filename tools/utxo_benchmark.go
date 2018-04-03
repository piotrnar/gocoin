package main

import (
	"time"
	"github.com/piotrnar/gocoin/lib/utxo"
	"github.com/piotrnar/gocoin/lib/others/sys"
)

var a1 int

func main() {
	var tmp int

	println("UtxoIdxLen:", utxo.UtxoIdxLen)
	sta := time.Now()
	db := utxo.NewUnspentDb(&utxo.NewUnspentOpts{})
	if db == nil {
		println("place UTXO.db or UTXO.old in the current folder")
		return
	}

	println(len(db.HashMap), "UTXO records/txs loaded in", time.Now().Sub(sta).String())

	print("Going through the map...")
	sta = time.Now()
	for k, v := range db.HashMap {
		if (*byte)(v) == nil || k[0]==0 {
			tmp++
		}
	}
	tim := time.Now().Sub(sta)
	println("\rGoing through the map done in", tim.String())

	print("Going through the map for the slice...")
	sta = time.Now()
	for _, v := range db.HashMap {
		if utxo.Slice(v)[0]==0 {
			//ss[0] = 0
			tmp++
		}
	}
	println("\rGoing through the map for the slice done in", time.Now().Sub(sta).String())

	print("Fetching all records in static mode ...")
	sta = time.Now()
	for k, v := range db.HashMap {
		utxo.NewUtxoRecStatic(k, utxo.Slice(v))
		//utxo.NewUtxoRecStatic2(k, v)
	}
	println("\rFetching all records in static mode done in", time.Now().Sub(sta).String())

	print("Fetching all records in dynamic mode ...")
	sta = time.Now()
	for k, v := range db.HashMap {
		utxo.NewUtxoRec(k, utxo.Slice(v))
		//utxo.NewUtxoRec2(k, v)
	}
	println("\rFetching all records in dynamic mode done in", time.Now().Sub(sta).String())

	al, sy := sys.MemUsed()
	println("Mem Used:", al>>20, "/", sy>>20)
}
