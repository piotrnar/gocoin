package script

/*
#cgo LDFLAGS:

#include <stdio.h>
#include <dlfcn.h>

unsigned int (*_bitcoinconsensus_version)();

int (*_bitcoinconsensus_verify_script_with_amount)(const unsigned char *scriptPubKey, unsigned int scriptPubKeyLen, int64_t amount,
                                    const unsigned char *txTo        , unsigned int txToLen,
                                    unsigned int nIn, unsigned int flags, int* err);

int (*_bitcoinconsensus_verify_script_with_spent_outputs)(const unsigned char *scriptPubKey, unsigned int scriptPubKeyLen, int64_t amount,
                                    const unsigned char *txTo        , unsigned int txToLen,
                                    const unsigned char *spentOutputs, unsigned int spentOutputsLen,
                                    unsigned int nIn, unsigned int flags, int* err);


int init_bitcoinconsensus_dylib() {
	void *so = dlopen("libbitcoinconsensus.dylib", RTLD_LAZY);
	if (so) {
		*(void **)(&_bitcoinconsensus_version) = dlsym(so, "bitcoinconsensus_version");
		*(void **)(&_bitcoinconsensus_verify_script_with_amount) = dlsym(so, "bitcoinconsensus_verify_script_with_amount");
		*(void **)(&_bitcoinconsensus_verify_script_with_spent_outputs) = dlsym(so, "bitcoinconsensus_verify_script_with_spent_outputs");
		if (!_bitcoinconsensus_version) {
			printf("libbitcoinconsensus.dylib not found\n");
			return 0;
		}
		if (!_bitcoinconsensus_verify_script_with_amount) {
			printf("libbitcoinconsensus.dylib is too old. Use one of bitcoin-core 0.13.1\n");
			return 0;
		}
		if (!_bitcoinconsensus_verify_script_with_spent_outputs) {
			printf("libbitcoinconsensus.dylib is too old. Use one of bitcoin-core 0.22.0\n");
			return 0;
		}
		return 1;
	}
	return 0;
}

int bitcoinconsensus_verify_script_with_amount(const unsigned char *scriptPubKey, unsigned int scriptPubKeyLen, int64_t amount,
                                    const unsigned char *txTo        , unsigned int txToLen,
                                    unsigned int nIn, unsigned int flags, int *err) {
	return _bitcoinconsensus_verify_script_with_amount(scriptPubKey, scriptPubKeyLen, amount, txTo, txToLen, nIn, flags, err);
}


int bitcoinconsensus_verify_script_with_spent_outputs(const unsigned char *scriptPubKey, unsigned int scriptPubKeyLen, int64_t amount,
                                    const unsigned char *txTo        , unsigned int txToLen,
                                    const unsigned char *spentOutputs, unsigned int spentOutputsLen,
                                    unsigned int nIn, unsigned int flags, int* err) {
	return _bitcoinconsensus_verify_script_with_spent_outputs(scriptPubKey, scriptPubKeyLen, amount,
                                    txTo, txToLen, spentOutputs, spentOutputsLen, nIn, flags, err);
}


unsigned int bitcoinconsensus_version() {
	return _bitcoinconsensus_version();
}
*/
import "C"

import (
	//"os"
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/piotrnar/gocoin/lib/btc"
	"unsafe"
)

func verify_script_with_amount(pkScr []byte, amount uint64, i int, tx *btc.Tx, ver_flags uint32) (result bool) {
	var bcerr int
	txTo := tx.Raw
	if txTo == nil {
		txTo = tx.SerializeNew()
	}
	var pkscr_ptr *C.uchar // default to null
	var pkscr_len C.uint   // default to 0
	if pkScr != nil {
		pkscr_ptr = (*C.uchar)(unsafe.Pointer(&pkScr[0]))
		pkscr_len = C.uint(len(pkScr))
	}
	r1 := int(C.bitcoinconsensus_verify_script_with_amount(pkscr_ptr, pkscr_len, C.int64_t(amount),
		(*C.uchar)(unsafe.Pointer(&txTo[0])), C.uint(len(txTo)), C.uint(i), C.uint(ver_flags),
		(*C.int)(unsafe.Pointer(&bcerr))))

	//println("res:", bcerr, r1)
	result = bcerr == 0 && r1 == 1
	return
}

func verify_script_with_spent_outputs(pkScr []byte, amount uint64, outs []byte, i int, tx *btc.Tx, ver_flags uint32) (result bool) {
	var bcerr int
	txTo := tx.Raw
	if txTo == nil {
		txTo = tx.SerializeNew()
	}
	var pkscr_ptr *C.uchar // default to null
	var pkscr_len C.uint   // default to 0
	if pkScr != nil {
		pkscr_ptr = (*C.uchar)(unsafe.Pointer(&pkScr[0]))
		pkscr_len = C.uint(len(pkScr))
	}
	var outs_ptr *C.uchar // default to null
	var outs_len C.uint   // default to 0
	if outs != nil {
		outs_ptr = (*C.uchar)(unsafe.Pointer(&outs[0]))
		outs_len = C.uint(len(outs))
	}
	r1 := int(C.bitcoinconsensus_verify_script_with_spent_outputs(pkscr_ptr, pkscr_len, C.int64_t(amount),
		(*C.uchar)(unsafe.Pointer(&txTo[0])), C.uint(len(txTo)), outs_ptr, outs_len,
		C.uint(i), C.uint(ver_flags), (*C.int)(unsafe.Pointer(&bcerr))))

	//println("reS:", bcerr, r1)
	result = bcerr == 0 && r1 == 1
	return
}

func txout_serialize(to *btc.TxOut) []byte {
	b := new(bytes.Buffer)
	binary.Write(b, binary.LittleEndian, to.Value)
	btc.WriteVlen(b, uint64(len(to.Pk_script)))
	b.Write(to.Pk_script)
	return b.Bytes()
}

func myVerifyTxScript(pkScr []byte, checker *SigChecker, ver_flags uint32) (result bool) {
	tx := checker.Tx
	i := checker.Idx
	
	if (ver_flags & VER_TAPROOT) != 0 {
		_b := new(bytes.Buffer)
		btc.WriteVlen(_b, uint64(len(tx.Spent_outputs)))
		outs := _b.Bytes()
		for _, to := range tx.Spent_outputs {
			outs = append(outs, txout_serialize(to)...)
		}

		return verify_script_with_spent_outputs(pkScr, checker.Amount, outs, i, tx, ver_flags)
	}
	
	return verify_script_with_amount(pkScr, checker.Amount, i, tx, ver_flags)
}

func init() {
	if C.init_bitcoinconsensus_dylib() != 0 {
		fmt.Println("using libbitcoinconsensus version:", C.bitcoinconsensus_version())
		HookVerifyTxScript = myVerifyTxScript
	} else {
		panic("libbitcoinconsensus.dylib not found")
	}
}
