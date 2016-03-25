package main

import (
	"os"
	"fmt"
	"flag"
	"io/ioutil"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
)

/*
	blockchain.dat - contains raw blocks data, no headers, nothing
	blockchain.new - contains records of 136 bytes (all values LSB):
		[0] - flags:
			bit(0) - "trusted" flag - this block's scripts have been verified
			bit(1) - "invalid" flag - this block's scripts have failed
			bit(2) - "compressed" flag - this block's data is compressed
			bit(3) - "snappy" flag - this block is compressed with snappy (not gzip'ed)
		[4:36]  - 256-bit block hash
		[36:40] - 32-bit block height (genesis is 0)
		[40:48] - 64-bit block pos in blockchain.dat file
		[48:52] - 32-bit block lenght in bytes
		[52:56] - 32-bit number of transaction in the block
		[56:136] - 80 bytes blocks header
*/
var (
	fl_help bool
	fl_block uint
	fl_dir string
	fl_scan bool
	fl_split string
	fl_skip uint
	fl_append string
	fl_trunc bool
)


func print_record(sl []byte) {
	bh := btc.NewSha2Hash(sl[56:136])
	fmt.Print("Block ", bh.String(), " Height ", binary.LittleEndian.Uint32(sl[36:40]))
	fmt.Println()
	fmt.Println("  ...", binary.LittleEndian.Uint32(sl[48:52]), " Bytes @ ",
		binary.LittleEndian.Uint64(sl[40:48]), " in dat file")
	hdr := sl[56:136]
	fmt.Println("   ->", btc.NewUint256(hdr[4:36]).String())
}


func main() {
	flag.BoolVar(&fl_help, "h", false, "Show help")
	flag.UintVar(&fl_block, "block", 0, "Print hash(es) of the given block height")
	flag.BoolVar(&fl_scan, "scan", false, "Scan database for first extra blocks")
	flag.StringVar(&fl_dir, "dir", "", "Use blockdb from this directory")
	flag.StringVar(&fl_split, "split", "", "Split blockdb at this block's hash")
	flag.UintVar(&fl_skip, "skip", 0, "Skip this many blocks when splitting")
	flag.StringVar(&fl_append, "append", "", "Append blocks from this folder to the database")
	flag.BoolVar(&fl_trunc, "trunc", false, "Truncate insted of splitting")

	flag.Parse()

	if fl_help {
		flag.PrintDefaults()
		return
	}

	if fl_dir!="" && fl_dir[len(fl_dir)-1]!=os.PathSeparator {
		fl_dir += string(os.PathSeparator)
	}

	if fl_append!="" {
		if fl_append[len(fl_append)-1]!=os.PathSeparator {
			fl_append += string(os.PathSeparator)
		}
		fmt.Println("Loading", fl_append+"blockchain.new")
		dat, er := ioutil.ReadFile(fl_append+"blockchain.new")
		if er != nil {
			fmt.Println(er.Error())
			return
		}

		f, er := os.Open(fl_append+"blockchain.dat")
		if er != nil {
			fmt.Println(er.Error())
			return
		}

		fo, er := os.OpenFile(fl_dir+"blockchain.dat", os.O_RDWR, 0600)
		if er != nil {
			f.Close()
			fmt.Println(er.Error())
			return
		}
		datfilelen, _ := fo.Seek(0, os.SEEK_END)

		fmt.Println("Appending", datfilelen, "bytes to blockchain.dat")
		for {
			var buf [1024*1024]byte
			n, _ := f.Read(buf[:])
			if n>0 {
				fo.Write(buf[:n])
			}
			if n!=len(buf) {
				break
			}
		}
		fo.Close()
		f.Close()

		fmt.Println("OK. Now appending", len(dat)/136, "records to blockchain.new")
		fo, er = os.OpenFile(fl_dir+"blockchain.new", os.O_RDWR, 0600)
		if er != nil {
			f.Close()
			fmt.Println(er.Error())
			return
		}

		for off:=0; off<len(dat); off+=136 {
			sl := dat[off:off+136]
			newoffs := binary.LittleEndian.Uint64(sl[40:48]) + uint64(datfilelen)
			binary.LittleEndian.PutUint64(sl[40:48], newoffs)
			fo.Write(sl)
		}
		fo.Close()

		return
	}

	fmt.Println("Loading", fl_dir+"blockchain.new")
	dat, er := ioutil.ReadFile(fl_dir+"blockchain.new")
	if er != nil {
		fmt.Println(er.Error())
		return
	}

	fmt.Println(len(dat)/136, "records")
	if fl_scan {
		last_bl_height := binary.LittleEndian.Uint32(dat[36:40])
		fmt.Println("Scanning database for first extra block(s)...")
		fmt.Println("First block in the file has height", last_bl_height)
		for off:=136; off<len(dat); off+=136 {
			sl := dat[off:off+136]
			height := binary.LittleEndian.Uint32(sl[36:40])

			if height!=last_bl_height+1 {
				fmt.Println("Unexpected", height, last_bl_height+1, "found at offset", off)
				print_record(dat[off-136:off])
				print_record(dat[off:off+136])
				fmt.Println()
			}
			last_bl_height = height
		}
		return
	}

	if fl_block!=0 {
		for off:=0; off<len(dat); off+=136 {
			sl := dat[off:off+136]
			height := binary.LittleEndian.Uint32(sl[36:40])
			if uint(height)==fl_block {
				print_record(dat[off:off+136])
			}
		}
		return
	}

	if fl_split!="" {
		th := btc.NewUint256FromString(fl_split)
		if th==nil {
			println("incorrect block hash")
			return
		}
		for off:=0; off<len(dat); off+=136 {
			sl := dat[off:off+136]
			height := binary.LittleEndian.Uint32(sl[36:40])
			bh := btc.NewSha2Hash(sl[56:136])
			if bh.Hash==th.Hash {
				trunc_dat_offs := int64(binary.LittleEndian.Uint64(sl[40:48]))
				fmt.Println("Truncate blockchain.new at offset", off)
				fmt.Println("Truncate blockchain.dat at offset", trunc_dat_offs)
				if !fl_trunc {
					new_dir := fl_dir + fmt.Sprint(height) + string(os.PathSeparator)
					os.Mkdir(new_dir, os.ModePerm)

					f, e := os.Open(fl_dir+"blockchain.dat")
					if e != nil {
						fmt.Println(e.Error())
						return
					}
					df, e := os.Create(new_dir+"blockchain.dat")
					if e != nil {
						f.Close()
						fmt.Println(e.Error())
						return
					}

					f.Seek(trunc_dat_offs, os.SEEK_SET)

					fmt.Println("But fist save the rest in", new_dir, "...")
					if fl_skip!=0 {
						fmt.Println("Skip", fl_skip, "blocks in the output file")
						for fl_skip>0 {
							skipbytes := binary.LittleEndian.Uint32(sl[48:52])
							fmt.Println(" -", skipbytes, "bytes of block", binary.LittleEndian.Uint32(sl[36:40]))
							off+=136
							if off<len(dat) {
								sl = dat[off:off+136]
								fl_skip--
							} else {
								break
							}
						}
					}

					for {
						var buf [1024*1024]byte
						n, _ := f.Read(buf[:])
						if n>0 {
							df.Write(buf[:n])
						}
						if n!=len(buf) {
							break
						}
					}
					df.Close()
					f.Close()

					df, e = os.Create(new_dir+"blockchain.new")
					if e != nil {
						f.Close()
						fmt.Println(e.Error())
						return
					}
					var off2 int
					for off2=off; off2<len(dat); off2+=136 {
						sl := dat[off2:off2+136]
						newoffs := binary.LittleEndian.Uint64(sl[40:48]) - uint64(trunc_dat_offs)
						binary.LittleEndian.PutUint64(sl[40:48], newoffs)
						df.Write(sl)
					}
					df.Close()
				}

				os.Truncate(fl_dir+"blockchain.new", int64(off))
				os.Truncate(fl_dir+"blockchain.dat", trunc_dat_offs)
				return
			}
		}
		fmt.Println("Block not found - nothing truncated")
	}

	var minbh, maxbh uint32
	minbh = binary.LittleEndian.Uint32(dat[36:40])
	maxbh = minbh
	for off:=136; off<len(dat); off+=136 {
		sl := dat[off:off+136]
		bh := binary.LittleEndian.Uint32(sl[36:40])
		if bh>maxbh {
			maxbh = bh
		} else if bh<maxbh {
			minbh = bh
		}
	}
	fmt.Println("Block heights from", minbh, "to", maxbh)
}
