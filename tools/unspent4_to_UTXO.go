package main

import (
	"strings"
	"strconv"
	"bufio"
	"encoding/binary"
	"fmt"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/qdb"
	"io"
	"io/ioutil"
	"os"
	"time"
)

func load_map4() (ndb map[qdb.KeyType][]byte) {
	var odb *qdb.DB
	ndb = make(map[qdb.KeyType][]byte, 21e6)
	for i := 0; i < 16; i++ {
		println("Loading", i, "...")
		er := qdb.NewDBExt(&odb, &qdb.NewDBOpts{Dir: fmt.Sprintf("%06d", i),
			Volatile: true, LoadData: true, WalkFunction: func(key qdb.KeyType, val []byte) uint32 {
				if _, ok := ndb[key]; ok {
					panic("duplicate")
				}
				ndb[key] = val
				return 0
			}})
		if er != nil {
			println(er.Error())
			println("Make sure to run this tool from insiode the unspent4/ directory")
			return
		}
		odb.Close()
	}
	return
}

func ReadAll(rd io.Reader, b []byte) (er error) {
	var n int
	for i := 0; i < len(b); i += n {
		n, er = rd.Read(b[i:])
		if er != nil {
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
	var maxbl_fn string

	fis, _ := ioutil.ReadDir("./")
	var maxbl, undobl int
	for _, fi := range fis {
		if !fi.IsDir() && fi.Size() >= 32 {
			ss := strings.SplitN(fi.Name(), ".", 2)
			cb, er := strconv.ParseUint(ss[0], 10, 32)
			if er == nil && int(cb) > maxbl {
				maxbl = int(cb)
				maxbl_fn = fi.Name()
				if len(ss) == 2 && ss[1] == "tmp" {
					undobl = maxbl
				}
			}
		}
	}
	if maxbl == 0 {
		println("This unspent4 database is corrupt")
		os.Exit(1)
	}
	if undobl == maxbl {
		println("This unspent4 database is not properly closed")
		os.Exit(1)
	}

	block_height := uint64(maxbl)
	block_hash := make([]byte, 32)

	f, _ := os.Open(maxbl_fn)
	f.Read(block_hash)
	f.Close()

	of, er := os.Create("UTXO3.db")
	if er != nil {
		println("Create file:", er.Error())
		return
	}

	cnt_dwn_from = len(ndb) / 100
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
		if cnt_dwn == 0 {
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
