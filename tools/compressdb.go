// This tool can compress gocoins block database, but it is only needed if the database
// was created with an old gocoin, that did not support compression yet (before 0.2.15)
package main

import (
	"os"
	"bytes"
	"bufio"
	"compress/gzip"
	"encoding/binary"
)

func main() {
	var buf [1e6]byte
	var b [92]byte

	if len(os.Args)!=3 {
		println("To compress blockchain file specify input and output folder")
		println("Input folder must contain blockchain.idx and blockchain.dat")
		return
	}
	fi_idx, e := os.Open(os.Args[1]+"/blockchain.idx")
	if e != nil {
		println(e.Error())
		os.Exit(1)
	}
	defer fi_idx.Close()

	fi_dat, e := os.Open(os.Args[1]+"/blockchain.dat")
	if e != nil {
		println(e.Error())
		os.Exit(1)
	}
	defer fi_dat.Close()

	fo_idx_f, e := os.Create(os.Args[2]+"/blockchain.idx")
	if e != nil {
		println(e.Error())
		os.Exit(1)
	}
	defer fo_idx_f.Close()
	fo_idx := bufio.NewWriter(fo_idx_f)

	fo_dat_f, e := os.Create(os.Args[2]+"/blockchain.dat")
	if e != nil {
		println(e.Error())
		os.Exit(1)
	}
	defer fo_dat_f.Close()
	fo_dat := bufio.NewWriter(fo_dat_f)

	var totin, totout, blksize uint64
	for {
		_, e := fi_idx.Read(b[:])
		if e != nil {
			break
		}
		fpos := binary.LittleEndian.Uint64(b[80:88])
		blen := binary.LittleEndian.Uint32(b[88:92])

		fi_dat.Seek(int64(fpos), os.SEEK_SET)
		_, e = fi_dat.Read(buf[:blen])
		if e != nil {
			println(e.Error())
			return
		}

		if (b[0]&0x04)!=0 {
			// Already compressed
			blksize = uint64(blen)
			_, e = fo_dat.Write(buf[:blen])
			if e != nil {
				println(e.Error())
			}
		} else {
			cb := new(bytes.Buffer)
			zip_out := gzip.NewWriter(cb)
			zip_out.Write(buf[:blen])
			zip_out.Close()

			blksize = uint64(len(cb.Bytes()))

			_, e = fo_dat.Write(cb.Bytes())
			if e != nil {
				println(e.Error())
			}

			b[0] |= 0x04  // set compressed flag
		}
		binary.LittleEndian.PutUint64(b[80:88], uint64(totout))
		binary.LittleEndian.PutUint32(b[88:92], uint32(blksize))
		fo_idx.Write(b[:])

		totin += uint64(blen)
		totout += blksize

		if ((totout ^ (totout-uint64(blksize))) & 0xffff000000) != 0 {
			println(totin>>20, "MB. ", 100*totout/totin, "perc")
		}
	}
	fo_dat.Flush()
	fo_idx.Flush()
}
