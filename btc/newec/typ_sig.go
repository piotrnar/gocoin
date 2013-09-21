package newec

import (
//	"fmt"
)

type ecdsa_sig_t struct {
	r, s num_t
}

func (r *ecdsa_sig_t) sig_parse(sig []byte) bool {
	if sig[0] != 0x30 || len(sig) < 5 {
		return false
	}

	lenr := int(sig[3])

	if 5+lenr >= len(sig) {
		return false
	}

	lens := int(sig[lenr+5])

	if int(sig[1]) != lenr+lens+4 {
		return false
	}

	if lenr+lens+6 > len(sig) {
		return false
	}

	if sig[2] != 0x02 {
		return false
	}

	if lenr == 0 {
		return false
	}

	if sig[lenr+4] != 0x02 {
		return false
	}

	if lens == 0 {
		return false
	}

	r.r.SetBytes(sig[4:4+lenr])
	r.s.SetBytes(sig[6+lenr:6+lenr+lens])
	return true
}


func (r *ecdsa_sig_t) sig_verify(pubkey *ge_t, message *num_t) (ret bool) {
	var r2 num_t;
	ret = r.sig_recompute(&r2, pubkey, message) && r.r.Cmp(r2.big())==0
	return
}


func (sig *ecdsa_sig_t) sig_recompute(r2 *num_t, pubkey *ge_t, message *num_t) (ret bool) {
	if sig.r.Sign()<=0 || sig.s.Sign()<=0 {
		return false
	}
	if sig.r.Cmp(order.big()) >= 0 || sig.s.Cmp(order.big()) >= 0 {
		return false
	}

	var sn, u1, u2 num_t

	sn.ModInverse(sig.s.big(), order.big())
	u1.mod_mul(&sn, message, &order)
	u2.mod_mul(&sn, &sig.r, &order)

	var pr, pubkeyj gej_t
	pubkeyj.set_ge(pubkey)

	pubkeyj.ecmult(&pr, &u2, &u1)
	if !pr.is_infinity() {
		var xr fe_t
		pr.get_x(&xr)
		xr.normalize()
		var xrb [32]byte
		xr.get_b32(xrb[:])
		r2.SetBytes(xrb[:])
		r2.Mod(r2.big(), order.big())
		ret = true
	}

	return
}
