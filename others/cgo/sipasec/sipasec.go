package sipasec

/*
#cgo CFLAGS: -I /DEV/secp256k1/include
#cgo windows LDFLAGS: /DEV/secp256k1/libsecp256k1.a /DEV/secp256k1/gmp/libgmp.a
#cgo !windows LDFLAGS: -lsecp256k1 -lgmp

#include <stdio.h>
#include "secp256k1.h"

*/
import "C"
import "unsafe"


// Verify ECDSA signature
func EC_Verify(pkey, sign, hash []byte) int {
	return int(C.secp256k1_ecdsa_verify((*C.uchar)(unsafe.Pointer(&hash[0])), C.int(32),
		(*C.uchar)(unsafe.Pointer(&sign[0])), C.int(len(sign)),
		(*C.uchar)(unsafe.Pointer(&pkey[0])), C.int(len(pkey))))
}

func init() {
	C.secp256k1_start()
}
