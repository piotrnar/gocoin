package newec

/*
#cgo windows LDFLAGS: openssl/libcrypto.a
#cgo !windows LDFLAGS: -lcrypto
#cgo CFLAGS: -Igmp -std="gnu99"

#include "secp256k1.h"

*/
import "C"
import "unsafe"

func secp256k1_ecdsa_verify(msg, sig, pubkey []byte) (ret int) {
	ret = -3
	var m num_t
	m.init()
	var s ecdsa_sig_t
	s.init()
	m.SetBytes(msg)

	var q ge_t
	if !q.pubkey_parse(pubkey) {
		ret = -1
		goto end
	}

	if !s.sig_parse(sig) {
		ret = -2
		goto end
	}

	if !s.sig_verify(&q, &m) {
		ret = 0
		goto end
	}
	ret = 1

end:
	s.free()
	m.free()
	return
}

// Verify ECDSA signature
func EC_Verify(pkey, sign, hash []byte) int {
	if 1==1 {
		return int(C.secp256k1_ecdsa_verify((*C.uchar)(unsafe.Pointer(&hash[0])), C.int(32),
			(*C.uchar)(unsafe.Pointer(&sign[0])), C.int(len(sign)),
			(*C.uchar)(unsafe.Pointer(&pkey[0])), C.int(len(pkey))))
	} else {
		return secp256k1_ecdsa_verify(hash, sign, pkey)
	}
}

func Verify(k, s, m []byte) bool {
	return secp256k1_ecdsa_verify(m, s, k)==1
}

func init() {
	C.secp256k1_start()
	init_contants()
	ecmult_start()
}
