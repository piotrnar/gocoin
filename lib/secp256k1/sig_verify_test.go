package secp256k1

import (
	"math/big"
	"math/rand"
	"testing"
)

// verify_ref is the original (slower) implementation of Signature.Verify,
// which converts the result to affine coordinates, to get its X mod N.
func verify_ref(sig *Signature, pubkey *XY, message *Number) bool {
	var sn, u1, u2, r2 Number

	sn.mod_inv(&sig.S, &TheCurve.Order)
	u1.mod_mul(&sn, message, &TheCurve.Order)
	u2.mod_mul(&sn, &sig.R, &TheCurve.Order)

	var pr, pubkeyj XYZ
	pubkeyj.SetXY(pubkey)

	pubkeyj.ECmult(&pr, &u2, &u1)
	if pr.IsInfinity() {
		return false
	}
	var xr Field
	pr.get_x(&xr)
	xr.Normalize()
	var xrb [32]byte
	xr.GetB32(xrb[:])
	r2.SetBytes(xrb[:])
	r2.Mod(&r2.Int, &TheCurve.Order.Int)
	return sig.R.Cmp(&r2.Int) == 0
}

func rand_number(rnd *rand.Rand, bytes int) (n Number) {
	b := make([]byte, bytes)
	rnd.Read(b)
	n.SetBytes(b)
	return
}

func rand_scalar(rnd *rand.Rand) (n Number) {
	for {
		n = rand_number(rnd, 32)
		if !n.is_zero() && n.is_below(&TheCurve.Order) {
			return
		}
	}
}

func TestWNAFRandom(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	var wnaf [258]int
	check := func(a *Number, w uint) {
		for i := range wnaf {
			wnaf[i] = 0x55555555 // make sure it does not rely on the buffer being zeroed
		}
		cnt := ecmult_wnaf(wnaf[:], a, w)
		if cnt > a.BitLen()+1 {
			t.Fatal("wnaf too long", cnt, a.String(), w)
		}
		if cnt > 0 && wnaf[cnt-1] == 0 {
			t.Fatal("wnaf ends with zero", a.String(), w)
		}
		sum := new(big.Int)
		for i := cnt - 1; i >= 0; i-- {
			sum.Lsh(sum, 1)
			d := wnaf[i]
			if d != 0 && (d&1 == 0 || d >= 1<<(w-1) || d <= -(1<<(w-1))) {
				t.Fatal("bad wnaf digit", d, "at", i, a.String(), w)
			}
			sum.Add(sum, big.NewInt(int64(d)))
		}
		if sum.Cmp(&a.Int) != 0 {
			t.Fatal("wnaf does not sum up to", a.String(), w, "but to", sum.String())
		}
	}

	var a Number
	for _, w := range []uint{2, 3, WINDOW_A, 8, WINDOW_G} {
		check(&a, w) // zero
		for v := int64(-300); v <= 300; v++ {
			a.SetInt64(v)
			check(&a, w)
		}
		for i := 0; i < 5000; i++ {
			a = rand_number(rnd, 1+rnd.Intn(32))
			if rnd.Intn(4) == 0 {
				// long runs of ones, to exercise the carry
				a.Int.SetBit(a.Int.Lsh(big.NewInt(1), uint(1+rnd.Intn(256))), 0, 0)
				a.Int.Sub(&a.Int, big.NewInt(int64(rnd.Intn(3))))
			}
			check(&a, w)
			a.Int.Neg(&a.Int)
			check(&a, w)
		}
	}

	a = rand_number(rnd, 16)
	a.Int.Neg(&a.Int)
	if n := testing.AllocsPerRun(100, func() { ecmult_wnaf(wnaf[:], &a, WINDOW_A) }); n != 0 {
		t.Error("ecmult_wnaf allocates memory:", n)
	}
}

func TestSigVerifyRandom(t *testing.T) {
	rnd := rand.New(rand.NewSource(2))
	var sig, bad Signature
	var pubj XYZ
	var pub XY

	check := func(s *Signature, msg *Number, exp int, lab string) {
		res := s.Verify(&pub, msg)
		if ref := verify_ref(s, &pub, msg); res != ref {
			t.Fatal(lab, "- Verify returned", res, "but the reference implementation", ref)
		}
		if exp >= 0 && res != (exp == 1) {
			t.Fatal(lab, "- unexpected Verify result", res)
		}
	}

	for i := 0; i < 300; i++ {
		sec := rand_scalar(rnd)
		non := rand_scalar(rnd)
		msg := rand_number(rnd, 32)

		ECmultGen(&pubj, &sec)
		pub.SetXYZ(&pubj)
		pub.X.Normalize()
		pub.Y.Normalize()

		if sig.Sign(&sec, &msg, &non, nil) != 1 {
			t.Fatal("Sign failed")
		}
		check(&sig, &msg, 1, "valid")

		// S above the order is equivalent to S (it has always been like this)
		bad.R.Set(&sig.R.Int)
		bad.S.Add(&sig.S.Int, &TheCurve.Order.Int)
		check(&bad, &msg, 1, "S+N")

		// R above the order must never verify
		bad.S.Set(&sig.S.Int)
		bad.R.Add(&sig.R.Int, &TheCurve.Order.Int)
		check(&bad, &msg, 0, "R+N")

		bad.R.SetInt64(0)
		check(&bad, &msg, 0, "R=0")

		bad.R.Set(&sig.R.Int)
		bad.S.SetInt64(0)
		check(&bad, &msg, 0, "S=0")

		bad.S.Set(&sig.S.Int)
		bad.R.Add(&sig.R.Int, BigInt1)
		check(&bad, &msg, 0, "R+1")

		bad.R = rand_number(rnd, 1+rnd.Intn(33))
		bad.S = rand_number(rnd, 1+rnd.Intn(33))
		check(&bad, &msg, -1, "random")

		msg.Add(&msg.Int, BigInt1)
		check(&sig, &msg, 0, "msg+1")
	}
}

// Test the rare case when the X coordinate of the point is not below the order,
// so the signature's R is X-N (probability of this is about 2^-128 for a random signature).
func TestSigVerifyHighX(t *testing.T) {
	var x, r, s, sn, inv, msg, zero Number
	var fx Field
	var pt, pub XY
	var ptj, pubj XYZ
	var sig Signature

	rnd := rand.New(rand.NewSource(3))
	found := 0
	for x.Set(&TheCurve.Order.Int); x.Cmp(&TheCurve.p.Int) < 0 && found < 20; x.inc() {
		fx.SetB32(x.get_bin(32))
		pt.SetXO(&fx, found&1 != 0)
		if !pt.IsValid() {
			continue // no point with such X on the curve
		}
		found++

		// With the message of zero, the result is (R/S)*pubkey, so make pubkey = (S/R)*pt
		r.sub(&x, &TheCurve.Order)
		if r.is_zero() {
			continue
		}
		s = rand_scalar(rnd)
		sn.mod_inv(&s, &TheCurve.Order)
		inv.mod_mul(&sn, &r, &TheCurve.Order) // R/S
		inv.mod_inv(&inv, &TheCurve.Order)    // S/R
		ptj.SetXY(&pt)
		ptj.ECmult(&pubj, &inv, &zero)
		pub.SetXYZ(&pubj)
		pub.X.Normalize()
		pub.Y.Normalize()

		sig.R.Set(&r.Int)
		sig.S.Set(&s.Int)
		if !verify_ref(&sig, &pub, &msg) {
			t.Fatal("reference implementation rejects the signature with X =", x.Text(16))
		}
		if !sig.Verify(&pub, &msg) {
			t.Fatal("Verify rejects the signature with X =", x.Text(16))
		}

		// a different (low) R must not pass through the R+N path
		sig.R.inc()
		if sig.Verify(&pub, &msg) || verify_ref(&sig, &pub, &msg) {
			t.Fatal("Verify accepts a bad signature with X =", x.Text(16))
		}
	}
	if found < 20 {
		t.Fatal("Not enough points found", found)
	}
}

func TestGejEqX(t *testing.T) {
	rnd := rand.New(rand.NewSource(4))
	var pj XYZ
	var p XY
	for i := 0; i < 100; i++ {
		k := rand_scalar(rnd)
		ECmultGen(&pj, &k)
		cpy := pj
		p.SetXYZ(&cpy)
		p.X.Normalize()
		if !pj.eq_x(&p.X) {
			t.Fatal("eq_x does not match its own X")
		}
		var one Field
		one.SetInt(1)
		p.X.SetAdd(&one)
		if pj.eq_x(&p.X) {
			t.Fatal("eq_x matches X+1")
		}
	}
}
