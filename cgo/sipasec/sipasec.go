package sipasec

/*
#cgo CFLAGS: -I .
#cgo LDFLAGS: -L . libsecp256k1.a libgmp.a
#cgo windows LDFLAGS: libgmp.a
#cgo linux darwin LDFLAGS: -lgmp

#include <stdio.h>
#include "secp256k1.h"

*/
import "C"
import "unsafe"


// Verify ECDSA signature
func EC_Verify(pkey, sign, hash []byte) int {
	h := (*C.uchar)(unsafe.Pointer(&hash[0]))
	s := (*C.uchar)(unsafe.Pointer(&sign[0]))
	k := (*C.uchar)(unsafe.Pointer(&pkey[0]))
	i := C.secp256k1_ecdsa_verify(h, C.int(32), s, C.int(len(sign)), k, C.int(len(pkey)))
	return int(i)
}

func init() {
	C.secp256k1_start()
}
