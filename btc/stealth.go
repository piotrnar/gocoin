package btc

import (
	"bytes"
	"errors"
	"crypto/sha256"
	"github.com/piotrnar/gocoin/secp256k1"
	"code.google.com/p/go.crypto/ripemd160"
)


func StealthAddressVersion(testnet bool) byte {
	if testnet {
		return 43
	} else {
		return 42
	}
}


type StealthAddr struct {
	Version byte
	Options byte
	ScanKey [33]byte
	SpendKeys [][33]byte
	Sigs byte
	Prefix []byte
}


func NewStealthAddr(dec []byte) (a *StealthAddr, e error) {
	var tmp byte

	if (len(dec)<2+33+33+1+1+4) {
		e = errors.New("StealthAddr: data too short")
		return
	}

	sh := Sha2Sum(dec[0:len(dec)-4])
	if !bytes.Equal(sh[:4], dec[len(dec)-4:len(dec)]) {
		e = errors.New("StealthAddr: Checksum error")
		return
	}

	a = new(StealthAddr)

	b := bytes.NewBuffer(dec[0:len(dec)-4])

	if a.Version, e = b.ReadByte(); e != nil {
		a = nil
		return
	}
	if a.Options, e = b.ReadByte(); e != nil {
		a = nil
		return
	}
	if _, e = b.Read(a.ScanKey[:]); e != nil {
		a = nil
		return
	}
	if tmp, e = b.ReadByte(); e != nil {
		a = nil
		return
	}
	a.SpendKeys = make([][33]byte, int(tmp))
	for i := range a.SpendKeys {
		if _, e = b.Read(a.SpendKeys[i][:]); e != nil {
			a = nil
			return
		}
	}
	if a.Sigs, e = b.ReadByte(); e != nil {
		a = nil
		return
	}
	a.Prefix = b.Bytes()

	if len(a.Prefix)>0 && a.Prefix[0]>32 {
		e = errors.New("StealthAddr: Prefix out of range")
		a = nil
	}

	return
}


func NewStealthAddrFromString(hs string) (a *StealthAddr, e error) {
	dec := Decodeb58(hs)
	if dec == nil {
		e = errors.New("StealthAddr: Cannot decode b58 string *"+hs+"*")
		return
	}
	return NewStealthAddr(dec)
}


func (a *StealthAddr) Bytes(checksum bool) []byte {
	b := new(bytes.Buffer)
	b.WriteByte(a.Version)
	b.WriteByte(a.Options)
	b.Write(a.ScanKey[:])
	b.WriteByte(byte(len(a.SpendKeys)))
	for i:=range a.SpendKeys {
		b.Write(a.SpendKeys[i][:])
	}
	b.WriteByte(a.Sigs)
	b.Write(a.Prefix)
	if checksum {
		sh := Sha2Sum(b.Bytes())
		b.Write(sh[:4])
	}
	return b.Bytes()
}


func (a *StealthAddr) String() (string) {
	return Encodeb58(a.Bytes(true))
}


// Calculate a unique 200-bits long hash of the address
func (a *StealthAddr) Hash160() []byte {
	rim := ripemd160.New()
	rim.Write(a.Bytes(false))
	return rim.Sum(nil)
}


// Calculate the stealth difference
func StealthDH(pub, priv []byte) []byte {
	var res [33]byte

	if !secp256k1.Multiply(pub, priv, res[:]) {
		return nil
	}

	s := sha256.New()
	s.Write([]byte{0x03})
	s.Write(res[1:])
	return s.Sum(nil)
}


// Calculate the stealth difference
func StealthPub(pub, priv []byte) (res []byte) {
	res = make([]byte, 33)
	if secp256k1.Multiply(pub, priv, res) {
	} else {
		res = nil
	}
	return
}


// Calculate the stealth difference
func (sa *StealthAddr) CheckPrefix(cur []byte) bool {
	var idx, plen, byt, mask, match byte
	if len(sa.Prefix)==0 {
		return true
	}
	plen = sa.Prefix[0]
	if plen > 0 {
		mask = 0x80
		match = cur[0]^sa.Prefix[1]
		for {
			if (match&mask) != 0 {
				return false
			}
			if idx==(plen-1) {
				break
			}
			idx++
			if mask==0x01 {
				byt++
				mask = 0x80
				match = cur[byt]^sa.Prefix[byt+1]
			} else {
				mask >>= 1
			}
		}
	}
	return true
}


func (sa *StealthAddr) CheckNonce(payload []byte) bool {
	sha := sha256.New()
	sha.Write(payload)
	return sa.CheckPrefix(sha.Sum(nil)[:4])
}
