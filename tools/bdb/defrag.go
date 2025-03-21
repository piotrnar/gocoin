package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"syscall"

	"github.com/piotrnar/gocoin/lib/btc"
)

var (
	fl_defrag  bool
	fl_tophash string
)

type one_tree_node struct {
	off int // offset in teh idx file
	one_idx_rec
	parent *one_tree_node
	next   *one_tree_node
}

func do_data_file(idx uint32, first_block *one_tree_node) {
	var total_data_size uint64
	dat_filename := dat_fname(idx)

	for n := first_block; n != nil; n = n.next {
		if n.DatIdx() != idx {
			continue
		}
		if n.next != nil && n.next.DatIdx() == idx && n.next.DPos() < n.DPos() {
			fmt.Println("There is a problem... swapped order in the data file!", n.off)
			return
		}
		total_data_size += uint64(n.DLen())
	}

	fdat, er := os.OpenFile(fl_dir+dat_filename, os.O_RDWR, 0600)
	if er != nil {
		println(er.Error())
		return
	}

	if fl, _ := fdat.Seek(0, io.SeekEnd); uint64(fl) == total_data_size {
		fdat.Close()
		fmt.Println("All good -", dat_filename, "has an optimal length")
		return
	} else {
		fmt.Println(dat_filename, "is", fl, "bytes, but should be", total_data_size)
	}

	if !fl_commit {
		fdat.Close()
		fmt.Println("Warning:", dat_filename, "shall be defragmented. Use \"-defrag -commit\"")
		return
	}

	fidx, er := os.OpenFile(fl_dir+"blockchain.new", os.O_RDWR, 0600)
	if er != nil {
		println(er.Error())
		return
	}

	// Capture Ctrl+C
	killchan := make(chan os.Signal, 1)
	signal.Notify(killchan, os.Interrupt, syscall.SIGTERM)

	var doff uint64
	var prv_perc uint64 = 101
	for n := first_block; n != nil; n = n.next {
		if n.DatIdx() != idx {
			continue
		}
		perc := 1000 * doff / total_data_size
		dp := n.DPos()
		dl := n.DLen()
		if perc != prv_perc {
			fmt.Printf("\rDefragmenting %s - %.1f%% (%d bytes saved so far)...",
				dat_filename, float64(perc)/10.0, dp-doff)
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

	fmt.Println("Truncating", dat_filename, "at position", doff)
	os.Truncate(fl_dir+dat_filename, int64(doff))
}

func do_defrag(dat []byte) {
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
		fmt.Println("More than one block at maximum height")
		if fl_tophash == "" {
			fmt.Println("Use -top <hash> to select one of the top blocks:")
			for bh, v := range blks {
				if v.Height() == maxbl {
					fmt.Println(" ", btc.NewUint256(bh[:]).String())
				}
			}
			return
		}
		bh := btc.NewUint256FromString(fl_tophash)
		if bh == nil {
			println("-top parameter is not a valid uint256")
			os.Exit(1)
		}
		if maxblptr = blks[bh.Hash]; maxblptr == nil {
			println("-top parameter is does not point to an existing block")
			os.Exit(1)
		}
		maxbl = maxblptr.Height()
		fmt.Println("Max block height to use =", maxbl)
	}
	used := make(map[[32]byte]bool)
	var first_block *one_tree_node
	for n := maxblptr; n != nil; n = n.parent {
		if n.parent != nil {
			n.parent.next = n
		}
		used[n.PIdx()] = true
		if first_block == nil || first_block.Height() > n.Height() {
			first_block = n
		}
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
			n.sl[0] = n.sl[0] & 0xfc
			f.Write(n.sl)
			off += len(n.sl)
		}
		f.Close()
		os.Rename(fl_dir+"blockchain.tmp", fl_dir+"blockchain.new")
		fmt.Println(fl_dir+"blockchain.new", "updated")
	} else {
		fmt.Println("The index file looks perfect")
	}

	fidxs := make(map[uint32]int)
	for n := first_block; n != nil; n = n.next {
		fidxs[n.DatIdx()]++
	}
	idxs := make([]uint32, 0, len(fidxs))
	for ind := range fidxs {
		idxs = append(idxs, ind)
	}
	slices.Sort(idxs)
	for _, ind := range idxs {
		do_data_file(ind, first_block)
	}
}
