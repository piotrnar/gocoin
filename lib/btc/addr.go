package btc

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/piotrnar/gocoin/lib/others/bech32"
)

type BtcAddr struct {
	Version  byte
	Hash160  [20]byte
	Checksum []byte
	Pubkey   []byte
	Enc58str string

	*SegwitProg // if this is not nil, means that this is a native segwit address

	// This is used only by the client
	Extra struct {
		Label  string
		Wallet string
		Virgin bool
	}
}

type SegwitProg struct {
	HRP     string
	Version int
	Program []byte
}

func NewAddrFromString(hs string) (a *BtcAddr, e error) {
	if strings.HasPrefix(hs, "bc1") || strings.HasPrefix(hs, "tb1") {
		var sw = &SegwitProg{HRP: hs[:2]}
		sw.Version, sw.Program = bech32.SegwitDecode(sw.HRP, hs)
		if sw.Program != nil {
			a = &BtcAddr{SegwitProg: sw}
		}
		return
	}

	dec := Decodeb58(hs)
	if dec == nil {
		e = errors.New("Cannot decode b58 string '" + hs + "'")
		return
	}
	if len(dec) < 25 {
		e = errors.New("Address too short " + hex.EncodeToString(dec))
		return
	}
	if len(dec) == 25 {
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
		e = errors.New("Unrecognized address payload " + hex.EncodeToString(dec))
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
	RimpHash(in, a.Hash160[:])
	return
}

func AddrVerPubkey(testnet bool) byte {
	if testnet {
		return 111
	} else {
		return 0
	}
}

func AddrVerScript(testnet bool) byte {
	if testnet {
		return 196
	} else {
		return 5
	}
}

func NewAddrFromPkScript(scr []byte, testnet bool) *BtcAddr {
	// check segwit bech32:
	if len(scr) == 0 {
		return nil
	}

	if version, program := IsWitnessProgram(scr); program != nil {
		sw := &SegwitProg{HRP: GetSegwitHRP(testnet), Version: version, Program: program}

		str := bech32.SegwitEncode(sw.HRP, version, program)
		if str == "" {
			return nil
		}

		ad := new(BtcAddr)
		ad.Enc58str = str
		ad.SegwitProg = sw

		return ad
	}

	if len(scr) == 25 && scr[0] == 0x76 && scr[1] == 0xa9 && scr[2] == 0x14 && scr[23] == 0x88 && scr[24] == 0xac {
		return NewAddrFromHash160(scr[3:23], AddrVerPubkey(testnet))
	} else if len(scr) == 67 && scr[0] == 0x41 && scr[66] == 0xac {
		return NewAddrFromPubkey(scr[1:66], AddrVerPubkey(testnet))
	} else if len(scr) == 35 && scr[0] == 0x21 && scr[34] == 0xac {
		return NewAddrFromPubkey(scr[1:34], AddrVerPubkey(testnet))
	} else if len(scr) == 23 && scr[0] == 0xa9 && scr[1] == 0x14 && scr[22] == 0x87 {
		return NewAddrFromHash160(scr[2:22], AddrVerScript(testnet))
	}
	return nil
}

// String returns the Base58 encoded address.
func (a *BtcAddr) String() string {
	if a.Enc58str == "" {
		if a.SegwitProg != nil {
			a.Enc58str = a.SegwitProg.String()
		} else {
			var ad [25]byte
			ad[0] = a.Version
			copy(ad[1:21], a.Hash160[:])
			if a.Checksum == nil {
				sh := Sha2Sum(ad[0:21])
				a.Checksum = make([]byte, 4)
				copy(a.Checksum, sh[:4])
			}
			copy(ad[21:25], a.Checksum[:])
			a.Enc58str = Encodeb58(ad[:])
		}
	}
	return a.Enc58str
}

func (a *BtcAddr) IsCompressed() bool {
	if len(a.Pubkey) == 33 {
		return true
	}
	if len(a.Pubkey) != 65 {
		panic("Cannot determine whether the key was compressed")
	}
	return false
}

// String with a label
func (a *BtcAddr) Label() (s string) {
	if a.Extra.Wallet != "" {
		s += " " + a.Extra.Wallet + ":"
	}
	if a.Extra.Label != "" {
		s += " " + a.Extra.Label
	}
	if a.Extra.Virgin {
		s += " ***"
	}
	return
}

// Owns checks if a pk_script send coins to this address.
func (a *BtcAddr) Owns(scr []byte) (yes bool) {
	// The most common spend script
	if len(scr) == 25 && scr[0] == 0x76 && scr[1] == 0xa9 && scr[2] == 0x14 && scr[23] == 0x88 && scr[24] == 0xac {
		yes = bytes.Equal(scr[3:23], a.Hash160[:])
		return
	}

	// Spend script with an entire public key
	if len(scr) == 67 && scr[0] == 0x41 && scr[1] == 0x04 && scr[66] == 0xac {
		if a.Pubkey == nil {
			h := Rimp160AfterSha256(scr[1:66])
			if h == a.Hash160 {
				a.Pubkey = make([]byte, 65)
				copy(a.Pubkey, scr[1:66])
				yes = true
			}
			return
		}
		yes = bytes.Equal(scr[1:34], a.Pubkey[:33])
		return
	}

	// Spend script with a compressed public key
	if len(scr) == 35 && scr[0] == 0x21 && (scr[1] == 0x02 || scr[1] == 0x03) && scr[34] == 0xac {
		if a.Pubkey == nil {
			h := Rimp160AfterSha256(scr[1:34])
			if h == a.Hash160 {
				a.Pubkey = make([]byte, 33)
				copy(a.Pubkey, scr[1:34])
				yes = true
			}
			return
		}
		yes = bytes.Equal(scr[1:34], a.Pubkey[:33])
		return
	}

	return
}

func (a *BtcAddr) OutScript() (res []byte) {
	if a.SegwitProg != nil {
		res = make([]byte, 2+len(a.SegwitProg.Program))
		res[0] = byte(a.SegwitProg.Version)
		res[1] = byte(len(a.SegwitProg.Program))
		copy(res[2:], a.SegwitProg.Program)
	} else if a.Version == AddrVerPubkey(false) || a.Version == AddrVerPubkey(true) || a.Version == 48 /*Litecoin*/ {
		res = make([]byte, 25)
		res[0] = 0x76
		res[1] = 0xa9
		res[2] = 20
		copy(res[3:23], a.Hash160[:])
		res[23] = 0x88
		res[24] = 0xac
	} else if a.Version == AddrVerScript(false) || a.Version == AddrVerScript(true) {
		res = make([]byte, 23)
		res[0] = 0xa9
		res[1] = 20
		copy(res[2:22], a.Hash160[:])
		res[22] = 0x87
	} else {
		panic(fmt.Sprint("Cannot create OutScript for address version ", a.Version))
	}
	return
}

var b58set []byte = []byte("123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz")

func b58chr2int(chr byte) int {
	for i := range b58set {
		if b58set[i] == chr {
			return i
		}
	}
	return -1
}

var bn0 *big.Int = big.NewInt(0)
var bn58 *big.Int = big.NewInt(58)

func Encodeb58(a []byte) (s string) {
	idx := len(a)*138/100 + 1
	buf := make([]byte, idx)
	bn := new(big.Int).SetBytes(a)
	var mo *big.Int
	for bn.Cmp(bn0) != 0 {
		bn, mo = bn.DivMod(bn, bn58, new(big.Int))
		idx--
		buf[idx] = b58set[mo.Int64()]
	}
	for i := range a {
		if a[i] != 0 {
			break
		}
		idx--
		buf[idx] = b58set[0]
	}

	s = string(buf[idx:])

	return
}

func Decodeb58(s string) (res []byte) {
	bn := big.NewInt(0)
	for i := range s {
		v := b58chr2int(byte(s[i]))
		if v < 0 {
			return nil
		}
		bn = bn.Mul(bn, bn58)
		bn = bn.Add(bn, big.NewInt(int64(v)))
	}

	// We want to "restore leading zeros" as satoshi's implementation does:
	var i int
	for i < len(s) && s[i] == b58set[0] {
		i++
	}
	if i > 0 {
		res = make([]byte, i)
	}
	res = append(res, bn.Bytes()...)
	return
}

func (sw *SegwitProg) String() (res string) {
	res = bech32.SegwitEncode(sw.HRP, sw.Version, sw.Program)
	return
}

func GetSegwitHRP(testnet bool) string {
	if testnet {
		return "tb"
	} else {
		return "bc"
	}
}
