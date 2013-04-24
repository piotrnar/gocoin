package main

import (
	"bytes"
	"fmt"
	"os"
)

const (
	Version = 32500
)

var Services = []byte{0x01,0x00,0x00,0x00,0x00,0x00,0x00,0x00}

func dbgmem(mem []byte){
	var i int
	for i = 0; i<len(mem); i++ {
		fmt.Printf(" %02x", mem[i]);
		if (i&31)==31 {
			fmt.Printf("\n");
		}
	}
	if (i&31) != 0 {
		fmt.Printf("\n");
	}
}


func bin2hex(mem []byte) string {
	var s string
	for i := 0; i<len(mem); i++ {
		s+= fmt.Sprintf("%02x", mem[i])
	}
	return s
}


func hash2str(h [32]byte) (s string) {
	for i := 0; i<32; i++ {
		s+= fmt.Sprintf("%02x", h[31-i])
	}
	return
}


func WriteLSB (b *bytes.Buffer, v uint64, nbyt int) {
	for i := 0; i<nbyt; i++ {
		b.WriteByte(uint8(v))
		v>>= 8;
	}
}

func ReadLSB (b *bytes.Buffer, nbyt int) (ret uint64) {
	for i := 0; i<nbyt; i++ {
		v, e := b.ReadByte()
		if e != nil {
			println("ReadLSB:", e.Error())
			os.Exit(1);
		}
		ret |= uint64(v) << uint(8*i)
	}
	return
}


func GetVarLen(b *bytes.Buffer) (res uint64) {
	c, e := b.ReadByte()
	if e != nil {
		println("GetVarLen:", e.Error())
		os.Exit(1)
	}
	if c < 0xfd {
		res = uint64(c)
		return
	}

	var buf [8]byte;
	c = 2 << (2-(0xff-c));

	_, e = b.Read(buf[:c])
	if e != nil {
		println("GetVarLen2:", e.Error())
		os.Exit(1)
	}
	var i byte
	for i=0; i<c; i++ {
		res |= (uint64(buf[i]) << uint64(8*i))
	}
	return
}

