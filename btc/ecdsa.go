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
	//var res [1]byte
	var buf [256]byte
	conn, e := net.DialTCP("tcp4", nil, EcdsaServer)
	if e != nil {
		println(e.Error())
		return false
	}
	buf[0] = 1
	buf[1] = byte(len(kd))
	buf[2] = byte(len(sd))
	copy(buf[16:], kd)
	copy(buf[128:], sd)
	copy(buf[224:], h)
	_, e = conn.Write(buf[:])
	if e != nil {
		println("NetVerify", e.Error())
		return false
	}
	_, e = conn.Read(buf[:1])
	conn.Close()
	if e != nil {
		println("NetVerify", e.Error())
		return false
	}
	return buf[0]!=0
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
	if len(kd)>65 || len(sd)>96 || len(hash)!=32 {
		println("EcdsaVerify input len error", len(kd), len(sd), len(hash))
		return false
	}
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
