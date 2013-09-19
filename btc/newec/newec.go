package newec

func secp256k1_ecdsa_verify(msg, sig, pubkey []byte) (ret int) {
	ret = -3
	var m num_t
	m.init()
	var s ecdsa_sig_t
	s.init()
	m.SetBytes(msg)

	var q ge_t
	if !q.pubkey_parse(pubkey) {
		ret = -1
		goto end
	}

	if !s.sig_parse(sig) {
		ret = -2
		goto end
	}

	if !s.sig_verify(&q, &m) {
		ret = 0
		goto end
	}
	ret = 1

end:
	s.free()
	m.free()
	return
}

// Verify ECDSA signature
func EC_Verify(pkey, sign, hash []byte) int {
	return secp256k1_ecdsa_verify(hash, sign, pkey)
}

func Verify(k, s, m []byte) bool {
	return secp256k1_ecdsa_verify(m, s, k)==1
}

func init() {
	cgo_start()
	init_contants()
	ecmult_start()
}
