package secp256k1

import (
	"fmt"
)

type Signature struct {
	r, s Number
}

func (s *Signature) Print(lab string) {
	fmt.Println(lab + ".R:", s.r.String())
	fmt.Println(lab + ".S:", s.s.String())
}

func (r *Signature) sig_parse(sig []byte) bool {
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

func (r *Signature) Verify(pubkey *ge_t, message *Number) (ret bool) {
	var r2 Number
	ret = r.recompute(&r2, pubkey, message) && r.r.Cmp(&r2.Int) == 0
	return
}

func (sig *Signature) recompute(r2 *Number, pubkey *ge_t, message *Number) (ret bool) {
	var sn, u1, u2 Number

	sn.mod_inv(&sig.s, &TheCurve.order)
	u1.mod_mul(&sn, message, &TheCurve.order)
	u2.mod_mul(&sn, &sig.r, &TheCurve.order)

	var pr, pubkeyj gej_t
	pubkeyj.set_ge(pubkey)

	pubkeyj.ecmult(&pr, &u2, &u1)
	if !pr.is_infinity() {
		var xr Fe_t
		pr.get_x(&xr)
		xr.Normalize()
		var xrb [32]byte
		xr.GetB32(xrb[:])
		r2.SetBytes(xrb[:])
		r2.Mod(&r2.Int, &TheCurve.order.Int)
		ret = true
	}

	return
}

func (sig *Signature) recover(pubkey *ge_t, m *Number, recid int) (ret bool) {
	var rx, rn, u1, u2 Number
	var fx Fe_t
	var x ge_t
	var xj, qj gej_t

	if sig.r.Sign()<=0 || sig.s.Sign()<=0 {
		return false
	}
	if sig.r.Cmp(&TheCurve.order.Int)>=0 || sig.s.Cmp(&TheCurve.order.Int)>=0 {
		return false
	}

	rx.Set(&sig.r.Int)
	if (recid&2)!=0 {
		rx.Add(&rx.Int, &TheCurve.order.Int)
		if rx.Cmp(&TheCurve.p.Int) >= 0 {
			return false
		}
	}

	fx.SetB32(rx.get_bin(32))

	x.set_xo(&fx, (recid&1)!=0)
	if !x.is_valid() {
		return false
	}


	xj.set_ge(&x)
	rn.mod_inv(&sig.r, &TheCurve.order)
	u1.mod_mul(&rn, m, &TheCurve.order)
	u1.Sub(&TheCurve.order.Int, &u1.Int)
	u2.mod_mul(&rn, &sig.s, &TheCurve.order)
	xj.ecmult(&qj, &u2, &u1)
	pubkey.set_gej(&qj)

	return true
}


func (sig *Signature) Sign(seckey, message, nonce *Number, recid *int) int {
	var r ge_t
	var rp gej_t
	var n Number
	var b [32]byte

	ecmult_gen(&rp, nonce)
	r.set_gej(&rp)
	r.x.Normalize()
	r.y.Normalize()
	r.x.GetB32(b[:])
	sig.r.SetBytes(b[:])
	if recid != nil {
		*recid = 0
		if sig.r.Cmp(&TheCurve.order.Int) >= 0 {
			*recid |= 2
		}
		if r.y.IsOdd() {
			*recid |= 1
		}
	}
	sig.r.mod(&TheCurve.order)
	n.mod_mul(&sig.r, seckey, &TheCurve.order)
	n.Add(&n.Int, &message.Int)
	n.mod(&TheCurve.order)
	sig.s.mod_inv(nonce, &TheCurve.order)
	sig.s.mod_mul(&sig.s, &n, &TheCurve.order)
	if sig.s.Sign()==0 {
		return 0
	}
	if sig.s.IsOdd() {
		sig.s.Sub(&TheCurve.order.Int, &sig.s.Int)
		if recid!=nil {
			*recid ^= 1
		}
	}
	return 1
}
