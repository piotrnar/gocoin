package btc

import (
	"crypto/sha256"
	"math/big"
	"bytes"
	"os"
	"fmt"
)

func ShaSum(b []byte) (out [32]byte) {
	s := sha256.New()
	s.Write(b[:])
	copy(out[:], s.Sum(nil))
	return
}

func Sha2Sum(b []byte) (out [32]byte) {
	s := sha256.New()
	s.Write(b[:])
	tmp := s.Sum(nil)
	s.Reset()
	s.Write(tmp)
	copy(out[:], s.Sum(nil))
	return
}

func AddressAsB58(a []byte) (s string) {
	const CHRS = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"
	bn0 := big.NewInt(0)
	bn58 := big.NewInt(58)
	bn := big.NewInt(0).SetBytes(a)
	var mo *big.Int
	for bn.Cmp(bn0) != 0 {
		bn, mo = new(big.Int).DivMod(bn, bn58, new(big.Int))
		s = string(CHRS[mo.Int64()]) + s
	}
	s = "1" + s
	return
}

func lsb2uint(lt []byte) (res uint64) {
	for i:=0; i<len(lt); i++ {
		res |= (uint64(lt[i]) << uint(i*8))
	}
	return
}

func msb2uint(lt []byte) (res uint64) {
	for i:=0; i<len(lt); i++ {
		res = (res<<8) | uint64(lt[i])
	}
	return
}

func getVlenVal(b *bytes.Buffer) (res uint64) {
	c, e := b.ReadByte()
	errorFatal(e, "GetVarLen first")
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

func getVlenData(b *bytes.Buffer) (res []byte, le uint64) {
	var buf [9]byte
	_, e := b.Read(buf[:1])
	errorFatal(e, "getVlenData first")
	if buf[0] < 0xfd {
		le = uint64(buf[0])
		res = buf[:1]
		return
	}

	c := 2 << (2-(0xff-buf[0]));
	_, e = b.Read(buf[1:c+1])
	errorFatal(e, "getVlenData remaining")
	
	res = buf[:c+1]
	for i:=0; i<c; i++ {
		le |= (uint64(buf[i+1]) << uint64(8*i))
	}
	return
}

func getVlen(buf []byte) (le uint64, size uint32) {
	if buf[0] < 0xfd {
		le = uint64(buf[0])
		size = 1
		return
	}

	size = 1 + (2 << (2-(0xff-buf[0])));
	le = lsb2uint(buf[1:size])
	
	var i uint32
	for i=0; i<size-1; i++ {
		le |= (uint64(buf[i+1]) << uint64(8*i))
	}
	return
}

func allzeros(b []byte) bool {
	for i := range b {
		if b[i]!=0 {
			return false
		}
	}
	return true
}


func errorFatal(er error, s string) {
	if er != nil {
		println(s+":", er.Error())
		os.Exit(1)
	}
}

func GetBlockReward(height uint32) (uint64) {
	return 50e8 >> (height/210000)
}


func bin2hex(mem []byte) string {
	var s string
	for i := 0; i<len(mem); i++ {
		s+= fmt.Sprintf("%02x", mem[i])
	}
	return s
}


func read32bit(f *os.File) (v uint32, e error) {
	var b [4]byte
	_, e = f.Read(b[:])
	if e == nil {
		v = uint32(msb2uint(b[:]))
	}
	return 
}


func write32bit(f *os.File, v uint32) {
	var b [4]byte
	b[0] = byte(v>>24)
	b[1] = byte(v>>16)
	b[2] = byte(v>>8)
	b[3] = byte(v)
	f.Write(b[:])
}


func read64bit(f *os.File) (v uint64, e error) {
	var b [8]byte
	_, e = f.Read(b[:])
	if e == nil {
		v = msb2uint(b[:])
	}
	return 
}


func write64bit(f *os.File, v uint64) {
	var b [8]byte
	b[0] = byte(v>>56)
	b[1] = byte(v>>48)
	b[2] = byte(v>>40)
	b[3] = byte(v>>32)
	b[4] = byte(v>>24)
	b[5] = byte(v>>16)
	b[6] = byte(v>>8)
	b[7] = byte(v)
	f.Write(b[:])
}

func put32lsb(b []byte, v uint32) uint32 {
	b[3] = byte(v>>24)
	b[2] = byte(v>>16)
	b[1] = byte(v>>8)
	b[0] = byte(v)
	return 4
}

func put64lsb(b []byte, v uint64) uint32 {
	b[7] = byte(v>>56)
	b[6] = byte(v>>48)
	b[5] = byte(v>>40)
	b[4] = byte(v>>32)
	b[3] = byte(v>>24)
	b[2] = byte(v>>16)
	b[1] = byte(v>>8)
	b[0] = byte(v)
	return 8
}

func putVlen(b []byte, vl int) uint32 {
	if (vl>=0 && vl<0xfd) {
		b[0] = byte(vl)
		return 1
	}
	println("putVlen only supports small number now")
	os.Exit(1)
	return 1
}

func getfilepos(f *os.File) (p int64) {
	p, _ = f.Seek(0, os.SEEK_CUR)
	return
}
