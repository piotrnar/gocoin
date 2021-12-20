package btc

import (
	"bytes"
	"encoding/hex"
	"errors"
	"math/big"

	"github.com/piotrnar/gocoin/lib/secp256k1"
)

// PublicFromPrivate gets the ECDSA public key in Bitcoin protocol format, from the give private key.
func PublicFromPrivate(priv_key []byte, compressed bool) (res []byte) {
	if compressed {
		res = make([]byte, 33)
	} else {
		res = make([]byte, 65)
	}

	if !secp256k1.BaseMultiply(priv_key, res) {
		res = nil
	}
	return
}

// VerifyKeyPair verifies the secret key's range and if a test message signed with it verifies OK.
// Returns nil if everything looks OK.
func VerifyKeyPair(priv []byte, publ []byte) error {
	var sig Signature

	const TestMessage = "Just some test message..."
	hash := Sha2Sum([]byte(TestMessage))

	D := new(big.Int).SetBytes(priv)

	if D.Cmp(big.NewInt(0)) == 0 {
		return errors.New("pubkey value is zero")
	}

	if D.Cmp(&secp256k1.TheCurve.Order.Int) != -1 {
		return errors.New("pubkey value is too big")
	}

	r, s, e := EcdsaSign(priv, hash[:])
	if e != nil {
		return errors.New("EcdsaSign failed: " + e.Error())
	}

	sig.R.Set(r)
	sig.S.Set(s)
	ok := EcdsaVerify(publ, sig.Bytes(), hash[:])
	if !ok {
		return errors.New("EcdsaVerify failed")
	}

	// Now the same using a schnorr sign/verify
	randata := Sha2Sum(append(hash[:], []byte(TestMessage)...))
	ssig := secp256k1.SchnorrSign(hash[:], priv, randata[:])
	if len(ssig) != 64 {
		return errors.New("SchnorrSign failed: " + hex.EncodeToString(ssig))
	}

	ok = secp256k1.SchnorrVerify(publ[1:33], ssig, hash[:])
	if !ok {
		return errors.New("SchnorrVerify failed")
	}

	println("addr ok")
	return nil
}

// DeriveNextPrivate is used for implementing Type-2 determinitic keys.
// B_private_key = ( A_private_key + secret ) % N
func DeriveNextPrivate(p, s []byte) (toreturn []byte) {
	var prv, secret big.Int
	prv.SetBytes(p)
	secret.SetBytes(s)
	res := new(big.Int).Mod(new(big.Int).Add(&prv, &secret), &secp256k1.TheCurve.Order.Int).Bytes()
	toreturn = make([]byte, 32)
	copy(toreturn[32-len(res):], res)
	return
}

// DeriveNextPublic is used for implementing Type-2 determinitic keys.
// B_public_key = G * secret + A_public_key
func DeriveNextPublic(public, secret []byte) (out []byte) {
	out = make([]byte, len(public))
	secp256k1.BaseMultiplyAdd(public, secret, out)
	return
}

// NewSpendOutputs returns one TxOut record.
func NewSpendOutputs(addr *BtcAddr, amount uint64, testnet bool) ([]*TxOut, error) {
	out := new(TxOut)
	out.Value = amount
	out.Pk_script = addr.OutScript()
	return []*TxOut{out}, nil
}

// PrivateAddr is a Base58 encoded private address with checksum and its corresponding public key/address.
type PrivateAddr struct {
	Version byte
	Key     []byte
	*BtcAddr
}

func NewPrivateAddr(key []byte, ver byte, compr bool) (ad *PrivateAddr) {
	ad = new(PrivateAddr)
	ad.Version = ver
	ad.Key = key
	pub := PublicFromPrivate(key, compr)
	if pub == nil {
		panic("PublicFromPrivate error")
	}
	ad.BtcAddr = NewAddrFromPubkey(pub, ver-0x80)
	return
}

func DecodePrivateAddr(s string) (*PrivateAddr, error) {
	pkb := Decodeb58(s)

	if pkb == nil {
		return nil, errors.New("Decodeb58 failed")
	}

	if len(pkb) < 37 {
		return nil, errors.New("Decoded data too short")
	}

	if len(pkb) > 38 {
		return nil, errors.New("Decoded data too long")
	}

	var sh [32]byte
	ShaHash(pkb[:len(pkb)-4], sh[:])
	if !bytes.Equal(sh[:4], pkb[len(pkb)-4:]) {
		return nil, errors.New("Checksum error")
	}

	return NewPrivateAddr(pkb[1:33], pkb[0], len(pkb) == 38 && pkb[33] == 1), nil
}

// String returns the Base58 encoded private key (with checksum).
func (ad *PrivateAddr) String() string {
	var ha [32]byte
	buf := new(bytes.Buffer)
	buf.WriteByte(ad.Version)
	buf.Write(ad.Key)
	if ad.BtcAddr.IsCompressed() {
		buf.WriteByte(1)
	}
	ShaHash(buf.Bytes(), ha[:])
	buf.Write(ha[:4])
	return Encodeb58(buf.Bytes())
}
