package btc

import (
	"bytes"
	"fmt"
	"math/big"
	"errors"
	"crypto/sha256"
	"code.google.com/p/go.crypto/ripemd160"
)

const (
	ADDRVER_BTC = 0x00
	ADDRVER_TESTNET = 0x6F
)

type BtcAddr struct {
	Version byte
	Hash160 [20]byte
	Checksum []byte
	Pubkey []byte
	Enc58str string 
}

func NewAddrFromString(hs string) (a *BtcAddr, e error) {
	dec := Decodeb58(hs)
	if dec == nil {
		e = errors.New("Cannot decode b58 string *"+hs+"*")
		return
	}
	if (len(dec)<25) {
		dec = append(bytes.Repeat([]byte{0}, 25-len(dec)), dec...)
	}
	if (len(dec)==25) {
		sh := Sha2Sum(dec[0:21])
		if !bytes.Equal(sh[:4], dec[21:25]) {
			e = errors.New("Address Checksum error")
		} else {
			a = new(BtcAddr)
			a.Version = dec[0]
			copy(a.Hash160[:], dec[1:21])
			a.Checksum = make([]byte, 4)
			copy(a.Checksum, dec[21:25])
			a.Enc58str = hs
		}
	} else {
		e = errors.New(fmt.Sprintf("Unsupported hash length %d", len(dec)))
	}
	return
}

func NewAddrFromHash160(in []byte, ver byte) (a *BtcAddr) {
	a = new(BtcAddr)
	a.Version = ver
	copy(a.Hash160[:], in[:])
	return
}

func NewAddrFromPubkey(in []byte, ver byte) (a *BtcAddr) {
	a = new(BtcAddr)
	a.Pubkey = make([]byte, len(in))
	copy(a.Pubkey[:], in[:])
	a.Version = ver
	sha := sha256.New()
	rim := ripemd160.New()
	sha.Write(in)
	rim.Write(sha.Sum(nil)[:])
	copy(a.Hash160[:], rim.Sum(nil))
	return
}

func NewAddrFromDataWithSum(in []byte, ver byte) (a *BtcAddr, e error) {
	var ad [25]byte
	ad[0] = ver
	copy(ad[1:25], in[:])
	sh := Sha2Sum(ad[0:21])
	if !bytes.Equal(in[20:24], sh[:4]) {
		e = errors.New("Address Checksum error")
		return
	}

	copy(ad[21:25], sh[:4])
	
	a = new(BtcAddr)
	a.Version = ver
	copy(a.Hash160[:], in[:])
	
	a.Checksum = make([]byte, 4)
	copy(a.Checksum, sh[:4])
	return
}

func (a *BtcAddr) String() string {
	if a.Enc58str=="" {
		var ad [25]byte
		ad[0] = a.Version
		copy(ad[1:21], a.Hash160[:])
		if a.Checksum==nil {
			sh := Sha2Sum(ad[0:21])
			a.Checksum = make([]byte, 4)
			copy(a.Checksum, sh[:4])
		}
		copy(ad[21:25], a.Checksum[:])
		a.Enc58str = Encodeb58(ad[:])
	}
	return a.Enc58str
}

func (a *BtcAddr) Owns(scr []byte) (yes bool) {
	if len(scr)==25 && scr[0]==0x76 && scr[1]==0xa9 && scr[2]==0x14 && scr[23]==0x88 && scr[24]==0xac {
		yes = bytes.Equal(scr[3:23], a.Hash160[:])
		return 
	} else if len(scr)==67 && scr[0]==0x41 && scr[1]==0x04 && scr[66]==0xac {
		if a.Pubkey == nil {
			rim := ripemd160.New()
			rim.Write(scr[1:66])
			h := rim.Sum(nil)
			if bytes.Equal(h, a.Hash160[:]) {
				a.Pubkey = make([]byte, 65)
				copy(a.Pubkey, scr[1:66])
				yes = true
				return
			}
			return
		}
		yes = bytes.Equal(scr[1:66], a.Pubkey)
		return 
	} else if len(scr)==23 && scr[0]==0xa9 && scr[1]==0x14 && scr[22]==0x87 {
		yes = bytes.Equal(scr[2:22], a.Hash160[:])
		return 
	}
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
	return -1
}


var bn0 *big.Int = big.NewInt(0)
var bn58 *big.Int = big.NewInt(58)

func Encodeb58(a []byte) (s string) {
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

func Decodeb58(s string) []byte {
	bn := big.NewInt(0)
	for i := range s {
		v := b58chr2int(byte(s[i]))
		if v < 0 {
			return nil
		}
		bn = bn.Mul(bn, bn58)
		bn = bn.Add(bn, big.NewInt(int64(v)))
	}
	return bn.Bytes()
}


