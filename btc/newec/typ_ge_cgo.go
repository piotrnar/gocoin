package newec

/*
void secp256k1_ge_set_xo(void *r, void *x, int odd);
*/
import "C"

import (
	"unsafe"
)

func (r *ge_t) ___set_xo(x *fe_t, odd bool) {
	if odd {
		C.secp256k1_ge_set_xo(unsafe.Pointer(r), unsafe.Pointer(r), 1)
	} else {
		C.secp256k1_ge_set_xo(unsafe.Pointer(r), unsafe.Pointer(r), 0)
	}
}
