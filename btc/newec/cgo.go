package newec

/*
#cgo windows LDFLAGS: openssl/libcrypto.a
#cgo !windows LDFLAGS: -lcrypto
#cgo CFLAGS: -Igmp -std="gnu99"

#include "secp256k1.h"

void secp256k1_gej_add_ge(void *r, void *a, void *b);
void secp256k1_gej_add(void *r, void *a, void *b);
*/
import "C"
import "unsafe"

func (a *gej_t) add(r, b *gej_t) {
	C.secp256k1_gej_add(unsafe.Pointer(r), unsafe.Pointer(a), unsafe.Pointer(b))
}

func LIB_Verify(pkey, sign, hash []byte) int {
	return int(C.secp256k1_ecdsa_verify((*C.uchar)(unsafe.Pointer(&hash[0])), C.int(32),
		(*C.uchar)(unsafe.Pointer(&sign[0])), C.int(len(sign)),
		(*C.uchar)(unsafe.Pointer(&pkey[0])), C.int(len(pkey))))
}

func cgo_start() {
	C.secp256k1_start()
}

/*
func (a *gej_t) add_ge(r *gej_t, b *ge_t) {
	C.secp256k1_gej_add_ge(unsafe.Pointer(r), unsafe.Pointer(a), unsafe.Pointer(b))
}
*/
