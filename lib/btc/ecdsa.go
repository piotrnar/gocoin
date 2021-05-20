package btc

import (
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"github.com/piotrnar/gocoin/lib/secp256k1"
	"math/big"
	"sync/atomic"
)

var (
	ecdsaVerifyCnt    uint64
	schnorrVerifyCnt  uint64
	checkp2cVerifyCnt uint64

	EC_Verify           func(k, s, h []byte) bool
	Schnorr_Verify      func(pkey, sign, msg []byte) bool
	Check_PayToContract func(m_keydata, base, hash []byte, parity bool) bool
)

func EcdsaVerifyCnt() uint64 {
	return atomic.LoadUint64(&ecdsaVerifyCnt)
}

func SchnorrVerifyCnt() uint64 {
	return atomic.LoadUint64(&schnorrVerifyCnt)
}

func CheckPay2ContractCnt() uint64 {
	return atomic.LoadUint64(&checkp2cVerifyCnt)
}

func EcdsaVerify(kd []byte, sd []byte, hash []byte) bool {
	atomic.AddUint64(&ecdsaVerifyCnt, 1)
	if len(kd) == 0 || len(sd) == 0 {
		return false
	}
	if EC_Verify != nil {
		return EC_Verify(kd, sd, hash)
	}
	return secp256k1.Verify(kd, sd, hash)
}

func EcdsaSign(priv, hash []byte) (r, s *big.Int, err error) {
	var sig secp256k1.Signature
	var sec, msg, nonce secp256k1.Number

	sec.SetBytes(priv)
	msg.SetBytes(hash)

	sha := sha256.New()
	sha.Write(priv)
	sha.Write(hash)
	for {
		var buf [32]byte
		rand.Read(buf[:])
		sha.Write(buf[:])
		nonce.SetBytes(sha.Sum(nil))
		if nonce.Sign() > 0 && nonce.Cmp(&secp256k1.TheCurve.Order.Int) < 0 {
			break
		}
	}

	if sig.Sign(&sec, &msg, &nonce, nil) != 1 {
		err = errors.New("ESCDS Sign error()")
	}
	return &sig.R.Int, &sig.S.Int, nil
}

func SchnorrVerify(pkey, sig, msg []byte) bool {
	atomic.AddUint64(&schnorrVerifyCnt, 1)
	if Schnorr_Verify != nil {
		return Schnorr_Verify(pkey, sig, msg)
	}
	return secp256k1.SchnorrVerify(pkey, sig, msg)
}

func CheckPayToContract(m_keydata, base, hash []byte, parity bool) bool {
	atomic.AddUint64(&checkp2cVerifyCnt, 1)
	if Check_PayToContract != nil {
		return Check_PayToContract(m_keydata, base, hash, parity)
	}
	return secp256k1.CheckPayToContract(m_keydata, base, hash, parity)
}
