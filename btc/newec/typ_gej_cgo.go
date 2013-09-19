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

func (a *gej_t) _ecmult(r *gej_t, na, ng *num_t) {
	println("gej.ecmult not implemented")
	na_bn := na.get_bn()
	ng_bn := ng.get_bn()
	C.secp256k1_ecmult(unsafe.Pointer(&r.x), unsafe.Pointer(&a.x), na_bn, ng_bn)
}

func (a *gej_t) double(r *gej_t) {
	C.secp256k1_gej_double(unsafe.Pointer(r), unsafe.Pointer(a))
}

func (a *gej_t) add_ge(r *gej_t, b *ge_t) {
	C.secp256k1_gej_add_ge(unsafe.Pointer(r), unsafe.Pointer(a), unsafe.Pointer(b))
}

func (a *gej_t) add(r, b *gej_t) {
	C.secp256k1_gej_add(unsafe.Pointer(r), unsafe.Pointer(a), unsafe.Pointer(b))
}

func (a *gej_t) mul_lambda(r *gej_t) {
	C.secp256k1_gej_mul_lambda(unsafe.Pointer(r), unsafe.Pointer(a))
}


/*
func (a *gej_t) get_x(r *fe_t) {
	C.secp256k1_gej_get_x(unsafe.Pointer(r), unsafe.Pointer(&a.x))
}
*/