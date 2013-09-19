package newec

/*
void secp256k1_ecmult(void *r, void *a, void *na, void *ng);
void secp256k1_gej_get_x(void *r, void *a);
void secp256k1_gej_double(void *r, void *a);
void secp256k1_gej_add_ge(void *r, void *a, void *b);
void secp256k1_gej_add(void *r, void *a, void *b);
void secp256k1_gej_mul_lambda(void *r, void *a);
*/
import "C"

import (
	"unsafe"
)

func (a *gej_t) double(r *gej_t) {
	C.secp256k1_gej_double(unsafe.Pointer(r), unsafe.Pointer(a))
}

func (a *gej_t) add_ge(r *gej_t, b *ge_t) {
	C.secp256k1_gej_add_ge(unsafe.Pointer(r), unsafe.Pointer(a), unsafe.Pointer(b))
}

func (a *gej_t) add(r, b *gej_t) {
	C.secp256k1_gej_add(unsafe.Pointer(r), unsafe.Pointer(a), unsafe.Pointer(b))
}

/*
func (a *gej_t) mul_lambda(r *gej_t) {
	C.secp256k1_gej_mul_lambda(unsafe.Pointer(r), unsafe.Pointer(a))
}

*/
