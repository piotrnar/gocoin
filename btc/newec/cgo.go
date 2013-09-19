package newec

/*
void secp256k1_fe_sqrt(void *r, void *a);
void secp256k1_gej_double(void *r, void *a);
void secp256k1_gej_add_ge(void *r, void *a, void *b);
void secp256k1_gej_add(void *r, void *a, void *b);
void bytes2bn(void *out, void *bytes, int len);
*/
import "C"
import "unsafe"

func (a *fe_t) sqrt(r *fe_t) {
	C.secp256k1_fe_sqrt(unsafe.Pointer(r), unsafe.Pointer(a));
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

func (r *ecdsa_sig_t) get_bns() unsafe.Pointer {
	res := make([]byte, 64)
	dat := r.r.Bytes()
	C.bytes2bn(unsafe.Pointer(&res[0]), unsafe.Pointer(&dat[0]), C.int(len(dat)))
	dat = r.s.Bytes()
	C.bytes2bn(unsafe.Pointer(&res[24]), unsafe.Pointer(&dat[0]), C.int(len(dat)))
	return unsafe.Pointer(&res[0])
}
