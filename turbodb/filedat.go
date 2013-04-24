package turbodb

import (
	"os"
    "fmt"
	"errors"
	"encoding/binary"
)

// Opens file and checks the ffffffff-sequence-FINI marker at the end
func openAndGetSeq(fn string) (f *os.File, seq uint32) {
	var b [12]byte
	var e error
	var fpos int64
	
	if f, e = os.Open(fn); e != nil {
		return
	}
	
	if fpos, e = f.Seek(-12, os.SEEK_END); e!=nil || fpos<4 {
		f.Close()
		f = nil
		return
	}

	if _, e = f.Read(b[:]); e != nil {
		f.Close()
		f = nil
		return
	}

	if binary.LittleEndian.Uint32(b[0:4])!=0xffffffff || string(b[8:12])!="FINI" {
		f.Close()
		f = nil
		return
	}

	seq = binary.LittleEndian.Uint32(b[4:8])
	return
}


// allocate the cache map and loads it from disk
func (db *TurboDB) loadfiledat() (e error) {
	var ks uint32

	db.Cache = make(map[[KeySize]byte] []byte)

	f, seq := openAndGetSeq(db.pathname+"0")
	f1, seq1 := openAndGetSeq(db.pathname+"1")
	
	if f == nil && f1 == nil {
		e = errors.New("No database")
		return
	}

	if f!=nil && f1!=nil {
		// Both files are valid - take the one with higher sequence
		if int32(seq - seq1) >= 0 {
			f1.Close()
			os.Remove(db.pathname+"1")
			db.file_index = 0
		} else {
			f.Close()
			f = f1
			os.Remove(db.pathname+"0")
			db.file_index = 1
		}
	} else if f==nil {
		f = f1
		seq = seq1
		db.file_index = 1
	} else {
		db.file_index = 0
	}

	readlimit, _ := f.Seek(-12, os.SEEK_END)
	f.Seek(0, os.SEEK_SET)

	db.version_seq = seq

	e = binary.Read(f, binary.LittleEndian, &ks)
	if e != nil || ks != KeySize {
		f.Close()
		e = errors.New("Incompatible key size")
		os.Remove(db.pathname+"0")
		os.Remove(db.pathname+"1")
		return
	}

	var key [KeySize]byte
	filepos := int64(4)
	for filepos+KeySize+4 <= readlimit {
		_, e = f.Read(key[:])
		if e != nil {
			break
		}
		e = binary.Read(f, binary.LittleEndian, &ks)
		if e != nil {
			break
		}
		val := make([]byte, ks)
		_, e = f.Read(val[:])
		if e != nil {
			break
		}
		db.Cache[key] = val
		filepos += int64(KeySize+4+ks)
	}

	f.Close()
	return
}


func (db *TurboDB) savefiledat() (e error) {
	cnt := 0
	var f *os.File
	new_file_index := 1 - db.file_index
	fname := fmt.Sprint(db.pathname, new_file_index)
	
	f, e = os.Create(fname)
	if e != nil {
		return
	}
	e = binary.Write(f, binary.LittleEndian, uint32(KeySize))
	if e != nil {
		goto close_and_clean
	}

	for k, v := range db.Cache {
		_, e = f.Write(k[:])
		if e != nil {
			goto close_and_clean
		}
		e = binary.Write(f, binary.LittleEndian, uint32(len(v)))
		if e != nil {
			goto close_and_clean
		}
		_, e = f.Write(v[:])
		if e != nil {
			goto close_and_clean
		}
		cnt++
	}

	_, e = f.Write([]byte{0xff,0xff,0xff,0xff})
	if e != nil {
		goto close_and_clean
	}

	e = binary.Write(f, binary.LittleEndian, uint32(db.version_seq+1))
	if e != nil {
		goto close_and_clean
	}

	_, e = f.Write([]byte("FINI"))
	if e != nil {
		goto close_and_clean
	}

	os.Remove(fmt.Sprint(db.pathname, db.file_index))
	os.Remove(db.pathname+"log")
	
	db.version_seq++
	db.file_index = new_file_index
	
	return

close_and_clean:
	f.Close()
	os.Remove(fname)
	return
}
