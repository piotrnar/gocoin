/*
This code originates from:
 * https://github.com/WeMeetAgain/go-hdwallet
*/

package btc

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha512"
	"errors"
	"github.com/piotrnar/gocoin/secp256k1"
	"encoding/binary"
)

const (
	Public      = uint32(0x0488B21E)
	Private     = uint32(0x0488ADE4)
	TestPublic  = uint32(0x043587CF)
	TestPrivate = uint32(0x04358394)
)

// HDWallet defines the components of a hierarchical deterministic wallet
type HDWallet struct {
	VBytes      uint32
	Depth       byte
	Checksum    [4]byte
	I           uint32
	Chaincode   []byte //32 bytes
	Key         []byte //33 bytes
}

// Child returns the ith child of wallet w. Values of i >= 2^31
// signify private key derivation. Attempting private key derivation
// with a public key will throw an error.
func (w *HDWallet) Child(i uint32) (*HDWallet, error) {
	var ha, newkey []byte
	var chksum [20]byte

	if w.VBytes==Private || w.VBytes==TestPrivate {
		pub := PublicFromPrivate(w.Key, true)
		mac := hmac.New(sha512.New, w.Chaincode)
		if i >= uint32(0x80000000) {
			mac.Write(w.Key)
		} else {
			mac.Write(pub)
		}
		binary.Write(mac, binary.BigEndian, i)
		ha = mac.Sum(nil)
		newkey = append([]byte{0}, DeriveNextPrivate(ha[:32], w.Key)...)
		RimpHash(PublicFromPrivate(w.Key, true), chksum[:])
	} else if w.VBytes==Public || w.VBytes==TestPublic {
		mac := hmac.New(sha512.New, w.Chaincode)
		if i >= uint32(0x80000000) {
			return &HDWallet{}, errors.New("Can't do Private derivation on Public key!")
		}
		mac.Write(w.Key)
		binary.Write(mac, binary.BigEndian, i)
		ha = mac.Sum(nil)
		newkey = DeriveNextPublic(w.Key, ha[:32])
		RimpHash(w.Key, chksum[:])
	} else {
		return nil, errors.New("Unexpected VBytes")
	}
	res := new(HDWallet)
	res.VBytes = w.VBytes
	res.Depth = w.Depth+1
	copy(res.Checksum[:], chksum[:4])
	res.I = i
	res.Chaincode = ha[32:]
	res.Key = newkey
	return res, nil
}

// Serialize returns the serialized form of the wallet.
// vbytes || depth || fingerprint || i || chaincode || key
func (w *HDWallet) Serialize() []byte {
	var tmp [32]byte
	b := new(bytes.Buffer)
	binary.Write(b, binary.BigEndian, w.VBytes)
	b.WriteByte(w.Depth)
	b.Write(w.Checksum[:])
	binary.Write(b, binary.BigEndian, w.I)
	b.Write(w.Chaincode)
	b.Write(w.Key)
	ShaHash(b.Bytes(), tmp[:])
	return append(b.Bytes(), tmp[:4]...)
}

// String returns the base58-encoded string form of the wallet.
func (w *HDWallet) String() string {
	return Encodeb58(w.Serialize())
}

// StringWallet returns a wallet given a base58-encoded extended key
func StringWallet(data string) (*HDWallet, error) {
	dbin := Decodeb58(data)
	if err := ByteCheck(dbin); err != nil {
		return &HDWallet{}, err
	}
	var res [32]byte
	ShaHash(dbin[:(len(dbin) - 4)], res[:])
	if !bytes.Equal(res[:4], dbin[(len(dbin)-4):]) {
		return &HDWallet{}, errors.New("Invalid checksum")
	}
	r := new(HDWallet)
	r.VBytes = binary.BigEndian.Uint32(dbin[0:4])
	r.Depth = dbin[4]
	copy(r.Checksum[:], dbin[5:9])
	r.I = binary.BigEndian.Uint32(dbin[9:13])
	r.Chaincode = dbin[13:45]
	r.Key = dbin[45:78]
	return r, nil
}

// Pub returns a new wallet which is the public key version of w.
// If w is a public key, Pub returns a copy of w
func (w *HDWallet) Pub() *HDWallet {
	if w.VBytes==Public {
		r := new(HDWallet)
		*r = *w
		return r
	} else {
		return &HDWallet{VBytes:Public, Depth:w.Depth, Checksum:w.Checksum,
			I:w.I, Chaincode:w.Chaincode, Key:PublicFromPrivate(w.Key, true)}
	}
}

// StringChild returns the ith base58-encoded extended key of a base58-encoded extended key.
func StringChild(data string, i uint32) (string, error) {
	w, err := StringWallet(data)
	if err != nil {
		return "", err
	} else {
		w, err = w.Child(i)
		if err != nil {
			return "", err
		} else {
			return w.String(), nil
		}
	}
}

//StringToAddress returns the Bitcoin address of a base58-encoded extended key.
func StringAddress(data string) (string, error) {
	w, err := StringWallet(data)
	if err != nil {
		return "", err
	}

	// WTF the testvectors expect address made from uncompreessed public key?
	tnet := w.VBytes==TestPublic || w.VBytes==TestPrivate
	if false {
		return NewAddrFromPubkey(w.Key, AddrVerPubkey(tnet)).String(), nil
	} else {
		var xy secp256k1.XY
		xy.ParsePubkey(w.Key)
		return NewAddrFromPubkey(xy.Bytes(false), AddrVerPubkey(tnet)).String(), nil
	}
}

// MasterKey returns a new wallet given a random seed.
func MasterKey(seed []byte) *HDWallet {
	key := []byte("Bitcoin seed")
	mac := hmac.New(sha512.New, key)
	mac.Write(seed)
	I := mac.Sum(nil)
	return &HDWallet{VBytes:Private, Chaincode:I[len(I)/2:], Key:append([]byte{0}, I[:len(I)/2]...)}
}

// StringCheck is a validation check of a base58-encoded extended key.
func StringCheck(key string) error {
	return ByteCheck(Decodeb58(key))
}

func ByteCheck(dbin []byte) error {
	// check proper length
	if len(dbin) != 82 {
		return errors.New("invalid string")
	}

	// check for correct Public or Private VBytes
	vb := binary.BigEndian.Uint32(dbin[:4])
	if vb!=Public && vb!=Private && vb!=TestPublic && vb!=TestPrivate {
		return errors.New("invalid string")
	}

	// if Public, check x coord is on curve
	if vb==Public || vb==TestPublic {
		var xy secp256k1.XY
		xy.ParsePubkey(dbin[45:78])
		if !xy.IsValid() {
			return errors.New("invalid string")
		}
	}
	return nil
}
