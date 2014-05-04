package newec

func secp256k1_ecdsa_verify(msg, sig, pubkey []byte) int {
	var m num_t
	var s sig_t
	m.SetBytes(msg)

	var q ge_t
	if !q.pubkey_parse(pubkey) {
		return -1
	}

	if !s.sig_parse(sig) {
		return -2
	}

	if !s.sig_verify(&q, &m) {
		return 0
	}
	return 1
}

// Verify ECDSA signature
func EC_Verify(pkey, sign, hash []byte) int {
	return secp256k1_ecdsa_verify(hash, sign, pkey)
}

func Verify(k, s, m []byte) bool {
	return secp256k1_ecdsa_verify(m, s, k)==1
}

func init() {
	init_contants()
	ecmult_start()
}

func DecompressPoint(x []byte, off bool, y []byte) {
	var rx, ry, c, x2, x3 fe_t
	rx.set_b32(x)
	rx.sqr(&x2)
	rx.mul(&x3, &x2)
	c.set_int(7)
	c.set_add(&x3)
	c.sqrt(&ry)
	ry.normalize()
	if ry.is_odd() != off {
		ry.negate(&ry, 1)
	}
	ry.normalize()
	ry.get_b32(y)
	return
}


func RecoverPublicKey(r, s, h []byte, recid int, x, y []byte) bool {
	var sig sig_t
	var pubkey ge_t
	var msg num_t
	sig.r.set_bytes(r)
	sig.s.set_bytes(s)
	msg.set_bytes(h)
	if !sig.recover(&pubkey, &msg, recid) {
		return false
	}
	pubkey.x.get_b32(x)
	pubkey.y.get_b32(y)
	return true
}
