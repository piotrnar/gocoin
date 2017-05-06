package main

import (
	"os"
	"fmt"
	"time"
	"bufio"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
)

func main() {
	var buf [1024*1024]byte

	if len(os.Args)!=2 {
		fmt.Println("Specify the filename containing UTXO database")
		return
	}
	f, er := os.Open(os.Args[1])
	if er != nil {
		fmt.Println(er.Error())
		return
	}
	defer f.Close()

	rd := bufio.NewReader(f)
	er = btc.ReadAll(rd, buf[:48])

	if er != nil {
		fmt.Println(er.Error())
		return
	}

	o, _ := os.Create("UTXO.out")
	wr := bufio.NewWriter(o)
	wr.Write(buf[:48])

	records_cnt := binary.LittleEndian.Uint64(buf[40:48])

	fmt.Println("Last Block Height:", binary.LittleEndian.Uint64(buf[:8]))
	fmt.Println("Last Block Hash:", btc.NewUint256(buf[8:40]).String())
	fmt.Println("Number of UTXO records:", records_cnt)

	var cnt_dwn, cnt_dwn_from int
	var le uint64
	var tot_recs, out_records uint64
	var unspendable_recs, unspendable_outs uint64
	cnt_dwn_from = int(records_cnt/100)
	sta := time.Now()
	for tot_recs < records_cnt {
		le, er = btc.ReadVLen(rd)
		if er!=nil || le<32 {
			fmt.Println(er.Error())
			return
		}

		if int(le) > len(buf) {
			panic("buffer too small")
		}

		er = btc.ReadAll(rd, buf[:le])
		if er!=nil {
			fmt.Println(er.Error())
			return
		}

		rec := chain.NewUtxoRecStatic(chain.UtxoKeyType(binary.LittleEndian.Uint64(buf[:8])), buf[8:le])
		var spendable_found, output_removed bool
		for idx, r := range rec.Outs {
			if r!=nil {
				if len(r.PKScr)>0 && r.PKScr[0]==0x6a {
					unspendable_outs++
					rec.Outs[idx] = nil
				} else {
					spendable_found = true
				}
			}
		}
		if !spendable_found {
			unspendable_recs++
		} else {
			if output_removed {
				dat := rec.Serialize(true)
				btc.WriteVlen(wr, uint64(len(dat)))
				wr.Write(dat)
			} else {
				btc.WriteVlen(wr, uint64(le))
				wr.Write(buf[:le])
			}
			out_records++
		}

		tot_recs++
		if cnt_dwn==0 {
			fmt.Print("\r", tot_recs*100/records_cnt, "% complete ... ")
			cnt_dwn = cnt_dwn_from
		} else {
			cnt_dwn--
		}
	}
	wr.Flush()
	o.Close()
	fmt.Print("\r                                                              \r")
	fmt.Println("Done in", time.Now().Sub(sta).String())
	fmt.Println(unspendable_recs, "transactions and", unspendable_outs, "single records purged")
	fmt.Println("Updating number of records to", out_records)
	o, _ = os.OpenFile("UTXO.out", os.O_RDWR, 0600)
	o.Seek(40, os.SEEK_SET)
	binary.Write(o, binary.LittleEndian, &out_records)
	o.Close()
	fmt.Println("The purged database saved as UTXO.out")
}
