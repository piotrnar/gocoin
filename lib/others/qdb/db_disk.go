// Copyright 2009 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

/*
Qdb is a fast persistent storage database.

The records are binary blobs that can have a variable length, up to 4GB.

The key must be a unique 64-bit value, most likely a hash of the actual key.

They data is stored on a disk, in a folder specified during the call to NewDB().
There are can be three possible files in that folder
 * qdb.0, qdb.1 - these files store a compact version of the entire database
 * qdb.log - this one stores the changes since the most recent qdb.0 or qdb.1

*/
package qdb

import (
	"os"
	"io"
	"fmt"
	"strconv"
	"path/filepath"
	"encoding/binary"
)


func (db *DB) seq2fn(seq uint32) string {
	return fmt.Sprintf("%s%08x.dat", db.Dir, seq)
}

func (db *DB) checklogfile() {
	// If could not open, create it
	if db.LogFile == nil {
		fn := db.seq2fn(db.DataSeq)
		db.LogFile, _ = os.Create(fn)
		binary.Write(db.LogFile, binary.LittleEndian, uint32(db.DataSeq))
		db.LastValidLogPos = 4
	}
}


// load record from disk, if not loaded yet
func (db *DB) loadrec(idx *oneIdx) {
	if idx.data == nil {
		var f *os.File
		if f, _ = db.DatFiles[idx.DataSeq]; f==nil {
			fn := db.seq2fn(idx.DataSeq)
			f, _ = os.Open(fn)
			if f==nil {
				println("file", fn, "not found")
				os.Exit(1)
			}
			db.DatFiles[idx.DataSeq] = f
		}
		idx.LoadData(f)
	}
}

// add record at the end of the log
func (db *DB) addtolog(f io.Writer, key KeyType, val []byte) (fpos int64) {
	if f==nil {
		db.checklogfile()
		db.LogFile.Seek(db.LastValidLogPos, os.SEEK_SET)
		f = db.LogFile
	}

	fpos = db.LastValidLogPos
	f.Write(val)
	db.LastValidLogPos += int64(len(val)) // 4 bytes for CRC

	return
}

// add record at the end of the log
func (db *DB) cleanupold(used map[uint32]bool) {
	filepath.Walk(db.Dir, func(path string, info os.FileInfo, err error) error {
		fn := info.Name()
		if len(fn)==12 && fn[8:12]==".dat" {
			v, er := strconv.ParseUint(fn[:8], 16, 32)
			if er == nil && uint32(v)!=db.DataSeq {
				if _, ok := used[uint32(v)]; !ok {
					//println("deleting", v, path)
					if f, _ := db.DatFiles[uint32(v)]; f!=nil {
						f.Close()
						delete(db.DatFiles, uint32(v))
					}
					os.Remove(path)
				}
			}
		}
		return nil
	})
}
