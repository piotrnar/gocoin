package btc

import (
	"os"
	"io"
	"fmt"
	"errors"
	"strings"
	"strconv"
	"encoding/base64"
	"encoding/binary"
)

func allzeros(b []byte) bool {
	for i := range b {
		if b[i]!=0 {
			return false
		}
	}
	return true
}


// Puts var_length field into the given buffer
func PutVlen(b []byte, vl int) uint32 {
	uvl := uint32(vl)
	if uvl<0xfd {
		b[0] = byte(uvl)
		return 1
	}
	if uvl<0x10000 {
		b[0] = 0xfd
		b[1] = byte(uvl)
		b[2] = byte(uvl>>8)
		return 3
	}
	b[0] = 0xfe
	b[1] = byte(uvl)
	b[2] = byte(uvl>>8)
	b[3] = byte(uvl>>16)
	b[4] = byte(uvl>>24)
	return 5
}


// Returns length and number of bytes that the var_int took
// If there is not enough bytes in the buffer 0, 0 gets returned
func VLen(b []byte) (le int, var_int_siz int) {
	if len(b)==0 {
		return // better to quit with zeros than to cause a panic
	}
	c := b[0]
	if c < 0xfd {
		return int(c), 1
	}
	var_int_siz = 1 + (2 << (2-(0xff-c)))
	if len(b)<1+var_int_siz {
		return // better to quit with zeros than to cause a panic
	}

	var res uint64
	for i:=1; i<var_int_siz; i++ {
		res |= (uint64(b[i]) << uint64(8*(i-1)))
	}

	if res>0x7fffffff {
		panic("This should never happen")
	}

	le = int(res)
	return
}


func GetMerkel(txs []*Tx) ([]byte) {
	mtr := make([][]byte, len(txs))
	for i := range txs {
		mtr[i] = txs[i].Hash.Hash[:]
	}
	var j, i2 int
	for siz:=len(txs); siz>1; siz=(siz+1)/2 {
		for i := 0; i < siz; i += 2 {
			if i+1 < siz-1 {
				i2 = i+1
			} else {
				i2 = siz-1
			}
			h := Sha2Sum(append(mtr[j+i], mtr[j+i2]...))
			mtr = append(mtr, h[:])
		}
		j += siz
	}
	return mtr[len(mtr)-1]
}


// Reads var_len from the given reader
func ReadVLen(b io.Reader) (res uint64, e error) {
	var buf [8]byte;
	var n int

	n, e = b.Read(buf[:1])
	if e != nil {
		//println("ReadVLen1 error:", e.Error())
		return
	}

	if n != 1 {
		e = errors.New("Buffer empty")
		return
	}

	if buf[0] < 0xfd {
		res = uint64(buf[0])
		return
	}

	c := 2 << (2-(0xff-buf[0]));

	n, e = b.Read(buf[:c])
	if e != nil {
		println("ReadVLen1 error:", e.Error())
		return
	}
	if n != c {
		e = errors.New("Buffer too short")
		return
	}
	for i:=0; i<c; i++ {
		res |= (uint64(buf[i]) << uint64(8*i))
	}
	return
}

// Writes var_length field into the given writer
func WriteVlen(b io.Writer, var_len uint32) {
	if var_len < 0xfd {
		b.Write([]byte{byte(var_len)})
		return
	}
	if var_len < 0x10000 {
		b.Write([]byte{0xfd, byte(var_len), byte(var_len>>8)})
		return
	}
	b.Write([]byte{0xfe, byte(var_len), byte(var_len>>8), byte(var_len>>16), byte(var_len>>24)})
}

// Writes opcode to put a specific number of bytes to stack
func WritePutLen(b io.Writer, data_len uint32) {
	switch {
		case data_len <= OP_PUSHDATA1:
			b.Write([]byte{byte(data_len)})

		case data_len < 0x100:
			b.Write([]byte{OP_PUSHDATA1,byte(data_len)})

		case data_len < 0x10000:
			b.Write([]byte{OP_PUSHDATA2})
			binary.Write(b, binary.LittleEndian, uint16(data_len))

		default:
			b.Write([]byte{OP_PUSHDATA4})
			binary.Write(b, binary.LittleEndian, uint32(data_len))
	}
}


// Read bitcoin protocol string
func ReadString(rd io.Reader) (s string, e error) {
	var le uint64
	le, e = ReadVLen(rd)
	if e != nil {
		return
	}
	bu := make([]byte, le)
	_, e = rd.Read(bu)
	if e == nil {
		s = string(bu)
	}
	return
}


// Takes a base64 encoded bitcoin generated signature and decodes it
func ParseMessageSignature(encsig string) (nv byte, sig *Signature, er error) {
	var sd []byte

	sd, er = base64.StdEncoding.DecodeString(encsig)
	if er != nil {
		return
	}

	if len(sd)!=65 {
		er = errors.New("The decoded signature is not 65 bytes long")
		return
	}

	nv = sd[0]

	sig = new(Signature)
	sig.R.SetBytes(sd[1:33])
	sig.S.SetBytes(sd[33:65])

	if nv<27 || nv>34 {
		er = errors.New("nv out of range")
	}

	return
}


func IsPayToScript(scr []byte) bool {
	return len(scr)==23 && scr[0]==OP_HASH160 && scr[1]==0x14 && scr[22]==OP_EQUAL
}

func IsPushOnly(scr []byte) bool {
	idx := 0
	for idx<len(scr) {
		op, _, n, e := GetOpcode(scr[idx:])
		if e != nil {
			return false
		}
		if op > OP_16 {
			return false
		}
		idx += n
	}
	return true
}

// Parses a floating number string to return uint64 value expressed in Satoshi's
// Using strconv.ParseFloat followed by uint64(val*1e8) is not precise enough.
func StringToSatoshis(s string) (val uint64, er error) {
	var big, small uint64

	ss := strings.Split(s, ".")
	if len(ss)==1 {
		val, er = strconv.ParseUint(ss[0], 10, 64)
		if er != nil {
			return
		}
		val *= 1e8
		return
	}
	if len(ss)!=2 {
		println("Incorrect amount", s)
		os.Exit(1)
	}

	if len(ss[1])>8 {
		er = errors.New("Too many decimal points")
		return
	}
	if len(ss[1])<8 {
		ss[1] += strings.Repeat("0", 8-len(ss[1]))
	}

	small, er = strconv.ParseUint(ss[1], 10, 64)
	if er != nil {
		return
	}

	big, er = strconv.ParseUint(ss[0], 10, 64)
	if er == nil {
		val = 1e8*big + small
	}

	return
}

// Converts value of satoshis to a BTC-value string (with 8 decimal points)
func UintToBtc(val uint64) (string) {
	return fmt.Sprintf("%d.%08d", val/1e8, val%1e8)
}

// Return true if the given PK_script is a standard P2SH
func IsP2SH(d []byte) bool {
	return len(d)==23 && d[0]==0xa9 && d[1]==20 && d[22]==0x87
}
