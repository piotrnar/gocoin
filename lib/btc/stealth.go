package btc

import (
	"fmt"
	"bytes"
	"errors"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"golang.org/x/crypto/ripemd160"
	"github.com/piotrnar/gocoin/lib/secp256k1"
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
	if len(a.Prefix)==0 {
		a = nil
		e = errors.New("StealthAddr: Missing prefix")
	}

	if len(a.Prefix)>0 && a.Prefix[0]>32 {
		e = errors.New("StealthAddr: Prefix out of range")
		a = nil
	}

	return
}


func NewStealthAddrFromString(hs string) (a *StealthAddr, e error) {
	dec := Decodeb58(hs)
	if dec == nil {
		e = errors.New("StealthAddr: Cannot decode b58 string '"+hs+"'")
		return
	}
	return NewStealthAddr(dec)
}


func (a *StealthAddr) BytesNoPrefix() []byte {
	b := new(bytes.Buffer)
	b.WriteByte(a.Version)
	b.WriteByte(a.Options)
	b.Write(a.ScanKey[:])
	b.WriteByte(byte(len(a.SpendKeys)))
	for i:=range a.SpendKeys {
		b.Write(a.SpendKeys[i][:])
	}
	b.WriteByte(a.Sigs)
	return b.Bytes()
}


func (a *StealthAddr) PrefixLen() byte {
	if len(a.Prefix)==0 {
		return 0
	}
	return a.Prefix[0]
}


func (a *StealthAddr) Bytes(checksum bool) []byte {
	b := new(bytes.Buffer)
	b.Write(a.BytesNoPrefix())
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
	rim.Write(a.BytesNoPrefix())
	return rim.Sum(nil)
}


// Calculate the stealth difference
func StealthDH(pub, priv []byte) []byte {
	var res [33]byte

	if !secp256k1.Multiply(pub, priv, res[:]) {
		return nil
	}

	s := sha256.New()
	s.Write(res[:])
	return s.Sum(nil)
}


// Calculate the stealth difference
func StealthPub(pub, priv []byte) (res []byte) {
	res = make([]byte, 33)
	if !secp256k1.Multiply(pub, priv, res) {
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


// Thanks @dabura667 - https://bitcointalk.org/index.php?topic=590349.msg6560332#msg6560332
func MakeStealthTxOuts(sa *StealthAddr, value uint64, testnet bool) (res []*TxOut, er error) {
	if sa.Version != StealthAddressVersion(testnet) {
		er = errors.New(fmt.Sprint("ERROR: Unsupported version of a stealth address", sa.Version))
		return
	}

	if len(sa.SpendKeys) != 1 || sa.Sigs != 1 {
		er = errors.New(fmt.Sprint("ERROR: Currently only non-multisig stealth addresses are supported",
			len(sa.SpendKeys)))
		return
	}

	// Make two outpus
	res = make([]*TxOut, 2)
	var e, ephemkey, pkscr []byte
	var nonce, nonce_from uint32
	sha := sha256.New()

	// 6. create a new pub/priv keypair (lets call its pubkey "ephemkey" and privkey "e")
pick_different_e:
	e = make([]byte, 32)
	rand.Read(e)
	ephemkey = PublicFromPrivate(e, true)

	// 7. IF there is a prefix in the stealth address, brute force a nonce such
	// that SHA256(nonce.concate(ephemkey)) first 4 bytes are equal to the prefix.
	// IF NOT, then just run through the loop once and pickup a random nonce.
	// (probably make the while condition include "or prefix = null" or something to that nature.
	binary.Read(rand.Reader, binary.LittleEndian, &nonce_from)
	nonce = nonce_from
	for {
		binary.Write(sha, binary.LittleEndian, nonce)
		sha.Write(ephemkey)

		if sa.CheckPrefix(sha.Sum(nil)[:4]) {
			break
		}
		sha.Reset()

		nonce++
		if nonce==nonce_from {
			fmt.Println("EOF")
			goto pick_different_e
		}

		if (nonce&0xfffff)==0 {
			fmt.Print(".")
		}
	}

	// 8. Once you have the nonce and the ephemkey, you can create the first output, which is
	pkscr = make([]byte, 40)
	pkscr[0] = 0x6a // OP_RETURN
	pkscr[1] = 38 // length
	pkscr[2] = 0x06 // always 6
	binary.LittleEndian.PutUint32(pkscr[3:7], nonce)
	copy(pkscr[7:40], ephemkey)
	res[0] = &TxOut{Pk_script: pkscr}

	// 9. Now use ECC multiplication to calculate e*Q where Q = scan_pubkey
	// an e = privkey to ephemkey and then hash it.
	c := StealthDH(sa.ScanKey[:], e)
	rand.Read(e) // clear ephemkey private key from memory

	// 10. That hash is now "c". use ECC multiplication and addition to
	// calculate D + (c*G) where D = spend_pubkey, and G is the reference
	// point for secp256k1. This will give you a new pubkey. (we'll call it D')
	Dpr := DeriveNextPublic(sa.SpendKeys[0][:], c)

	// 11. Create a normal P2KH output spending to D' as public key.
	adr := NewAddrFromPubkey(Dpr, AddrVerPubkey(testnet))
	res[1] = &TxOut{Value: value, Pk_script: adr.OutScript() }

	return
}
