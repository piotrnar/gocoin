package secp256k1

import (
	"bytes"
	"encoding/hex"
	"fmt"
)

type Signature struct {
	R, S Number
}

func (s *Signature) Print(lab string) {
	fmt.Println(lab+".R:", hex.EncodeToString(s.R.Bytes()))
	fmt.Println(lab+".S:", hex.EncodeToString(s.S.Bytes()))
}

func (r *Signature) ParseBytes(sig []byte) int {
	if len(sig) < 5 || sig[0] != 0x30 {
		return -1
	}

	lenr := int(sig[3])
	if lenr == 0 || 5+lenr >= len(sig) || sig[lenr+4] != 0x02 {
		return -1
	}

	lens := int(sig[lenr+5])
	if lens == 0 || int(sig[1]) != lenr+lens+4 || lenr+lens+6 > len(sig) || sig[2] != 0x02 {
		return -1
	}

	r.R.SetBytes(sig[4 : 4+lenr])
	r.S.SetBytes(sig[6+lenr : 6+lenr+lens])
	return 6 + lenr + lens
}

// Verify checks if sig.R equals (mod N) to the X coordinate of the point: (message/S)*G + (R/S)*pubkey
func (sig *Signature) Verify(pubkey *XY, message *Number) bool {
	var sn, u1, u2 Number

	// The X coordinate gets reduced mod N, so R outside of the range <0, N) can never match it
	if sig.R.Sign() < 0 || sig.R.Cmp(&TheCurve.Order.Int) >= 0 {
		return false
	}

	sn.mod_inv(&sig.S, &TheCurve.Order)
	u1.mod_mul(&sn, message, &TheCurve.Order)
	u2.mod_mul(&sn, &sig.R, &TheCurve.Order)

	var pr, pubkeyj XYZ
	pubkeyj.SetXY(pubkey)

	pubkeyj.ECmult(&pr, &u2, &u1)
	if pr.IsInfinity() {
		return false
	}

	// Instead of converting the point to affine coordinates (which needs a costly field inversion)
	// and comparing its X (mod N) with R, check which X values would match R and compare them
	// with the point's jacobian X. As X is below P, the only candidates are R and (if below P) R+N.
	var xrb [32]byte
	var xr Field
	sig.R.FillBytes(xrb[:])
	xr.SetB32(xrb[:])
	if pr.eq_x(&xr) {
		return true
	}
	if sig.R.Cmp(&TheCurve.p_minus_n.Int) < 0 {
		var rn Number
		rn.Add(&sig.R.Int, &TheCurve.Order.Int)
		rn.FillBytes(xrb[:])
		xr.SetB32(xrb[:])
		return pr.eq_x(&xr)
	}
	return false
}

func (sig *Signature) recover(pubkey *XY, m *Number, recid int) (ret bool) {
	var rx, rn, u1, u2 Number
	var fx Field
	var X XY
	var xj, qj XYZ

	rx.Set(&sig.R.Int)
	if (recid & 2) != 0 {
		rx.Add(&rx.Int, &TheCurve.Order.Int)
		if rx.Cmp(&TheCurve.p.Int) >= 0 {
			return false
		}
	}

	fx.SetB32(rx.get_bin(32))

	X.SetXO(&fx, (recid&1) != 0)
	if !X.IsValid() {
		return false
	}

	xj.SetXY(&X)
	rn.mod_inv(&sig.R, &TheCurve.Order)
	u1.mod_mul(&rn, m, &TheCurve.Order)
	u1.Sub(&TheCurve.Order.Int, &u1.Int)
	u2.mod_mul(&rn, &sig.S, &TheCurve.Order)
	xj.ECmult(&qj, &u2, &u1)
	pubkey.SetXYZ(&qj)

	return true
}

func (sig *Signature) Sign(seckey, message, nonce *Number, recid *int) int {
	var r XY
	var rp XYZ
	var n Number
	var b [32]byte

	ECmultGen(&rp, nonce)
	r.SetXYZ(&rp)
	r.X.Normalize()
	r.Y.Normalize()
	r.X.GetB32(b[:])
	sig.R.SetBytes(b[:])
	if recid != nil {
		*recid = 0
		if sig.R.Cmp(&TheCurve.Order.Int) >= 0 {
			*recid |= 2
		}
		if r.Y.IsOdd() {
			*recid |= 1
		}
	}
	sig.R.mod(&TheCurve.Order)
	n.mod_mul(&sig.R, seckey, &TheCurve.Order)
	n.Add(&n.Int, &message.Int)
	n.mod(&TheCurve.Order)
	sig.S.mod_inv(nonce, &TheCurve.Order)
	sig.S.mod_mul(&sig.S, &n, &TheCurve.Order)
	if sig.S.Sign() == 0 {
		return 0
	}
	if sig.S.is_odd() {
		sig.S.Sub(&TheCurve.Order.Int, &sig.S.Int)
		if recid != nil {
			*recid ^= 1
		}
	}

	if FORCE_LOW_S && sig.S.Cmp(&TheCurve.HalfOrder.Int) == 1 {
		sig.S.Sub(&TheCurve.Order.Int, &sig.S.Int)
		if recid != nil {
			*recid ^= 1
		}
	}

	return 1
}

func (sig *Signature) Bytes() []byte {
	r := sig.R.Bytes()
	if r[0] >= 0x80 {
		r = append([]byte{0}, r...)
	}
	s := sig.S.Bytes()
	if s[0] >= 0x80 {
		s = append([]byte{0}, s...)
	}
	res := new(bytes.Buffer)
	res.WriteByte(0x30)
	res.WriteByte(byte(4 + len(r) + len(s)))
	res.WriteByte(0x02)
	res.WriteByte(byte(len(r)))
	res.Write(r)
	res.WriteByte(0x02)
	res.WriteByte(byte(len(s)))
	res.Write(s)
	return res.Bytes()
}
