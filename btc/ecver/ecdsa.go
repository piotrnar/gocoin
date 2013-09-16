package ecver

import (
	"encoding/hex"
)

type secp256k1_ecdsa_sig_t struct {
	r, s secp256k1_num_t
}

func (sig *secp256k1_ecdsa_sig_t) print(lab string) {
	println("sig." + lab + ".R:", hex.EncodeToString(sig.r.Bytes()))
	println("sig." + lab + ".S:", hex.EncodeToString(sig.s.Bytes()))
}

func (sig *secp256k1_ecdsa_sig_t) recompute(pkey *secp256k1_ge_t, msg *secp256k1_num_t) *secp256k1_num_t {
	if sig.r.Sign()<=0 || sig.s.Sign()<=0 {
		return nil
	}

	if sig.r.Cmp(secp256k1.N) >= 0 || sig.s.Cmp(secp256k1.N) >= 0 {
		return nil
	}

	sn := sig.s.mod_inverse(&order)

	u1 := sn.mod_mul(msg, &order)
	u2 := sn.mod_mul(&sig.r, &order)

	var pubkeyj secp256k1_gej_t
	pubkeyj.set_ge(pkey)

	pr := secp256k1_ecmult(&pubkeyj, u2, u1)

	if pr.infinity {
		return nil
	}

	var xr secp256k1_fe_t
	pr.get_x_p(&xr)

	return xr.num()
}


func (sig *secp256k1_ecdsa_sig_t) verify(pkey *secp256k1_ge_t, msg *secp256k1_num_t) bool {
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
	beta secp256k1_fe_t
	lambda, a1b2, b1, a2 secp256k1_num_t
	order secp256k1_num_t
)


func init() {
	beta.SetString(_beta, 16)
	lambda.SetString(_lambda, 16)
	a1b2.SetString(_a1b2, 16)
	b1.SetString(_b1, 16)
	a2.SetString(_a2, 16)
	order.Set(secp256k1.N)
	secp256k1_ecmult_start()
}
