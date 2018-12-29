package main

import (
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/lib/utxo"
	"os"
	"time"
)

func main() {
	var tmp uint32
	var dir = ""

	println("UtxoIdxLen:", utxo.UtxoIdxLen)
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}

	utxo.MembindInit()

	sta := time.Now()
	db := utxo.NewUnspentDb(&utxo.NewUnspentOpts{Dir:dir})
	if db == nil {
		println("place UTXO.db or UTXO.old in the current folder")
		return
	}

	println(len(db.HashMap), "UTXO records/txs loaded in", time.Now().Sub(sta).String())

	print("Going through the map...")
	sta = time.Now()
	for k, v := range db.HashMap {
		if v != nil {
			tmp += binary.LittleEndian.Uint32(k[:])
		}
	}
	tim := time.Now().Sub(sta)
	println("\rGoing through the map done in", tim.String(), tmp)

	print("Going through the map for the slice...")
	tmp = 0
	sta = time.Now()
	for _, v := range db.HashMap {
		tmp += binary.LittleEndian.Uint32(v)
	}
	println("\rGoing through the map for the slice done in", time.Now().Sub(sta).String(), tmp)

	print("Decoding all records in static mode ...")
	tmp = 0
	sta = time.Now()
	for k, v := range db.HashMap {
		tmp += utxo.NewUtxoRecStatic(k, v).InBlock
	}
	println("\rDecoding all records in static mode done in", time.Now().Sub(sta).String(), tmp)

	print("Decoding all records in dynamic mode ...")
	tmp = 0
	sta = time.Now()
	for k, v := range db.HashMap {
		tmp += utxo.NewUtxoRec(k, v).InBlock
	}
	println("\rDecoding all records in dynamic mode done in", time.Now().Sub(sta).String(), tmp)

	al, sy := sys.MemUsed()
	println("Mem Used:", al>>20, "/", sy>>20)
}
