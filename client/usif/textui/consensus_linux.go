// +build linux

package textui

/*
#cgo LDFLAGS: -lbitcoinconsensus

typedef enum bitcoinconsensus_error_t
{
    bitcoinconsensus_ERR_OK = 0,
    bitcoinconsensus_ERR_TX_INDEX,
    bitcoinconsensus_ERR_TX_SIZE_MISMATCH,
    bitcoinconsensus_ERR_TX_DESERIALIZE,
} bitcoinconsensus_error;

int bitcoinconsensus_verify_script(const unsigned char *scriptPubKey, unsigned int scriptPubKeyLen,
                                    const unsigned char *txTo        , unsigned int txToLen,
                                    unsigned int nIn, unsigned int flags, bitcoinconsensus_error* err);

unsigned int bitcoinconsensus_version();

*/
import "C"

import (
	"fmt"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/script"
	"sync/atomic"
	"unsafe"
)

var (
	ConsensusChecks uint64
	ConsensusExpErr uint64
	ConsensusErrors uint64
)

func check_consensus(pkScr []byte, i int, tx *btc.Tx, ver_flags uint32, result bool) {
	var pkscr_ptr *C.uchar
	if pkScr != nil {
		pkscr_ptr = (*C.uchar)(unsafe.Pointer(&pkScr[0]))
	}
	txTo := tx.Serialize()
	txo_ptr := (*C.uchar)(unsafe.Pointer(&txTo[0]))

	go func(pkscr_ptr *C.uchar, pklen C.uint, txto *C.uchar, txto_len, i, ver_flags C.uint) {
		r1 := int(C.bitcoinconsensus_verify_script(pkscr_ptr, pklen, txto, txto_len, i, ver_flags,
			(*C.bitcoinconsensus_error)(unsafe.Pointer(nil))))
		res := r1 == 1
		atomic.AddUint64(&ConsensusChecks, 1)
		if !result {
			atomic.AddUint64(&ConsensusExpErr, 1)
		}
		if res != result {
			atomic.AddUint64(&ConsensusErrors, 1)
			println("Compare to consensus failed!", res, result)
		}
	}(pkscr_ptr, C.uint(len(pkScr)), txo_ptr, C.uint(len(txTo)), C.uint(i), C.uint(ver_flags))
}

func consensus_stats(s string) {
	fmt.Println("Consensus Checks:", atomic.LoadUint64(&ConsensusChecks))
	fmt.Println("Consensus ExpErr:", atomic.LoadUint64(&ConsensusExpErr))
	fmt.Println("Consensus Errors:", atomic.LoadUint64(&ConsensusErrors))
}

func init() {
	fmt.Println("Using libbitcoinconsensus.so version", C.bitcoinconsensus_version(), "to ensure consensus rules.")
	script.VerifyConsensus = check_consensus
	newUi("cons", false, consensus_stats, "See statistics of the consensus checks")
}
