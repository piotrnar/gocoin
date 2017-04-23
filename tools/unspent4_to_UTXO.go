package main

import (
	"os"
	"io"
	"fmt"
	"time"
	"bufio"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/qdb"
	"github.com/piotrnar/gocoin/lib/btc"
)

func load_map4() (ndb map[qdb.KeyType][]byte) {
	var odb *qdb.DB
	ndb = make(map[qdb.KeyType][]byte, 20926242)
	for i := 0; i<16; i++ {
	println("Loading", i, "...")
		er := qdb.NewDBExt(&odb, &qdb.NewDBOpts{Dir:fmt.Sprintf("unspent4/%06d", i),
			Volatile:true, LoadData:true, WalkFunction:func(key qdb.KeyType, val []byte) uint32 {
				if _, ok := ndb[key]; ok {
					panic("duplicate")
				}
				ndb[key] = val
				return 0
			}})
		if er!=nil {
			println(er.Error())
			return
		}
		odb.Close()
	}
	return
}

func ReadAll(rd io.Reader, b []byte) (er error) {
	var n int
	for i:=0; i<len(b); i+=n {
		n, er = rd.Read(b[i:])
		if er!=nil {
			return
		}
	}
	return
}

/*
func load_map() (ndb map[qdb.KeyType][]byte) {
	var k qdb.KeyType
	var le uint32
	of, er := os.Open("unspent.db")
	if er!=nil {
		println("Create file:", er.Error())
		return
	}
	ndb = make(map[qdb.KeyType][]byte, 21e6)
	rd := bufio.NewReader(of)
	for {
		er = binary.Read(rd, binary.LittleEndian, &k)
		if er!=nil {
			break
		}

		//le, er = btc.ReadVLen(rd)
		er = binary.Read(rd, binary.LittleEndian, &le)
		if er!=nil {
			break
		}
		b := make([]byte, int(le))
		er = ReadAll(rd, b)
		if er!=nil {
			break
		}
		ndb[k] = b
	}
	of.Close()
	return
}
*/

func save_map(ndb map[qdb.KeyType][]byte) {
	var cnt_dwn, cnt_dwn_from, perc int

	block_height := uint64(463016)
	block_hash := []byte{
		0xC6,0x6D,0x3A,0xD0,0x51,0x43,0xBA,0x56,
		0x47,0xA0,0xB9,0x63,0x7B,0x25,0x07,0xE8,
		0x34,0x10,0xD5,0x10,0x6F,0x7B,0x55,0x00,
		0x00,0x00,0x00,0x00,0x00,0x00,0x00,0x00}

	of, er := os.Create("unspent4/UTXO3.db")
	if er!=nil {
		println("Create file:", er.Error())
		return
	}

	cnt_dwn_from = len(ndb)/100
	wr := bufio.NewWriter(of)
	binary.Write(wr, binary.LittleEndian, uint64(block_height))
	wr.Write(block_hash)
	binary.Write(wr, binary.LittleEndian, uint64(len(ndb)))
	for k, v := range ndb {
		btc.WriteVlen(wr, uint64(len(v)+8))
		binary.Write(wr, binary.LittleEndian, k)
		//binary.Write(wr, binary.LittleEndian, uint32(len(v)))
		_, er = wr.Write(v)
		if er != nil {
			println("\n\007Fatal error:", er.Error())
			break
		}
		if cnt_dwn==0 {
			fmt.Print("\rSaving UTXO.db - ", perc, "% complete ... ")
			cnt_dwn = cnt_dwn_from
			perc++
		} else {
			cnt_dwn--
		}
	}
	wr.Flush()
	of.Close()

	fmt.Print("\r                                                              \r")

/*

	of, er := os.Create("UTXO.db")
	if er!=nil {
		println("Create file:", er.Error())
		return
	}
	wr := bufio.NewWriter(of)
	binary.Write(wr, binary.LittleEndian, block_height)
	wr.Write(block_hash)
	binary.Write(wr, binary.LittleEndian, uint64(len(ndb)))
	for k, v := range ndb {
		binary.Write(wr, binary.LittleEndian, k)
		//btc.WriteVlen(wr, uint64(len(v)))
		binary.Write(wr, binary.LittleEndian, uint32(len(v)))
		_, er = wr.Write(v)
		if er != nil {
			println(er.Error())
			return
		}
	}
	wr.Flush()
	of.Close()
*/
}

func main() {
	var sta time.Time

	println("loading...")
	sta = time.Now()
	ndb := load_map4()
	println(len(ndb), "records loaded in", time.Now().Sub(sta).String())

	sta = time.Now()
	println("saving...")
	save_map(ndb)
	println("Finished in", time.Now().Sub(sta).String())
}
