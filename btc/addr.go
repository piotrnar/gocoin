package btc

import (
	"bytes"
	"fmt"
	"math/big"
	"errors"
	"os"
)

const (
	ADDRVER_BTC = 0x00
	ADDRVER_TESTNET = 0x6F
)

type BtcAddr struct {
	Version byte
	Hash160 [20]byte
	Checksum [4]byte
	Enc58str string
}

func NewAddrFromString(hs string) (a *BtcAddr, e error) {
	dec := decodeb58(hs)
	if (len(dec)<25) {
		dec = append(bytes.Repeat([]byte{0}, 25-len(dec)), dec...)
	}
	if (len(dec)==25) {
		sh := Sha2Sum(dec[0:21])
		if !bytes.Equal(sh[:4], dec[21:25]) {
			e = errors.New("Address checksum error")
		} else {
			a = new(BtcAddr)
			a.Version = dec[0]
			copy(a.Hash160[:], dec[1:21])
			copy(a.Checksum[:], dec[21:25])
			a.Enc58str = hs
		}
	} else {
		e = errors.New(fmt.Sprintf("Unsupported hash length %d", len(dec)))
	}
	return
}

func NewAddrFromHash160(in []byte, ver byte) (a *BtcAddr) {
	var ad [25]byte
	ad[0] = ver
	copy(ad[1:21], in[:])
	sh := Sha2Sum(ad[0:21])
	copy(ad[21:25], sh[:4])
	
	a = new(BtcAddr)
	a.Version = ver
	copy(a.Hash160[:], in[:])
	copy(a.Checksum[:], sh[:4])
	a.Enc58str = encodeb58(ad[0:25])
	return
}

func NewAddrFromDataWithSum(in []byte, ver byte) (a *BtcAddr, e error) {
	var ad [25]byte
	ad[0] = ver
	copy(ad[1:25], in[:])
	sh := Sha2Sum(ad[0:21])
	if !bytes.Equal(in[20:24], sh[:4]) {
		e = errors.New("Address checksum error")
		return
	}

	copy(ad[21:25], sh[:4])
	
	a = new(BtcAddr)
	a.Version = ver
	copy(a.Hash160[:], in[:])
	copy(a.Checksum[:], sh[:4])
	a.Enc58str = encodeb58(ad[0:25])
	return
}


func (a *BtcAddr) OutScript() (res []byte) {
	res = make([]byte, 25)
	res[0] = 0x76
	res[1] = 0xa9
	res[2] = 20
	copy(res[3:23], a.Hash160[:])
	res[23] = 0x88
	res[24] = 0xac
	return
}

var b58set []byte = []byte("123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz")

func b58chr2int(chr byte) int {
	for i:=range b58set {
		if b58set[i]==chr {
			return i
		}
	}
	fmt.Printf("Unexpected character (%d=0x%x )in b58 string\n", chr, chr)
	os.Exit(1)
	return -1
}


var bn0 *big.Int = big.NewInt(0)
var bn58 *big.Int = big.NewInt(58)

func encodeb58(a []byte) (s string) {
	idx := len(a) * 138 / 100 + 1
	buf := make([]byte, idx)
	bn := big.NewInt(0).SetBytes(a)
	var mo *big.Int
	for bn.Cmp(bn0) != 0 {
		bn, mo = bn.DivMod(bn, bn58, new(big.Int))
		idx--
		buf[idx] = b58set[mo.Int64()]
	}
	for i := range a {
		if a[i]!=0 {
			break
		}
		idx--
		buf[idx] = b58set[0]
	}
	
	s = string(buf[idx:])
	return
}

func decodeb58(s string) []byte {
	bn := big.NewInt(0)
	for i := range s {
		v := b58chr2int(byte(s[i]))
		bn = bn.Mul(bn, bn58)
		bn = bn.Add(bn, big.NewInt(int64(v)))
	}
	return bn.Bytes()
}


