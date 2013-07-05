package qdb

import (
	"os"
	"io"
	"fmt"
	"io/ioutil"
	"encoding/binary"
)


// Opens file and checks the ffffffff-sequence-FINI marker at the end
func read_and_check_file(fn string) (seq uint32, data []byte) {
	var le int
	var d []byte
	var f *os.File

	f, _ = os.Open(fn)
	if f == nil {
		return
	}

	d, _ = ioutil.ReadAll(f)
	f.Close()

	if d == nil {
		println(fn, "could not read file")
		return
	}

	le = len(d)
	if le < 16 {
		println(fn, "len", le)
		return
	}

	if string(d[le-4:le])!="FINI" {
		println(fn, "no FINI")
		return
	}

	if binary.LittleEndian.Uint32(d[le-12:le-8])!=0xFFFFFFFF {
		println(fn, "no FFFFFFFF")
		return
	}

	seq = binary.LittleEndian.Uint32(d[0:4])
	if seq != binary.LittleEndian.Uint32(d[le-8:le-4]) {
		println(fn, "seq mismatch", seq, binary.LittleEndian.Uint32(d[le-8:le-4]))
		return
	}

	data = d
	return
}


func (idx *dbidx) loadneweridx() []byte {
	s0, d0 := read_and_check_file(idx.path+"0")
	s1, d1 := read_and_check_file(idx.path+"1")

	if d0 == nil && d1 == nil {
		//println(idx.path, "- no valid file")
		return nil
	}

	if d0!=nil && d1!=nil {
		// Both files are valid - take the one with higher sequence
		if int32(s0 - s1) >= 0 {
			os.Remove(idx.path+"1")
			idx.datfile_idx = 0
			idx.version_seq = s0
			return d0
		} else {
			os.Remove(idx.path+"0")
			idx.datfile_idx = 1
			idx.version_seq = s1
			return d1
		}
	} else if d0==nil {
		os.Remove(idx.path+"0")
		idx.datfile_idx = 1
		idx.version_seq = s1
		return d1
	} else {
		os.Remove(idx.path+"1")
		idx.datfile_idx = 0
		idx.version_seq = s0
		return d0
	}
}


func (idx *dbidx) loaddat(used map[uint32]bool) {
	d := idx.loadneweridx()
	if d == nil {
		return
	}

	for pos:=4; pos+24<=len(d)-12; pos+=24 {
		key := KeyType(binary.LittleEndian.Uint64(d[pos:pos+8]))
		fpos := binary.LittleEndian.Uint32(d[pos+8:pos+12])
		flen := binary.LittleEndian.Uint32(d[pos+12:pos+16])
		fseq := binary.LittleEndian.Uint32(d[pos+16:pos+20])
		flgz := binary.LittleEndian.Uint32(d[pos+20:pos+24])
		idx.memput(key, &oneIdx{datpos:fpos, datlen:flen, datseq:fseq, flags:flgz})
		used[fseq] = true
	}
	return
}


func (idx *dbidx) loadlog(used map[uint32]bool) {
	idx.logfile, _ = os.OpenFile(idx.path+"log", os.O_RDWR, 0660)
	if idx.logfile==nil {
		return
	}

	var iseq uint32
	binary.Read(idx.logfile, binary.LittleEndian, &iseq)
	if iseq!=idx.version_seq {
		println("incorrect seq in the log file", iseq, idx.version_seq)
		idx.logfile.Close()
		idx.logfile = nil
		os.Remove(idx.path+"log")
		return
	}

	d, _ := ioutil.ReadAll(idx.logfile)
	for pos:=0; pos+12<=len(d); {
		key := KeyType(binary.LittleEndian.Uint64(d[pos:pos+8]))
		fpos := binary.LittleEndian.Uint32(d[pos+8:pos+12])
		pos += 12
		if fpos!=0 {
			if pos+12>len(d) {
				println("Unexpected END of file")
				break
			}
			flen := binary.LittleEndian.Uint32(d[pos:pos+4])
			fseq := binary.LittleEndian.Uint32(d[pos+4:pos+8])
			flgz := binary.LittleEndian.Uint32(d[pos+8:pos+12])
			pos += 12
			idx.memput(key, &oneIdx{datpos:fpos, datlen:flen, datseq:fseq, flags:flgz})
			used[fseq] = true
		} else {
			idx.memdel(key)
		}
	}

	return
}


func (idx *dbidx) checklogfile() {
	if idx.logfile == nil {
		idx.logfile, _ = os.Create(idx.path+"log")
		binary.Write(idx.logfile, binary.LittleEndian, uint32(idx.version_seq))
	}
	return
}


func (idx *dbidx) addtolog(wr io.Writer, k KeyType, rec *oneIdx) {
	if wr == nil {
		idx.checklogfile()
		wr = idx.logfile
	}
	binary.Write(wr, binary.LittleEndian, k)
	binary.Write(wr, binary.LittleEndian, rec.datpos)
	binary.Write(wr, binary.LittleEndian, rec.datlen)
	binary.Write(wr, binary.LittleEndian, rec.datseq)
	binary.Write(wr, binary.LittleEndian, rec.flags)
}


func (idx *dbidx) deltolog(wr io.Writer, k KeyType) {
	if wr == nil {
		idx.checklogfile()
		wr = idx.logfile
	}
	binary.Write(wr, binary.LittleEndian, k)
	wr.Write([]byte{0,0,0,0})
}


func (idx *dbidx) writedatfile() {
	idx.datfile_idx = 1-idx.datfile_idx
	idx.version_seq++
	f, _ := os.Create(fmt.Sprint(idx.path, idx.datfile_idx))
	binary.Write(f, binary.LittleEndian, idx.version_seq)
	idx.browse(func(key KeyType, rec *oneIdx) bool {
		binary.Write(f, binary.LittleEndian, key)
		binary.Write(f, binary.LittleEndian, rec.datpos)
		binary.Write(f, binary.LittleEndian, rec.datlen)
		binary.Write(f, binary.LittleEndian, rec.datseq)
		binary.Write(f, binary.LittleEndian, rec.flags)
		return true
	})
	f.Write([]byte{0xff,0xff,0xff,0xff})
	binary.Write(f, binary.LittleEndian, idx.version_seq)
	f.Write([]byte("FINI"))
	f.Close()

	// now delete the previous log
	if idx.logfile!=nil {
		idx.logfile.Close()
		idx.logfile = nil
	}
	os.Remove(idx.path+"log")
	os.Remove(fmt.Sprint(idx.path, 1-idx.datfile_idx))
}


func (idx *dbidx) writebuf(d []byte) {
	idx.checklogfile()
	idx.logfile.Write(d)
}
