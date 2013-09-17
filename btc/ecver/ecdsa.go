package ecver

import (
	"encoding/hex"
)

type sig_t struct {
	r, s num_t
}

func (sig *sig_t) print(lab string) {
	println("sig." + lab + ".R:", hex.EncodeToString(sig.r.Bytes()))
	println("sig." + lab + ".S:", hex.EncodeToString(sig.s.Bytes()))
}

func (sig *sig_t) recompute(pkey *ge_t, msg *num_t) *num_t {
	if sig.r.Sign()<=0 || sig.s.Sign()<=0 {
		return nil
	}

	if sig.r.Cmp(secp256k1.N) >= 0 || sig.s.Cmp(secp256k1.N) >= 0 {
		return nil
	}

	sn := sig.s.mod_inverse(&order)

	u1 := sn.mod_mul(msg, &order)
	u2 := sn.mod_mul(&sig.r, &order)

	var pubkeyj gej_t
	pubkeyj.set_ge(pkey)

	pr := pubkeyj.ecmult(u2, u1)

	if pr.infinity {
		return nil
	}

	var xr fe_t
	pr.get_x_p(&xr)

	return xr.num()
}


func (sig *sig_t) verify(pkey *ge_t, msg *num_t) bool {
	r2 := sig.recompute(pkey, msg)
	if r2!=nil {
		return r2.equal(&sig.r)
	}
	return false
}


const (
	WINDOW_A = 5
	WINDOW_G = 14

	_beta = "7AE96A2B657C07106E64479EAC3434E99CF0497512F58995C1396C28719501EE"
	_lambda = "5363AD4CC05C30E0A5261C028812645A122E22EA20816678DF02967C1B23BD72"
	_a1b2 = "000000000000000000000000000000003086D221A7D46BCDE86C90E49284EB15"
	_b1 = "00000000000000000000000000000000E4437ED6010E88286F547FA90ABFE4C3"
	_a2 = "0000000000000000000000000000000114CA50F7A8E2F3F657C1108D9D44CFD8"
)

var (
	beta fe_t
	lambda, a1b2, b1, a2 num_t
	order num_t
)


func init() {
	beta.SetString(_beta, 16)
	lambda.SetString(_lambda, 16)
	a1b2.SetString(_a1b2, 16)
	b1.SetString(_b1, 16)
	a2.SetString(_a2, 16)
	order.Set(secp256k1.N)
	ecmult_start()
}
