package qdb

import (
	"os"
	"io"
	"fmt"
	//"bytes"
	"bufio"
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


func (idx *QdbIndex) loadneweridx() []byte {
	s0, d0 := read_and_check_file(idx.IdxFilePath+"0")
	s1, d1 := read_and_check_file(idx.IdxFilePath+"1")

	if d0 == nil && d1 == nil {
		//println(idx.IdxFilePath, "- no valid file")
		return nil
	}

	if d0!=nil && d1!=nil {
		// Both files are valid - take the one with higher sequence
		if int32(s0 - s1) >= 0 {
			os.Remove(idx.IdxFilePath+"1")
			idx.DatfileIndex = 0
			idx.VersionSequence = s0
			return d0
		} else {
			os.Remove(idx.IdxFilePath+"0")
			idx.DatfileIndex = 1
			idx.VersionSequence = s1
			return d1
		}
	} else if d0==nil {
		os.Remove(idx.IdxFilePath+"0")
		idx.DatfileIndex = 1
		idx.VersionSequence = s1
		return d1
	} else {
		os.Remove(idx.IdxFilePath+"1")
		idx.DatfileIndex = 0
		idx.VersionSequence = s0
		return d0
	}
}


func (idx *QdbIndex) loaddat(used map[uint32]bool) {
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
		idx.memput(key, &oneIdx{datpos:fpos, datlen:flen, DataSeq:fseq, flags:flgz})
		used[fseq] = true
	}
	return
}


func (idx *QdbIndex) loadlog(used map[uint32]bool) {
	idx.file, _ = os.OpenFile(idx.IdxFilePath+"log", os.O_RDWR, 0660)
	if idx.file==nil {
		return
	}

	var iseq uint32
	binary.Read(idx.file, binary.LittleEndian, &iseq)
	if iseq!=idx.VersionSequence {
		println("incorrect seq in the log file", iseq, idx.VersionSequence)
		idx.file.Close()
		idx.file = nil
		os.Remove(idx.IdxFilePath+"log")
		return
	}

	d, _ := ioutil.ReadAll(idx.file)
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
			idx.memput(key, &oneIdx{datpos:fpos, datlen:flen, DataSeq:fseq, flags:flgz})
			used[fseq] = true
		} else {
			idx.memdel(key)
		}
	}

	return
}


func (idx *QdbIndex) checklogfile() {
	if idx.file == nil {
		idx.file, _ = os.Create(idx.IdxFilePath+"log")
		binary.Write(idx.file, binary.LittleEndian, uint32(idx.VersionSequence))
	}
	return
}


func (idx *QdbIndex) addtolog(wr io.Writer, k KeyType, rec *oneIdx) {
	if wr == nil {
		idx.checklogfile()
		wr = idx.file
	}
	binary.Write(wr, binary.LittleEndian, k)
	binary.Write(wr, binary.LittleEndian, rec.datpos)
	binary.Write(wr, binary.LittleEndian, rec.datlen)
	binary.Write(wr, binary.LittleEndian, rec.DataSeq)
	binary.Write(wr, binary.LittleEndian, rec.flags)
}


func (idx *QdbIndex) deltolog(wr io.Writer, k KeyType) {
	if wr == nil {
		idx.checklogfile()
		wr = idx.file
	}
	binary.Write(wr, binary.LittleEndian, k)
	wr.Write([]byte{0,0,0,0})
}


func (idx *QdbIndex) writedatfile() {
	idx.DatfileIndex = 1-idx.DatfileIndex
	idx.VersionSequence++

	//f := new(bytes.Buffer)
	ff, _ := os.Create(fmt.Sprint(idx.IdxFilePath, idx.DatfileIndex))
	f := bufio.NewWriterSize(ff, 0x100000)
	binary.Write(f, binary.LittleEndian, idx.VersionSequence)
	idx.browse(func(key KeyType, rec *oneIdx) bool {
		binary.Write(f, binary.LittleEndian, key)
		binary.Write(f, binary.LittleEndian, rec.datpos)
		binary.Write(f, binary.LittleEndian, rec.datlen)
		binary.Write(f, binary.LittleEndian, rec.DataSeq)
		binary.Write(f, binary.LittleEndian, rec.flags)
		return true
	})
	f.Write([]byte{0xff,0xff,0xff,0xff})
	binary.Write(f, binary.LittleEndian, idx.VersionSequence)
	f.Write([]byte("FINI"))

	//ioutil.WriteFile(fmt.Sprint(idx.IdxFilePath, idx.DatfileIndex), f.Bytes(), 0600)
	f.Flush()
	ff.Close()

	// now delete the previous log
	if idx.file!=nil {
		idx.file.Close()
		idx.file = nil
	}
	os.Remove(idx.IdxFilePath+"log")
	os.Remove(fmt.Sprint(idx.IdxFilePath, 1-idx.DatfileIndex))
}


func (idx *QdbIndex) writebuf(d []byte) {
	idx.checklogfile()
	idx.file.Write(d)
}
