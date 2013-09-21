package newec

func secp256k1_ecdsa_verify(msg, sig, pubkey []byte) int {
	var m num_t
	var s ecdsa_sig_t
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
