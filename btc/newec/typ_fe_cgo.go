package newec

/*
void secp256k1_fe_sqr(void *r, void *a);
void secp256k1_fe_mul(void *r, void *a, void *b);
void secp256k1_fe_mul_int(void *r, int a);
void secp256k1_fe_add(void *r, void *a);
void secp256k1_fe_inv_var(void *r, void *a);
void secp256k1_fe_inv(void *r, void *a);
*/
import "C"
import "unsafe"


func (a *fe_t) sqr(r *fe_t) {
	C.secp256k1_fe_sqr(unsafe.Pointer(r), unsafe.Pointer(a));
}

func (a *fe_t) mul(r, b *fe_t) {
	C.secp256k1_fe_mul(unsafe.Pointer(r), unsafe.Pointer(a), unsafe.Pointer(b))
}

func (a *fe_t) inv_var(r *fe_t) {
	C.secp256k1_fe_inv_var(unsafe.Pointer(r), unsafe.Pointer(a));
}

func (a *fe_t) inv(r *fe_t) {
	C.secp256k1_fe_inv(unsafe.Pointer(r), unsafe.Pointer(a));
}

/*
func (r *fe_t) mul_int(a int) {
	C.secp256k1_fe_mul_int(unsafe.Pointer(r), C.int(a))
}

func (r *fe_t) add(a *fe_t) {
	C.secp256k1_fe_add(unsafe.Pointer(r), unsafe.Pointer(a))
}
*/