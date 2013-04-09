package btc

import (
	"bytes"
)

func ReadUint32(rd *bytes.Reader) (res uint32, e error) {
	var b [4]byte
	_, e = rd.Read(b[:])
	if e == nil {
		res = uint32(b[0])
		res |= uint32(b[1])<<8
		res |= uint32(b[2])<<16
		res |= uint32(b[3])<<24
	}
	return
}


func ReadUint64(rd *bytes.Reader) (res uint64, e error) {
	var b [8]byte
	_, e = rd.Read(b[:])
	if e == nil {
		res = uint64(b[0])
		res |= uint64(b[1])<<8
		res |= uint64(b[2])<<16
		res |= uint64(b[3])<<24
		res |= uint64(b[4])<<32
		res |= uint64(b[5])<<40
		res |= uint64(b[6])<<48
		res |= uint64(b[7])<<56
	}
	return
}


func ReadVLen64(b *bytes.Reader) (res uint64, e error) {
	var c byte
	c, e = b.ReadByte()
	if e != nil {
		return
	}
	if c < 0xfd {
		res = uint64(c)
		return
	}

	var buf [8]byte;
	c = 2 << (2-(0xff-c));

	_, e = b.Read(buf[:c])
	errorFatal(e, "GetVarLen second")
	var i byte
	for i=0; i<c; i++ {
		res |= (uint64(buf[i]) << uint64(8*i))
	}
	return
}

