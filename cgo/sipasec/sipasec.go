package sipasec

/*
#cgo !windows LDFLAGS: -lsecp256k1 -lgmp
#cgo windows LDFLAGS: libsecp256k1.a libgmp.a

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
