package btc

import (
	"net"
	"sync/atomic"
	"crypto/ecdsa"
)

var (
	EcdsaServer *net.TCPAddr
	VerScriptCnt uint64
)

// Use ECDSA_server
func NetVerify(kd []byte, sd []byte, h []byte) bool {
	var res [1]byte
	conn, e := net.DialTCP("tcp4", nil, EcdsaServer)
	if e != nil {
		println(e.Error())
		return false
	}
	conn.Write([]byte{1})
	conn.Write([]byte{byte(len(kd))})
	conn.Write(kd)
	conn.Write([]byte{byte(len(sd))})
	conn.Write(sd)
	conn.Write(h[0:32])
	conn.Read(res[:])
	conn.Close()
	return res[0]!=0
}


// Use crypto/ecdsa
func NormalVerify(kd []byte, sd []byte, h []byte) bool {
	pk, e := NewPublicKey(kd)
	if e != nil {
		return false
	}
	s, e := NewSignature(sd)
	if e != nil {
		return false
	}
	return ecdsa.Verify(&pk.PublicKey, h, s.R, s.S)
}


func EcdsaVerify(kd []byte, sd []byte, hash []byte) bool {
	atomic.AddUint64(&VerScriptCnt, 1)
	if EcdsaServer!=nil {
		return NetVerify(kd, sd, hash)
	}
	return NormalVerify(kd, sd, hash)
}


/*
func (pk *PublicKey) Verify(h []byte, s *Signature) (ok bool) {
	if don(DBG_VERIFY) {
		fmt.Println("Verify signature, HashType", s.HashType)
		fmt.Println("R:", hex.EncodeToString(s.R.Bytes()))
		fmt.Println("S:", hex.EncodeToString(s.S.Bytes()))
		fmt.Println("Hash:", hex.EncodeToString(h))
		fmt.Println("Key:", hex.EncodeToString(pk.Bytes(false)))
	}
	ok = ecdsa.Verify(&pk.PublicKey, h, s.R, s.S)
	if don(DBG_VERIFY) {
		fmt.Println("Verify signature =>", ok)
	}
	return
}
*/
