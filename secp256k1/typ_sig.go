package secp256k1

import (
	"fmt"
)

type Signature struct {
	R, S Number
}

func (s *Signature) Print(lab string) {
	fmt.Println(lab + ".R:", s.R.String())
	fmt.Println(lab + ".S:", s.S.String())
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

	r.R.SetBytes(sig[4 : 4+lenr])
	r.S.SetBytes(sig[6+lenr : 6+lenr+lens])
	return true
}

func (r *Signature) Verify(pubkey *XY, message *Number) (ret bool) {
	var r2 Number
	ret = r.recompute(&r2, pubkey, message) && r.R.Cmp(&r2.Int) == 0
	return
}

func (sig *Signature) recompute(r2 *Number, pubkey *XY, message *Number) (ret bool) {
	var sn, u1, u2 Number

	sn.mod_inv(&sig.S, &TheCurve.Order)
	u1.mod_mul(&sn, message, &TheCurve.Order)
	u2.mod_mul(&sn, &sig.R, &TheCurve.Order)

	var pr, pubkeyj XYZ_t
	pubkeyj.set_ge(pubkey)

	pubkeyj.ecmult(&pr, &u2, &u1)
	if !pr.is_infinity() {
		var xr Fe_t
		pr.get_x(&xr)
		xr.Normalize()
		var xrb [32]byte
		xr.GetB32(xrb[:])
		r2.SetBytes(xrb[:])
		r2.Mod(&r2.Int, &TheCurve.Order.Int)
		ret = true
	}

	return
}

func (sig *Signature) recover(pubkey *XY, m *Number, recid int) (ret bool) {
	var rx, rn, u1, u2 Number
	var fx Fe_t
	var x XY
	var xj, qj XYZ_t

	if sig.R.Sign()<=0 || sig.S.Sign()<=0 {
		return false
	}
	if sig.R.Cmp(&TheCurve.Order.Int)>=0 || sig.S.Cmp(&TheCurve.Order.Int)>=0 {
		return false
	}

	rx.Set(&sig.R.Int)
	if (recid&2)!=0 {
		rx.Add(&rx.Int, &TheCurve.Order.Int)
		if rx.Cmp(&TheCurve.p.Int) >= 0 {
			return false
		}
	}

	fx.SetB32(rx.get_bin(32))

	x.set_xo(&fx, (recid&1)!=0)
	if !x.IsValid() {
		return false
	}


	xj.set_ge(&x)
	rn.mod_inv(&sig.R, &TheCurve.Order)
	u1.mod_mul(&rn, m, &TheCurve.Order)
	u1.Sub(&TheCurve.Order.Int, &u1.Int)
	u2.mod_mul(&rn, &sig.S, &TheCurve.Order)
	xj.ecmult(&qj, &u2, &u1)
	pubkey.set_gej(&qj)

	return true
}


func (sig *Signature) Sign(seckey, message, nonce *Number, recid *int) int {
	var r XY
	var rp XYZ_t
	var n Number
	var b [32]byte

	ecmult_gen(&rp, nonce)
	r.set_gej(&rp)
	r.x.Normalize()
	r.y.Normalize()
	r.x.GetB32(b[:])
	sig.R.SetBytes(b[:])
	if recid != nil {
		*recid = 0
		if sig.R.Cmp(&TheCurve.Order.Int) >= 0 {
			*recid |= 2
		}
		if r.y.IsOdd() {
			*recid |= 1
		}
	}
	sig.R.mod(&TheCurve.Order)
	n.mod_mul(&sig.R, seckey, &TheCurve.Order)
	n.Add(&n.Int, &message.Int)
	n.mod(&TheCurve.Order)
	sig.S.mod_inv(nonce, &TheCurve.Order)
	sig.S.mod_mul(&sig.S, &n, &TheCurve.Order)
	if sig.S.Sign()==0 {
		return 0
	}
	if sig.S.IsOdd() {
		sig.S.Sub(&TheCurve.Order.Int, &sig.S.Int)
		if recid!=nil {
			*recid ^= 1
		}
	}
	return 1
}
