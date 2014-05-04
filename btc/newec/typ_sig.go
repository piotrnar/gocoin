package newec

import (
//	"fmt"
)

type sig_t struct {
	r, s num_t
}

func (r *sig_t) sig_parse(sig []byte) bool {
	if sig[0] != 0x30 || len(sig) < 5 {
		return false
	}

	lenr := int(sig[3])
	if lenr == 0 || 5+lenr >= len(sig) || sig[lenr+4] != 0x02 {
		return false
	}

	lens := int(sig[lenr+5])
	if lens == 0 || int(sig[1]) != lenr+lens+4 || lenr+lens+6 > len(sig) || sig[2] != 0x02 {
		return false
	}

	r.r.SetBytes(sig[4 : 4+lenr])
	r.s.SetBytes(sig[6+lenr : 6+lenr+lens])
	return true
}

func (r *sig_t) sig_verify(pubkey *ge_t, message *num_t) (ret bool) {
	var r2 num_t
	ret = r.sig_recompute(&r2, pubkey, message) && r.r.Cmp(r2.big()) == 0
	return
}

func (sig *sig_t) sig_recompute(r2 *num_t, pubkey *ge_t, message *num_t) (ret bool) {
	var sn, u1, u2 num_t

	sn.mod_inv(&sig.s, &secp256k1.order)
	u1.mod_mul(&sn, message, &secp256k1.order)
	u2.mod_mul(&sn, &sig.r, &secp256k1.order)

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
		r2.Mod(r2.big(), secp256k1.order.big())
		ret = true
	}

	return
}

func (sig *sig_t) recover(pubkey *ge_t, m *num_t, recid int) (ret bool) {
	var rx, rn, u1, u2 num_t
	var fx fe_t
	var x ge_t
	var xj, qj gej_t

	if sig.r.sign()<=0 || sig.s.sign()<=0 {
		return false
	}
	if sig.r.cmp(&secp256k1.order)>=0 || sig.s.cmp(&secp256k1.order)>=0 {
		return false
	}

	rx.Set(sig.r.big())
	if (recid&2)!=0 {
		rx.add(&rx, &secp256k1.order)
		if rx.cmp(&secp256k1.p) >= 0 {
			return false
		}
	}

	fx.set_b32(rx.get_bin(32))

	x.set_xo(&fx, (recid&1)!=0)
	if !x.is_valid() {
		return false
	}


	xj.set_ge(&x)
	rn.mod_inv(&sig.r, &secp256k1.order)
	u1.mod_mul(&rn, m, &secp256k1.order)
	u1.sub(&secp256k1.order, &u1)
	u2.mod_mul(&rn, &sig.s, &secp256k1.order)
	xj.ecmult(&qj, &u2, &u1)
	pubkey.set_gej(&qj)

	return true
}
