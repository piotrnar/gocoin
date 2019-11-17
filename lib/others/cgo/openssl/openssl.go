package openssl

/*
#cgo !windows LDFLAGS: -lcrypto
#cgo windows LDFLAGS: /DEV/openssl-1.0.2a/libcrypto.a -lgdi32
#cgo windows CFLAGS: -I /DEV/openssl-1.0.2a/include

#include <stdio.h>
#include <openssl/ec.h>
#include <openssl/ecdsa.h>
#include <openssl/obj_mac.h>

int verify(void *pkey, unsigned int pkl, void *sign, unsigned int sil, void *hasz) {
	EC_KEY* ecpkey;
	ecpkey = EC_KEY_new_by_curve_name(NID_secp256k1);
	if (!ecpkey) {
		printf("EC_KEY_new_by_curve_name error!\n");
		return -1;
	}
	if (!o2i_ECPublicKey(&ecpkey, (const unsigned char **)&pkey, pkl)) {
		//printf("o2i_ECPublicKey fail!\n");
		return -2;
	}
	int res = ECDSA_verify(0, hasz, 32, sign, sil, ecpkey);
	EC_KEY_free(ecpkey);
	return res;
}
*/
import "C"
import "unsafe"


// EC_Verify verifies an ECDSA signature.
func EC_Verify(pkey, sign, hash []byte) int {
	return int(C.verify(unsafe.Pointer(&pkey[0]), C.uint(len(pkey)),
		unsafe.Pointer(&sign[0]), C.uint(len(sign)), unsafe.Pointer(&hash[0])))
}
