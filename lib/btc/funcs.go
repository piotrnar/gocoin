package btc

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

func allzeros(b []byte) bool {
	for i := range b {
		if b[i] != 0 {
			return false
		}
	}
	return true
}

// PutVlen puts var_length field into the given buffer.
func PutVlen(b []byte, vl int) uint32 {
	uvl := uint32(vl)
	if uvl < 0xfd {
		b[0] = byte(uvl)
		return 1
	}
	if uvl < 0x10000 {
		b[0] = 0xfd
		b[1] = byte(uvl)
		b[2] = byte(uvl >> 8)
		return 3
	}
	b[0] = 0xfe
	b[1] = byte(uvl)
	b[2] = byte(uvl >> 8)
	b[3] = byte(uvl >> 16)
	b[4] = byte(uvl >> 24)
	return 5
}

func PutULe(b []byte, uvl uint64) int {
	if uvl < 0xfd {
		b[0] = byte(uvl)
		return 1
	}
	if uvl < 0x10000 {
		b[0] = 0xfd
		binary.LittleEndian.PutUint16(b[1:3], uint16(uvl))
		return 3
	}
	if uvl < 0x100000000 {
		b[0] = 0xfe
		binary.LittleEndian.PutUint32(b[1:5], uint32(uvl))
		return 5
	}
	b[0] = 0xff
	binary.LittleEndian.PutUint64(b[1:9], uvl)
	return 9
}

// VLenSize returns how many bytes it would take to write this VLen.
func VLenSize(uvl uint64) int {
	if uvl < 0xfd {
		return 1
	}
	if uvl < 0x10000 {
		return 3
	}
	if uvl < 0x100000000 {
		return 5
	}
	return 9
}

// VLen returns var_int and number of bytes that the var_int took.
// If there is not enough bytes in the buffer, it returns 0, 0.
func VLen(b []byte) (le int, var_int_siz int) {
	if len(b) > 0 {
		switch b[0] {
		case 0xfd:
			if len(b) >= 3 {
				return int(binary.LittleEndian.Uint16(b[1:3])), 3
			}
		case 0xfe:
			if len(b) >= 5 {
				return int(binary.LittleEndian.Uint32(b[1:5])), 5
			}
		case 0xff:
			if len(b) >= 9 {
				return int(binary.LittleEndian.Uint64(b[1:9])), 9
			}
		default:
			return int(b[0]), 1
		}
	}
	return
}

// VULe returns var_uint and number of bytes that the var_uint took.
// If there is not enough bytes in the buffer, it returns 0, 0.
func VULe(b []byte) (le uint64, var_int_siz int) {
	if len(b) > 0 {
		switch b[0] {
		case 0xfd:
			if len(b) >= 3 {
				return uint64(binary.LittleEndian.Uint16(b[1:3])), 3
			}
		case 0xfe:
			if len(b) >= 5 {
				return uint64(binary.LittleEndian.Uint32(b[1:5])), 5
			}
		case 0xff:
			if len(b) >= 9 {
				return uint64(binary.LittleEndian.Uint64(b[1:9])), 9
			}
		default:
			return uint64(b[0]), 1
		}
	}
	return
}

func CalcMerkle(mtr [][32]byte) (res []byte, mutated bool) {
	var j, i2 int
	for siz := len(mtr); siz > 1; siz = (siz + 1) / 2 {
		for i := 0; i < siz; i += 2 {
			if i+1 < siz-1 {
				i2 = i + 1
			} else {
				i2 = siz - 1
			}
			if i != i2 && bytes.Equal(mtr[j+i][:], mtr[j+i2][:]) {
				mutated = true
			}
			s := sha256.New()
			s.Write(mtr[j+i][:])
			s.Write(mtr[j+i2][:])
			tmp := s.Sum(nil)
			s.Reset()
			s.Write(tmp)

			var sum [32]byte
			copy(sum[:], s.Sum(nil))
			mtr = append(mtr, sum)
		}
		j += siz
	}
	res = mtr[len(mtr)-1][:]
	return
}

func GetWitnessMerkle(txs []*Tx) (res []byte, mutated bool) {
	mtr := make([][32]byte, len(txs), 3*len(txs)) // make the buffer 3 times longer as we use append() inside CalcMerkle
	//mtr[0] = make([]byte, 32) // null
	for i := 1; i < len(txs); i++ {
		mtr[i] = txs[i].WTxID().Hash
	}
	res, mutated = CalcMerkle(mtr)
	return
}

func ReadAll(rd io.Reader, b []byte) (er error) {
	var n int
	for i := 0; i < len(b); i += n {
		n, er = rd.Read(b[i:])
		if er != nil {
			return
		}
	}
	return
}

// ReadVLen reads var_len from the given reader.
func ReadVLen(b io.Reader) (res uint64, e error) {
	var buf [8]byte

	if e = ReadAll(b, buf[:1]); e != nil {
		//println("ReadVLen1 error:", e.Error())
		return
	}

	if buf[0] < 0xfd {
		res = uint64(buf[0])
		return
	}

	c := 2 << (2 - (0xff - buf[0]))

	if e = ReadAll(b, buf[:c]); e != nil {
		println("ReadVLen1 error:", e.Error())
		return
	}
	for i := 0; i < c; i++ {
		res |= (uint64(buf[i]) << uint64(8*i))
	}
	return
}

// WriteVlen writes var_length field into the given writer.
func WriteVlen(b io.Writer, var_len uint64) {
	if var_len < 0xfd {
		b.Write([]byte{byte(var_len)})
		return
	}
	if var_len < 0x10000 {
		b.Write([]byte{0xfd})
		binary.Write(b, binary.LittleEndian, uint16(var_len))
		return
	}
	if var_len < 0x100000000 {
		b.Write([]byte{0xfe})
		binary.Write(b, binary.LittleEndian, uint32(var_len))
		return
	}
	b.Write([]byte{0xff})
	binary.Write(b, binary.LittleEndian, var_len)
}

// WritePutLen writes opcode to put a specific number of bytes to stack.
func WritePutLen(b io.Writer, data_len uint32) {
	switch {
	case data_len <= OP_PUSHDATA1:
		b.Write([]byte{byte(data_len)})

	case data_len < 0x100:
		b.Write([]byte{OP_PUSHDATA1, byte(data_len)})

	case data_len < 0x10000:
		b.Write([]byte{OP_PUSHDATA2})
		binary.Write(b, binary.LittleEndian, uint16(data_len))

	default:
		b.Write([]byte{OP_PUSHDATA4})
		binary.Write(b, binary.LittleEndian, uint32(data_len))
	}
}

// ReadString reads bitcoin protocol string.
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

// ParseMessageSignature takes a Base64 encoded Bitcoin generated signature and decodes it.
func ParseMessageSignature(encsig string) (nv byte, sig *Signature, er error) {
	var sd []byte

	sd, er = base64.StdEncoding.DecodeString(encsig)
	if er != nil {
		return
	}

	if len(sd) != 65 {
		er = errors.New("The decoded signature is not 65 bytes long")
		return
	}

	nv = sd[0]

	sig = new(Signature)
	sig.R.SetBytes(sd[1:33])
	sig.S.SetBytes(sd[33:65])

	if nv < 27 || nv > 34 {
		er = errors.New("nv out of range")
	}

	return
}

func IsPayToScript(scr []byte) bool {
	return len(scr) == 23 && scr[0] == OP_HASH160 && scr[1] == 0x14 && scr[22] == OP_EQUAL
}

// StringToSatoshis parses a floating number string to return uint64 value expressed in Satoshis.
// Using strconv.ParseFloat followed by uint64(val*1e8) is not precise enough.
func StringToSatoshis(s string) (val uint64, er error) {
	var big, small uint64

	ss := strings.Split(s, ".")
	if len(ss) == 1 {
		val, er = strconv.ParseUint(ss[0], 10, 64)
		if er != nil {
			return
		}
		val *= 1e8
		return
	}
	if len(ss) != 2 {
		println("Incorrect amount", s)
		os.Exit(1)
	}

	if len(ss[1]) > 8 {
		er = errors.New("Too many decimal points")
		return
	}
	if len(ss[1]) < 8 {
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

// UintToBtc converts value of satoshis to a BTC-value string (with 8 decimal points).
func UintToBtc(val uint64) string {
	return fmt.Sprintf("%d.%08d", val/1e8, val%1e8)
}

// IsP2SH returns true if the given PK_script is a standard P2SH.
func IsP2SH(d []byte) bool {
	return len(d) == 23 && d[0] == 0xa9 && d[1] == 20 && d[22] == 0x87
}

// IsUsefullOutScript returns true if the given PK_script is somehow useful to gocoin's node.
func IsUsefullOutScript(v []byte) bool {
	if len(v) == 25 && v[0] == 0x76 && v[1] == 0xa9 && v[2] == 0x14 && v[23] == 0x88 && v[24] == 0xac {
		return true // P2KH
	}
	if len(v) == 23 && v[0] == 0xa9 && v[1] == 0x14 && v[22] == 0x87 {
		return true // P2SH
	}
	return false
}

func GetOpcode(b []byte) (opcode int, ret []byte, pc int, e error) {
	// Read instruction
	if pc+1 > len(b) {
		e = errors.New("GetOpcode error 1")
		return
	}
	opcode = int(b[pc])
	pc++

	if opcode <= OP_PUSHDATA4 {
		size := 0
		if opcode < OP_PUSHDATA1 {
			size = opcode
		}
		if opcode == OP_PUSHDATA1 {
			if pc+1 > len(b) {
				e = errors.New("GetOpcode error 2")
				return
			}
			size = int(b[pc])
			pc++
		} else if opcode == OP_PUSHDATA2 {
			if pc+2 > len(b) {
				e = errors.New("GetOpcode error 3")
				return
			}
			size = int(binary.LittleEndian.Uint16(b[pc : pc+2]))
			pc += 2
		} else if opcode == OP_PUSHDATA4 {
			if pc+4 > len(b) {
				e = errors.New("GetOpcode error 4")
				return
			}
			size = int(binary.LittleEndian.Uint16(b[pc : pc+4]))
			pc += 4
		}
		if pc+size > len(b) {
			e = errors.New(fmt.Sprint("GetOpcode size to fetch exceeds remainig data left: ", pc+size, "/", len(b)))
			return
		}
		ret = b[pc : pc+size]
		pc += size
	}

	return
}

func GetSigOpCount(scr []byte, fAccurate bool) (n uint) {
	var pc int
	var lastOpcode byte = 0xff
	for pc < len(scr) {
		opcode, _, le, e := GetOpcode(scr[pc:])
		if e != nil {
			break
		}
		pc += le
		if opcode == 0xac /*OP_CHECKSIG*/ || opcode == 0xad /*OP_CHECKSIGVERIFY*/ {
			n++
		} else if opcode == 0xae /*OP_CHECKMULTISIG*/ || opcode == 0xaf /*OP_CHECKMULTISIGVERIFY*/ {
			if fAccurate && lastOpcode >= 0x51 /*OP_1*/ && lastOpcode <= 0x60 /*OP_16*/ {
				n += uint(DecodeOP_N(lastOpcode))
			} else {
				n += MAX_PUBKEYS_PER_MULTISIG
			}
		}
		lastOpcode = byte(opcode)
	}
	return
}

func DecodeOP_N(opcode byte) int {
	if opcode == 0x00 /*OP_0*/ {
		return 0
	}
	return int(opcode) - 0x50 /*OP_1-1*/
}

func GetP2SHSigOpCount(scr []byte) uint {
	// This is a pay-to-script-hash scriptPubKey;
	// get the last item that the scr
	// pushes onto the stack:
	var pc, opcode, le int
	var e error
	var data []byte
	for pc < len(scr) {
		opcode, data, le, e = GetOpcode(scr[pc:])
		if e != nil {
			return 0
		}
		pc += le
		if opcode > 0x60 /*OP_16*/ {
			return 0
		}
	}

	return GetSigOpCount(data, true)
}

func IsWitnessProgram(scr []byte) (version int, program []byte) {
	if len(scr) < 4 || len(scr) > 42 {
		return
	}
	if scr[0] != OP_0 && (scr[0] < OP_1 || scr[0] > OP_16) {
		return
	}
	if int(scr[1])+2 == len(scr) {
		version = DecodeOP_N(scr[0])
		program = scr[2:]
	}
	return
}

func WitnessSigOps(witversion int, witprogram []byte, witness [][]byte) uint {
	if witversion == 0 {
		if len(witprogram) == 20 {
			return 1
		}

		if len(witprogram) == 32 && len(witness) > 0 {
			subscript := witness[len(witness)-1]
			return GetSigOpCount(subscript, true)
		}
	}
	return 0
}

func IsPushOnly(scr []byte) bool {
	idx := 0
	for idx < len(scr) {
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

// Amount compression:
// * If the amount is 0, output 0
// * first, divide the amount (in base units) by the largest power of 10 possible; call the exponent e (e is max 9)
// * if e<9, the last digit of the resulting number cannot be 0; store it as d, and drop it (divide by 10)
//   * call the result n
//   * output 1 + 10*(9*n + d - 1) + e
// * if e==9, we only know the resulting number is not zero, so output 1 + 10*(n - 1) + 9
// (this is decodable, as d is in [1-9] and e is in [0-9])
func CompressAmount(n uint64) uint64 {
	if n == 0 {
		return 0
	}
	var e uint64
	for (n%10) == 0 && e < 9 {
		n /= 10
		e++
	}
	if e < 9 {
		d := n % 10
		n /= 10
		return 1 + (n*9+d-1)*10 + e
	} else {
		return 1 + (n-1)*10 + 9
	}
}

func DecompressAmount(x uint64) uint64 {
	// x = 0  OR  x = 1+10*(9*n + d - 1) + e  OR  x = 1+10*(n - 1) + 9
	if x == 0 {
		return 0
	}
	x--
	// x = 10*(9*n + d - 1) + e
	e := x % 10
	x /= 10
	var n uint64
	if e < 9 {
		// x = 9*n + d - 1
		d := (x % 9) + 1
		x /= 9
		// x = n
		n = x*10 + d
	} else {
		n = x + 1
	}
	for e != 0 {
		n *= 10
		e--
	}
	return n
}
