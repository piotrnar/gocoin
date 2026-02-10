package main

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/piotrnar/gocoin/lib/btc"
)

func decode_utxo_header(fn string) {
	var buf [48]byte
	var spare [1024 * 1024]byte
	fi, er := os.Open(fn)
	if er != nil {
		fmt.Println(er.Error())
		return
	}
	defer fi.Close()
	f := bufio.NewReaderSize(fi, 0x40000) // read ahed buffer size
	_, er = io.ReadFull(f, buf[:])
	if er != nil {
		fmt.Println(er.Error())
		return
	}
	le := binary.LittleEndian.Uint64(buf[:8])
	if (le & 0x8000000000000000) != 0 {
		fmt.Println("Records: Compressed")
	} else {
		fmt.Println("Records: Not compressed")
	}
	block_height := uint32(le)
	fmt.Println("Last Block Height:", block_height)
	fmt.Println("Last Block Hash:", btc.NewUint256(buf[8:40]).String())
	rec_cnt := binary.LittleEndian.Uint64(buf[40:48])
	fmt.Println("Number of UTXO records:", rec_cnt)

	if !create_csv {
		return
	}

	fname := fmt.Sprintf("data-%d.csv", block_height)
	lens := make(map[int]int)

	align := func(size uint64) uint64 {
		ali := uint64(8)
		//msiz := uint64(1024)
		msiz := uint64(1e6)
		for {
			if size < msiz {
				return (size + (ali - 1)) & ^(ali - 1)
			}
			ali <<= 1
			msiz <<= 1
		}

	}
	const MaxLen = 32724
	for i := uint64(0); i < rec_cnt; i++ {
		if (i & 0xfffff) == 0xfffff {
			fmt.Print("\rGathering record size stats from UTXO.db - ", 100*i/rec_cnt, "% complete...")
		}
		le, er = btc.ReadVLen(f)
		if er != nil {
			println("\n", er.Error())
			return
		}
		if _, er = io.ReadFull(f, spare[:int(le)]); er != nil {
			println("\n", er.Error())
			return
		}
		if le > MaxLen {
			continue
		}
		le = align(le)
		lens[int(le)]++
	}
	fmt.Println("\rNumber of unique sizes up to length", 32724, ":", len(lens), "                   ")
	type onerec struct {
		siz, cnt int
	}
	sss := make([]onerec, 0, len(lens))
	for k, v := range lens {
		sss = append(sss, onerec{siz: k, cnt: v})
	}
	sort.Slice(sss, func(i, j int) bool {
		if sss[i].cnt == sss[j].cnt {
			return sss[i].siz < sss[j].siz
		}
		return sss[i].cnt > sss[j].cnt
	})
	csv, er := os.Create(fname)
	if er != nil {
		println("ERROR:", er.Error())
		return
	}
	fmt.Fprintln(csv, "Size, Count")
	for _, r := range sss {
		//fmt.Println(i+1, "", r.siz, "->", r.cnt, "time(s)")
		fmt.Fprint(csv, r.siz, ", ", r.cnt, "\n")
	}
	csv.Close()
	fmt.Println("UTXO memory stats saved as", fname)
}
