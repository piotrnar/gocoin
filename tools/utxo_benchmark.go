package main

import (
	"time"
	"github.com/piotrnar/gocoin/lib/utxo"
)

var a1 int

func main() {
	var tmp int

	sta := time.Now()
	db := utxo.NewUnspentDb(&utxo.NewUnspentOpts{})
	if db == nil {
		println("place UTXO.db or UTXO.old in the current folder")
		return
	}

	println(len(db.HashMap), "UTXO records/txs loaded in", time.Now().Sub(sta).String())

	print("Checking how quickly we hav go through the map...")
	sta = time.Now()
	for k, v := range db.HashMap {
		if false {
			v = v
			k = k
		}
	}
	tim := time.Now().Sub(sta)
	println("\rGoing through the map done in", tim.String())

	print("Going through the map with using the slice...")
	sta = time.Now()
	for _, v := range db.HashMap {
		ss := utxo.Slice(v)
		//ss := *(*[]byte)(v)
		if ss[0]==0 {
			//ss[0] = 0
			tmp++
		}
	}
	println("\rGoing through the map with using the slice done in", time.Now().Sub(sta).String())

	print("Making all static records ...")
	sta = time.Now()
	for k, v := range db.HashMap {
		utxo.NewUtxoRecStatic(k, utxo.Slice(v))
		//utxo.NewUtxoRecStatic2(k, v)
	}
	println("\rMaking all static records done in", time.Now().Sub(sta).String())

	print("Making all dynamic records...")
	sta = time.Now()
	for k, v := range db.HashMap {
		utxo.NewUtxoRec(k, utxo.Slice(v))
		//utxo.NewUtxoRec2(k, v)
	}
	println("\rMaking all dynamic records done in", time.Now().Sub(sta).String())

	println("Ctrl+c ... (you can check your memory now)")
	for {
		time.Sleep(1e9)
	}
}
