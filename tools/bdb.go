package main

import (
	"sync"
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"flag"
	"fmt"
	"github.com/golang/snappy"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"io/ioutil"
	"os"
	"os/signal"
)

/*
	blockchain.dat - contains raw blocks data, no headers, nothing
	blockchain.new - contains records of 136 bytes (all values LSB):
		[0] - flags:
			bit(0) - "trusted" flag - this block's scripts have been verified
			bit(1) - "invalid" flag - this block's scripts have failed
			bit(2) - "compressed" flag - this block's data is compressed
			bit(3) - "snappy" flag - this block is compressed with snappy (not gzip'ed)
			bit(4) - if this bit is set, bytes [32:36] carry length of uncompressed block
			bit(5) - if this bit is set, bytes [28:32] carry data file index

		Used to be:
		[4:36]  - 256-bit block hash - DEPRECATED! (hash the header to get the value)

		[4:28] - reserved
		[28:32] - specifies which blockchain.dat file is used (if not zero, the filename is: blockchain-%08x.dat)
		[32:36] - length of uncompressed block

		[36:40] - 32-bit block height (genesis is 0)
		[40:48] - 64-bit block pos in blockchain.dat file
		[48:52] - 32-bit block lenght in bytes
		[52:56] - 32-bit number of transaction in the block
		[56:136] - 80 bytes blocks header
*/

const (
	TRUSTED = 0x01
	INVALID = 0x02
)

var (
	fl_help              bool
	fl_block, fl_stop    uint
	fl_dir               string
	fl_scan, fl_defrag   bool
	fl_split             string
	fl_skip              uint
	fl_append            string
	fl_trunc             bool
	fl_commit, fl_verify bool
	fl_savebl            string
	fl_purgeall          bool
	fl_purgeto           uint
	fl_flasg             bool
	fl_from, fl_to       uint
	fl_trusted           int
	fl_invalid           int
	fl_fixlen            bool
	fl_fixlenall         bool

	fl_mergedat          uint
	fl_movedat           uint

	fl_splitdat          int
	fl_mb                uint

	fl_datidx            int

	fl_purgedatidx       bool

	buf [5*1024*1024]byte // 5MB should be anough
)

/********************************************************/
type one_idx_rec struct {
	sl []byte
	hash [32]byte
}

func new_sl(sl []byte) (r one_idx_rec) {
	r.sl = sl[:136]
	btc.ShaHash(sl[56:136], r.hash[:])
	return
}

func (r one_idx_rec) Flags() uint32 {
	return binary.LittleEndian.Uint32(r.sl[0:4])
}

func (r one_idx_rec) Height() uint32 {
	return binary.LittleEndian.Uint32(r.sl[36:40])
}

func (r one_idx_rec) DPos() uint64 {
	return binary.LittleEndian.Uint64(r.sl[40:48])
}

func (r one_idx_rec) SetDPos(dp uint64) {
	binary.LittleEndian.PutUint64(r.sl[40:48], dp)
}

func (r one_idx_rec) DLen() uint32 {
	return binary.LittleEndian.Uint32(r.sl[48:52])
}

func (r one_idx_rec) SetDLen(l uint32) {
	binary.LittleEndian.PutUint32(r.sl[48:52], l)
}

func (r one_idx_rec) SetDatIdx(l uint32) {
	r.sl[0] |= 0x20
	binary.LittleEndian.PutUint32(r.sl[28:32], l)
}

func (r one_idx_rec) Hash() []byte {
	return r.hash[:]
}

func (r one_idx_rec) HIdx() (h [32]byte) {
	copy(h[:], r.hash[:])
	return
}

func (r one_idx_rec) Parent() []byte {
	return r.sl[60:92]
}

func (r one_idx_rec) PIdx() [32]byte {
	var h [32]byte
	copy(h[:], r.sl[60:92])
	return h
}

func (r one_idx_rec) DatIdx() uint32 {
	if (r.sl[0]&0x20) != 0 {
		return binary.LittleEndian.Uint32(r.sl[28:32])
	}
	return 0
}


/********************************************************/

type one_tree_node struct {
	off int // offset in teh idx file
	one_idx_rec
	parent *one_tree_node
	next   *one_tree_node
}

/********************************************************/

func print_record(sl []byte) {
	bh := btc.NewSha2Hash(sl[56:136])
	fmt.Println("Block", bh.String())
	fmt.Println(" ... Height", binary.LittleEndian.Uint32(sl[36:40]),
		" Flags", fmt.Sprintf("0x%02x", sl[0]),
		" - ", binary.LittleEndian.Uint32(sl[48:52]), "bytes @",
		binary.LittleEndian.Uint64(sl[40:48]), "in DAT")
	if (sl[0]&0x10) != 0 {
		fmt.Println("     Uncompressed length:",
			binary.LittleEndian.Uint32(sl[32:36]), "bytes")
	}
	if (sl[0]&0x20) != 0 {
		fmt.Println("     Data file index:",
			binary.LittleEndian.Uint32(sl[28:32]))
	}
	hdr := sl[56:136]
	fmt.Println("   ->", btc.NewUint256(hdr[4:36]).String())
}


func verify_block(blk []byte, sl one_idx_rec, off int) {
	bl, er := btc.NewBlock(blk)
	if er != nil {
		println("verify_block at off", off, er.Error())
		return
	}
	if !bytes.Equal(bl.Hash.Hash[:], sl.Hash()) {
		println("verify_block at off", off, "Header invalid")
		return
	}

	er = bl.BuildTxList()
	if er != nil {
		println("verify_block at off", off, er.Error())
		return
	}

	merk, _ := bl.GetMerkle()
	if !bytes.Equal(bl.MerkleRoot(), merk) {
		println("verify_block at off", off, "Payload invalid")
		return
	}
}

func decomp_block(fl uint32, buf []byte) (blk []byte) {
	if (fl & chain.BLOCK_COMPRSD)!=0 {
		if (fl & chain.BLOCK_SNAPPED) != 0 {
			blk, _ = snappy.Decode(nil, buf)
		} else {
			gz, _ := gzip.NewReader(bytes.NewReader(buf))
			blk, _ = ioutil.ReadAll(gz)
			gz.Close()
		}
	} else {
		blk = buf
	}
	return
}

// Look for the first and last records with the given index
func look_for_range(dat []byte, _idx uint32) (min_valid_off, max_valid_off int) {
	min_valid_off = -1
	for off := 0; off < len(dat); off += 136 {
		sl := new_sl(dat[off:])
		idx := sl.DatIdx()
		if sl.DLen() > 0 {
			if idx == _idx {
				if min_valid_off == -1 {
					min_valid_off = off
				}
				max_valid_off = off
			} else if min_valid_off != -1 {
				break
			}
		}
	}
	return
}

func dat_fname(idx uint32) string {
	if idx == 0 {
		return "blockchain.dat"
	}
	return fmt.Sprintf("blockchain-%08x.dat", idx)
}

func split_the_data_file(parent_f *os.File, idx uint32, maxlen uint64, dat []byte, min_valid_off, max_valid_off int) bool {
	fname := dat_fname(idx)

	if fi, _ := os.Stat(fname); fi != nil {
		fmt.Println(fi.Name(), "exist - get rid of it first")
		return false
	}

	rec_from := new_sl(dat[min_valid_off:min_valid_off+136])
	pos_from := rec_from.DPos()

	for off := min_valid_off; off <= max_valid_off; off += 136 {
		rec := new_sl(dat[off:off+136])
		if rec.DLen()==0 {
			continue
		}
		dpos := rec.DPos()
		if dpos - pos_from + uint64(rec.DLen()) > maxlen {
			if !split_the_data_file(parent_f, idx+1, maxlen, dat, off, max_valid_off) {
				return false // abort spliting
			}
			//println("truncate parent at", dpos)
			er := parent_f.Truncate(int64(dpos))
			if er != nil {
				println(er.Error())
			}
			max_valid_off = off-136
			break // go to the next stage
		}
	}

	// at this point parent_f should be truncated
	f, er := os.Create(fname)
	if er != nil {
		fmt.Println(er.Error())
		return false
	}

	parent_f.Seek(int64(pos_from), os.SEEK_SET)
	for {
		n, _ := parent_f.Read(buf[:])
		if n > 0 {
			f.Write(buf[:n])
		}
		if n != len(buf) {
			break
		}
	}

	//println(".. child split", fname, "at offs", min_valid_off/136, "...", max_valid_off/136, "fpos:", pos_from, " maxlen:", maxlen)
	for off := min_valid_off; off <= max_valid_off; off += 136 {
		sl := new_sl(dat[off:off+136])
		sl.SetDatIdx(idx)
		sl.SetDPos(sl.DPos() - pos_from)
	}
	// flush blockchain.new to disk wicth each noe split for safety
	ioutil.WriteFile("blockchain.tmp", dat, 0600)
	os.Rename("blockchain.tmp", "blockchain.new")

	return true
}



func main() {
	flag.BoolVar(&fl_help, "h", false, "Show help")
	flag.UintVar(&fl_block, "block", 0, "Print details of the given block number (or start -verify from it)")
	flag.BoolVar(&fl_scan, "scan", false, "Scan database for first extra blocks")
	flag.BoolVar(&fl_defrag, "defrag", false, "Purge all the orphaned blocks")
	flag.UintVar(&fl_stop, "stop", 0, "Stop after so many scan errors")
	flag.StringVar(&fl_dir, "dir", "", "Use blockdb from this directory")
	flag.StringVar(&fl_split, "split", "", "Split blockdb at this block's hash")
	flag.UintVar(&fl_skip, "skip", 0, "Skip this many blocks when splitting")
	flag.StringVar(&fl_append, "append", "", "Append blocks from this folder to the database")
	flag.BoolVar(&fl_trunc, "trunc", false, "Truncate insted of splitting")
	flag.BoolVar(&fl_commit, "commit", false, "Optimize the size of the data file")
	flag.BoolVar(&fl_verify, "verify", false, "Verify each block inside the database")
	flag.StringVar(&fl_savebl, "savebl", "", "Save block with the given hash to disk")
	flag.BoolVar(&fl_purgeall, "purgeall", false, "Purge all blocks from the database")
	flag.UintVar(&fl_purgeto, "purgeto", 0, "Purge all blocks till (but excluding) the given height")

	flag.UintVar(&fl_from, "from", 0, "Set/clear flag from this block")
	flag.UintVar(&fl_to, "to", 0xffffffff, "Set/clear flag to this block or merge/rename into this data file index")
	flag.IntVar(&fl_invalid, "invalid", -1, "Set (1) or clear (0) INVALID flag")
	flag.IntVar(&fl_trusted, "trusted", -1, "Set (1) or clear (0) TRUSTED flag")

	flag.BoolVar(&fl_fixlen, "fixlen", false, "Calculate (fix) orignial length of last 144 blocks")
	flag.BoolVar(&fl_fixlenall, "fixlenall", false, "Calculate (fix) orignial length of each block")

	flag.UintVar(&fl_mergedat, "mergedat", 0, "Merge this data file index into the data file specified by -to <idx>")
	flag.UintVar(&fl_movedat, "movedat", 0, "Rename this data file index into the data file specified by -to <idx>")

	flag.IntVar(&fl_splitdat, "splitdat", -1, "Split this data file into smaller parts (-mb <mb>)")
	flag.UintVar(&fl_mb, "mb", 1000, "Split big data file into smaller parts of this size in MB (at least 8 MB)")

	flag.IntVar(&fl_datidx, "datidx", -1, "Show records with the specific data file index")

	flag.BoolVar(&fl_purgedatidx, "purgedatidx", false, "Remove reerence to dat files which are not on disk")

	flag.Parse()

	if fl_help {
		flag.PrintDefaults()
		return
	}

	if fl_dir != "" && fl_dir[len(fl_dir)-1] != os.PathSeparator {
		fl_dir += string(os.PathSeparator)
	}

	if fl_append != "" {
		if fl_append[len(fl_append)-1] != os.PathSeparator {
			fl_append += string(os.PathSeparator)
		}
		fmt.Println("Loading", fl_append+"blockchain.new")
		dat, er := ioutil.ReadFile(fl_append + "blockchain.new")
		if er != nil {
			fmt.Println(er.Error())
			return
		}

		f, er := os.Open(fl_append + "blockchain.dat")
		if er != nil {
			fmt.Println(er.Error())
			return
		}

		fo, er := os.OpenFile(fl_dir+"blockchain.dat", os.O_WRONLY, 0600)
		if er != nil {
			f.Close()
			fmt.Println(er.Error())
			return
		}
		datfilelen, _ := fo.Seek(0, os.SEEK_END)

		fmt.Println("Appending blocks data to blockchain.dat")
		for {
			n, _ := f.Read(buf[:])
			if n > 0 {
				fo.Write(buf[:n])
			}
			if n != len(buf) {
				break
			}
		}
		fo.Close()
		f.Close()

		fmt.Println("Now appending", len(dat)/136, "records to blockchain.new")
		fo, er = os.OpenFile(fl_dir+"blockchain.new", os.O_WRONLY, 0600)
		if er != nil {
			f.Close()
			fmt.Println(er.Error())
			return
		}
		fo.Seek(0, os.SEEK_END)

		for off := 0; off < len(dat); off += 136 {
			sl := dat[off : off+136]
			newoffs := binary.LittleEndian.Uint64(sl[40:48]) + uint64(datfilelen)
			binary.LittleEndian.PutUint64(sl[40:48], newoffs)
			fo.Write(sl)
		}
		fo.Close()

		return
	}

	fmt.Println("Loading", fl_dir+"blockchain.new")
	dat, er := ioutil.ReadFile(fl_dir + "blockchain.new")
	if er != nil {
		fmt.Println(er.Error())
		return
	}

	fmt.Println(len(dat)/136, "records")

	if fl_mergedat != 0 {
		if fl_to >= fl_mergedat {
			fmt.Println("To index must be lower than from index")
			return
		}
		min_valid_from, max_valid_from := look_for_range(dat, uint32(fl_mergedat))
		if min_valid_from==-1 {
			fmt.Println("Invalid from index")
			return
		}

		from_fn := dat_fname(uint32(fl_mergedat))
		to_fn := dat_fname(uint32(fl_to))

		f, er := os.Open(from_fn)
		if er != nil {
			fmt.Println(er.Error())
			return
		}

		fo, er := os.OpenFile(to_fn, os.O_WRONLY, 0600)
		if er != nil {
			f.Close()
			fmt.Println(er.Error())
			return
		}
		offset_to_add, _ := fo.Seek(0, os.SEEK_END)

		fmt.Println("Appending", from_fn, "to", to_fn, "at offset", offset_to_add)
		for {
			n, _ := f.Read(buf[:])
			if n > 0 {
				fo.Write(buf[:n])
			}
			if n != len(buf) {
				break
			}
		}
		fo.Close()
		f.Close()

		var cnt int
		for off := min_valid_from; off <= max_valid_from; off += 136 {
			sl := dat[off : off+136]
			fpos := binary.LittleEndian.Uint64(sl[40:48])
			fpos += uint64(offset_to_add)
			binary.LittleEndian.PutUint64(sl[40:48], fpos)
			sl[0] |= 0x20
			binary.LittleEndian.PutUint32(sl[28:32], uint32(fl_to))
			cnt++
		}
		ioutil.WriteFile("blockchain.tmp", dat, 0600)
		os.Rename("blockchain.tmp", "blockchain.new")
		os.Remove(from_fn)
		fmt.Println(from_fn, "removed and", cnt, "records updated in blockchain.new")
		return
	}

	if fl_movedat != 0 {
		if fl_to == fl_movedat {
			fmt.Println("To index must be different than from index")
			return
		}
		min_valid, max_valid := look_for_range(dat, uint32(fl_movedat))
		if min_valid==-1 {
			fmt.Println("Invalid from index")
			return
		}
		to_fn := dat_fname(uint32(fl_to))

		if fi, _ := os.Stat(to_fn); fi != nil {
			fmt.Println(fi.Name(), "exist - get rid of it first")
			return
		}

		from_fn := dat_fname(uint32(fl_movedat))

		// first discard all the records with the target index
		for off := 0; off < len(dat); off += 136 {
			rec := new_sl(dat[off : off+136])
			if rec.DatIdx()==uint32(fl_to) {
				rec.SetDLen(0)
				rec.SetDatIdx(0xffffffff)
			}
		}

		// now set the new index
		var cnt int
		for off := min_valid; off <= max_valid; off += 136 {
			sl := dat[off : off+136]
			sl[0] |= 0x20
			binary.LittleEndian.PutUint32(sl[28:32], uint32(fl_to))
			cnt++
		}
		ioutil.WriteFile("blockchain.tmp", dat, 0600)
		os.Rename(from_fn, to_fn)
		os.Rename("blockchain.tmp", "blockchain.new")
		fmt.Println(from_fn, "renamed to ", to_fn, "and", cnt, "records updated in blockchain.new")
		return
	}

	if fl_splitdat >= 0 {
		if fl_mb < 8 {
			fmt.Println("Minimal value of -mb parameter is 8")
			return
		}
		fname := dat_fname(uint32(fl_splitdat))
		fmt.Println("Spliting file", fname, "into chunks - up to", fl_mb, "MB...")
		min_valid_off, max_valid_off := look_for_range(dat, uint32(fl_splitdat))
		f, er := os.OpenFile(fname, os.O_RDWR, 0600)
		if er != nil {
			fmt.Println(er.Error())
			return
		}
		defer f.Close()
		//fmt.Println("Range:", min_valid_off/136, "...", max_valid_off/136)

		maxlen := uint64(fl_mb) << 20
		for off := min_valid_off; off <= max_valid_off; off += 136 {
			rec := new_sl(dat[off:off+136])
			if rec.DLen()==0 {
				continue
			}
			dpos := rec.DPos()
			if dpos + uint64(rec.DLen()) > maxlen {
				//println("root split from", dpos)
				if !split_the_data_file(f, uint32(fl_splitdat)+1, maxlen, dat, off, max_valid_off) {
					fmt.Println("Splitting failed")
					return
				}
				f.Truncate(int64(dpos))
				fmt.Println("Splitting succeeded")
				return
			}
		}
		fmt.Println("There was nothing to split")
		return
	}

	if fl_datidx >= 0 {
		fname := dat_fname(uint32(fl_datidx))
		min_valid_off, max_valid_off := look_for_range(dat, uint32(fl_datidx))
		if min_valid_off==-1 {
			fmt.Println(fname, "is not used by any record")
			return
		}
		fmt.Println(fname, "is used by", (max_valid_off-min_valid_off)/136+1, "records. From", min_valid_off/136, "to", max_valid_off/136)
		fmt.Println("Block height from", new_sl(dat[min_valid_off:]).Height(), "to", new_sl(dat[max_valid_off:]).Height())
		return
	}

	if fl_purgedatidx {
		cache := make(map[uint32]bool)
		var cnt int
		for off := 0; off < len(dat); off += 136 {
			rec := new_sl(dat[off:])
			if rec.DLen()==0 && rec.DatIdx()==0xffffffff {
				continue
			}
			idx := rec.DatIdx()
			have_file, ok := cache[idx]
			if !ok {
				fi, _ := os.Stat(dat_fname(idx))
				have_file = fi!=nil
				cache[idx] = have_file
			}
			if !have_file {
				rec.SetDatIdx(0xffffffff)
				rec.SetDLen(0)
				cnt++
			}
		}
		if cnt > 0 {
			ioutil.WriteFile("blockchain.tmp", dat, 0600)
			os.Rename("blockchain.tmp", "blockchain.new")
			fmt.Println(cnt, "records removed from blockchain.new")
		} else {
			fmt.Println("Data files seem consisent - no need to remove anything")
		}
		return
	}


	if fl_invalid==0 || fl_invalid==1 || fl_trusted==0 || fl_trusted==1 {
		var cnt uint64
		for off := 0; off < len(dat); off += 136 {
			sl := dat[off : off+136]
			if uint(binary.LittleEndian.Uint32(sl[36:40])) < fl_from {
				continue
			}
			if uint(binary.LittleEndian.Uint32(sl[36:40])) > fl_to {
				continue
			}
			if fl_invalid==0 {
				if (sl[0]&INVALID)!=0 {
					sl[0] &= ^byte(INVALID)
					cnt++
				}
			} else if fl_invalid==1 {
				if (sl[0]&INVALID)==0 {
					sl[0] |= INVALID
					cnt++
				}
			}
			if fl_trusted==0 {
				if (sl[0]&TRUSTED)!=0 {
					sl[0] &= ^byte(TRUSTED)
					cnt++
				}
			} else if fl_trusted==1 {
				if (sl[0]&TRUSTED)==0 {
					sl[0] |= TRUSTED
					cnt++
				}
			}
		}
		ioutil.WriteFile("blockchain.tmp", dat, 0600)
		os.Rename("blockchain.tmp", "blockchain.new")
		fmt.Println(cnt, "flags updated in blockchain.new")
	}

	if fl_purgeall {
		for off := 0; off < len(dat); off += 136 {
			sl := dat[off : off+136]
			binary.LittleEndian.PutUint64(sl[40:48], 0)
			binary.LittleEndian.PutUint32(sl[48:52], 0)
		}
		ioutil.WriteFile("blockchain.tmp", dat, 0600)
		os.Rename("blockchain.tmp", "blockchain.new")
		fmt.Println("blockchain.new upated. Now delete blockchain.dat yourself...")
	}

	if fl_purgeto!=0 {
		var cur_dat_pos uint64

		f, er := os.Open("blockchain.dat")
		if er!=nil {
			println(er.Error())
			return
		}
		defer f.Close()

		newdir := fmt.Sprint("purged_to_", fl_purgeto, string(os.PathSeparator))
		os.Mkdir(newdir, os.ModePerm)

		o, er := os.Create(newdir+"blockchain.dat")
		if er!=nil {
			println(er.Error())
			return
		}
		defer o.Close()

		for off := 0; off < len(dat); off += 136 {
			sl := new_sl(dat[off : off+136])

			if uint(sl.Height()) < fl_purgeto {
				sl.SetDLen(0)
				sl.SetDPos(0)
			} else {
				blen := int(sl.DLen())
				f.Seek(int64(sl.DPos()), os.SEEK_SET)
				er = btc.ReadAll(f, buf[:blen])
				if er != nil {
					println(er.Error())
					return
				}
				sl.SetDPos(cur_dat_pos)
				cur_dat_pos += uint64(blen)
				o.Write(buf[:blen])
			}
		}
		ioutil.WriteFile(newdir+"blockchain.new", dat, 0600)
		return
	}

	if fl_scan {
		var scan_errs uint
		last_bl_height := binary.LittleEndian.Uint32(dat[36:40])
		exp_offset := uint64(binary.LittleEndian.Uint32(dat[48:52]))
		fmt.Println("Scanning database for first extra block(s)...")
		fmt.Println("First block in the file has height", last_bl_height)
		for off := 136; off < len(dat); off += 136 {
			sl := dat[off : off+136]
			height := binary.LittleEndian.Uint32(sl[36:40])
			off_in_bl := binary.LittleEndian.Uint64(sl[40:48])

			if height != last_bl_height+1 {
				fmt.Println("Out of sequence block number", height, last_bl_height+1, "found at offset", off)
				print_record(dat[off-136 : off])
				print_record(dat[off : off+136])
				fmt.Println()
				scan_errs++
			}
			if off_in_bl != exp_offset {
				fmt.Println("Spare data found just before block number", height, off_in_bl, exp_offset)
				print_record(dat[off-136 : off])
				print_record(dat[off : off+136])
				scan_errs++
			}

			if fl_stop != 0 && scan_errs >= fl_stop {
				break
			}

			last_bl_height = height

			exp_offset += uint64(binary.LittleEndian.Uint32(sl[48:52]))
		}
		return
	}

	if fl_defrag {
		blks := make(map[[32]byte]*one_tree_node, len(dat)/136)
		for off := 0; off < len(dat); off += 136 {
			sl := new_sl(dat[off : off+136])
			blks[sl.HIdx()] = &one_tree_node{off: off, one_idx_rec: sl}
		}
		var maxbl uint32
		var maxblptr *one_tree_node
		for _, v := range blks {
			v.parent = blks[v.PIdx()]
			h := v.Height()
			if h > maxbl {
				maxbl = h
				maxblptr = v
			} else if h == maxbl {
				maxblptr = nil
			}
		}
		fmt.Println("Max block height =", maxbl)
		if maxblptr == nil {
			fmt.Println("More than one block at maximum height - cannot continue")
			return
		}
		used := make(map[[32]byte]bool)
		var first_block *one_tree_node
		var total_data_size uint64
		for n := maxblptr; n != nil; n = n.parent {
			if n.parent != nil {
				n.parent.next = n
			}
			used[n.PIdx()] = true
			if first_block == nil || first_block.Height() > n.Height() {
				first_block = n
			}
			total_data_size += uint64(n.DLen())
		}
		if len(used) < len(blks) {
			fmt.Println("Purge", len(blks)-len(used), "blocks from the index file...")
			f, e := os.Create(fl_dir + "blockchain.tmp")
			if e != nil {
				println(e.Error())
				return
			}
			var off int
			for n := first_block; n != nil; n = n.next {
				n.off = off
				n.sl[0] = n.sl[0]&0xfc
				f.Write(n.sl)
				off += len(n.sl)
			}
			f.Close()
			os.Rename(fl_dir+"blockchain.tmp", fl_dir+"blockchain.new")
		} else {
			fmt.Println("The index file looks perfect")
		}

		for n := first_block; n!=nil && n.next!=nil; n = n.next {
			if n.next.DPos() < n.DPos() {
				fmt.Println("There is a problem... swapped order in the data file!", n.off)
				return
			}
		}

		fdat, er := os.OpenFile(fl_dir+"blockchain.dat", os.O_RDWR, 0600)
		if er != nil {
			println(er.Error())
			return
		}

		if fl, _ := fdat.Seek(0, os.SEEK_END); uint64(fl) == total_data_size {
			fdat.Close()
			fmt.Println("All good - blockchain.dat has an optimal length")
			return
		}

		if !fl_commit {
			fdat.Close()
			fmt.Println("Warning: blockchain.dat shall be defragmented. Use \"-defrag -commit\"")
			return
		}

		fidx, er := os.OpenFile(fl_dir+"blockchain.new", os.O_RDWR, 0600)
		if er != nil {
			println(er.Error())
			return
		}

		// Capture Ctrl+C
		killchan := make(chan os.Signal, 1)
		signal.Notify(killchan, os.Interrupt, os.Kill)

		var doff uint64
		var prv_perc uint64 = 101
		for n := first_block; n != nil; n = n.next {
			perc := 1000 * doff / total_data_size
			dp := n.DPos()
			dl := n.DLen()
			if perc != prv_perc {
				fmt.Printf("\rDefragmenting data file - %.1f%% (%d bytes saved so far)...",
					float64(perc)/10.0, dp-doff)
				prv_perc = perc
			}
			if dp > doff {
				fdat.Seek(int64(dp), os.SEEK_SET)
				fdat.Read(buf[:int(dl)])

				n.SetDPos(doff)

				fdat.Seek(int64(doff), os.SEEK_SET)
				fdat.Write(buf[:int(dl)])

				fidx.Seek(int64(n.off), os.SEEK_SET)
				fidx.Write(n.sl)
			}
			doff += uint64(dl)

			select {
			case <-killchan:
				fmt.Println("interrupted")
				fidx.Close()
				fdat.Close()
				fmt.Println("Database closed - should be still usable, but no space saved")
				return
			default:
			}
		}

		fidx.Close()
		fdat.Close()
		fmt.Println()

		fmt.Println("Truncating blockchain.dat at position", doff)
		os.Truncate(fl_dir+"blockchain.dat", int64(doff))

		return
	}

	if fl_verify {
		var prv_perc int = -1
		var totlen uint64
		var done sync.WaitGroup
		var dat_file_open uint32 = 0xffffffff
		var fdat *os.File
		var cnt, cnt_nd, cnt_err int

		for off := 0; off < len(dat); off += 136 {
			sl := new_sl(dat[off : off+136])

			le := int(sl.DLen())
			if le == 0 {
				continue
			}

			dp := int64(sl.DPos())
			hei := uint(sl.Height())

			idx := sl.DatIdx()
			if idx == 0xffffffff {
				continue
			}

			if idx != dat_file_open {
				var er error
				dat_file_open = idx
				if fdat != nil {
					fdat.Close()
				}
				fdat, er = os.OpenFile(fl_dir + dat_fname(idx), os.O_RDWR, 0600)
				if er != nil {
					//println(er.Error())
					continue
				}
			}

			perc := 1000 * off / len(dat)
			if perc != prv_perc {
				fmt.Printf("\rVerifying blocks data - %.1f%% @ %d / %dMB processed...",
					float64(perc)/10.0, hei, totlen>>20)
				prv_perc = perc
			}

			if fl_block!=0 && hei<fl_block {
				continue
			}

			fdat.Seek(dp, os.SEEK_SET)
			n, _ := fdat.Read(buf[:le])
			if n != le {
				//fmt.Println("Block", hei, "not in dat file", idx, dp)
				cnt_nd++
				continue
			}

			blk := decomp_block(sl.Flags(), buf[:le])
			if blk == nil {
				fmt.Println("Block", hei, "decompression failed")
				cnt_err++
				continue
			}

			done.Add(1)
			go func(blk []byte, sl one_idx_rec, off int) {
				verify_block(blk, sl, off)
				done.Done()
				cnt++
			}(blk, sl, off)

			totlen += uint64(len(blk))
		}
		done.Wait()  // wait for all the goroutines to complete
		fdat.Close()
		if fdat != nil {
			fdat.Close()
		}
		fmt.Println("\nAll blocks done -", totlen>>20, "MB and", cnt, "blocks verified OK")
		fmt.Println("No data errors:", cnt_nd, "  Decompression errors:", cnt_err)
		return
	}

	if fl_block != 0 {
		for off := 0; off < len(dat); off += 136 {
			sl := dat[off : off+136]
			height := binary.LittleEndian.Uint32(sl[36:40])
			if uint(height) == fl_block {
				print_record(dat[off : off+136])
			}
		}
		return
	}

	if fl_split != "" {
		th := btc.NewUint256FromString(fl_split)
		if th == nil {
			println("incorrect block hash")
			return
		}
		for off := 0; off < len(dat); off += 136 {
			sl := dat[off : off+136]
			height := binary.LittleEndian.Uint32(sl[36:40])
			bh := btc.NewSha2Hash(sl[56:136])
			if bh.Hash == th.Hash {
				trunc_idx_offs := int64(off)
				trunc_dat_offs := int64(binary.LittleEndian.Uint64(sl[40:48]))
				fmt.Println("Truncate blockchain.new at offset", trunc_idx_offs)
				fmt.Println("Truncate blockchain.dat at offset", trunc_dat_offs)
				if !fl_trunc {
					new_dir := fl_dir + fmt.Sprint(height) + string(os.PathSeparator)
					os.Mkdir(new_dir, os.ModePerm)

					f, e := os.Open(fl_dir + "blockchain.dat")
					if e != nil {
						fmt.Println(e.Error())
						return
					}
					df, e := os.Create(new_dir + "blockchain.dat")
					if e != nil {
						f.Close()
						fmt.Println(e.Error())
						return
					}

					f.Seek(trunc_dat_offs, os.SEEK_SET)

					fmt.Println("But fist save the rest in", new_dir, "...")
					if fl_skip != 0 {
						fmt.Println("Skip", fl_skip, "blocks in the output file")
						for fl_skip > 0 {
							skipbytes := binary.LittleEndian.Uint32(sl[48:52])
							fmt.Println(" -", skipbytes, "bytes of block", binary.LittleEndian.Uint32(sl[36:40]))
							off += 136
							if off < len(dat) {
								sl = dat[off : off+136]
								fl_skip--
							} else {
								break
							}
						}
					}

					for {
						n, _ := f.Read(buf[:])
						if n > 0 {
							df.Write(buf[:n])
						}
						if n != len(buf) {
							break
						}
					}
					df.Close()
					f.Close()

					df, e = os.Create(new_dir + "blockchain.new")
					if e != nil {
						f.Close()
						fmt.Println(e.Error())
						return
					}
					var off2 int
					for off2 = off; off2 < len(dat); off2 += 136 {
						sl := dat[off2 : off2+136]
						newoffs := binary.LittleEndian.Uint64(sl[40:48]) - uint64(trunc_dat_offs)
						binary.LittleEndian.PutUint64(sl[40:48], newoffs)
						df.Write(sl)
					}
					df.Close()
				}

				os.Truncate(fl_dir+"blockchain.new", trunc_idx_offs)
				os.Truncate(fl_dir+"blockchain.dat", trunc_dat_offs)
				return
			}
		}
		fmt.Println("Block not found - nothing truncated")
	}

	if fl_savebl!="" {
		bh := btc.NewUint256FromString(fl_savebl)
		if bh == nil {
			println("Incortrect block hash:", fl_savebl)
			return
		}
		for off := 0; off < len(dat); off += 136 {
			sl := new_sl(dat[off : off+136])
			if bytes.Equal(sl.Hash(), bh.Hash[:]) {
				f, er := os.Open(fl_dir+dat_fname(sl.DatIdx()))
				if er != nil {
					println(er.Error())
					return
				}
				bu := buf[:int(sl.DLen())]
				f.Seek(int64(sl.DPos()), os.SEEK_SET)
				f.Read(bu)
				f.Close()
				ioutil.WriteFile(bh.String()+".bin", decomp_block(sl.Flags(), bu), 0600)
				fmt.Println(bh.String()+".bin written to disk. It has height", sl.Height())
				return
			}
		}
		fmt.Println("Block", bh.String(), "not found in the database")
		return
	}

	if fl_fixlen || fl_fixlenall {
		fdat, er := os.OpenFile(fl_dir+"blockchain.dat", os.O_RDWR, 0600)
		if er != nil {
			println(er.Error())
			return
		}

		dat_file_size, _ := fdat.Seek(0, os.SEEK_END)

		var prv_perc int64 = -1
		var totlen uint64
		var off int
		if !fl_fixlenall {
			off = len(dat) - 144*136
		}
		for ; off < len(dat); off += 136 {
			sl := new_sl(dat[off : off+136])
			olen := binary.LittleEndian.Uint32(sl.sl[32:36])
			if olen == 0 {
				sl := new_sl(dat[off : off+136])
				dp := int64(sl.DPos())
				le := int(sl.DLen())

				perc := 1000 * dp / dat_file_size
				if perc != prv_perc {
					fmt.Printf("\rUpdating blocks length - %.1f%% / %dMB processed...",
						float64(perc)/10.0, totlen>>20)
					prv_perc = perc
				}

				fdat.Seek(dp, os.SEEK_SET)
				fdat.Read(buf[:le])
				blk := decomp_block(sl.Flags(), buf[:le])
				binary.LittleEndian.PutUint32(sl.sl[32:36], uint32(len(blk)))
				sl.sl[0] |= 0x10

				totlen += uint64(len(blk))
			}
		}
		ioutil.WriteFile("blockchain.tmp", dat, 0600)
		os.Rename("blockchain.tmp", "blockchain.new")
		fmt.Println("blockchain.new updated")
	}

	var minbh, maxbh, valididx, validlen uint32
	minbh = binary.LittleEndian.Uint32(dat[36:40])
	maxbh = minbh
	for off := 136; off < len(dat); off += 136 {
		sl := new_sl(dat[off : off+136])
		bh := sl.Height()
		if bh > maxbh {
			maxbh = bh
		} else if bh < minbh {
			minbh = bh
		}
		if sl.DatIdx() != 0xffffffff {
			valididx++
		}
		if sl.DLen() != 0 {
			validlen++
		}
	}
	fmt.Println("Block heights from", minbh, "to", maxbh)
	fmt.Println("Number of records with valid length:", validlen)
	fmt.Println("Number of records with valid data file:", valididx)
}
