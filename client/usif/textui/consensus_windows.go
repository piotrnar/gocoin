// +build windows

// On Windows OS copy this file to gocoin\client\usif\textui to enable consensus checking
// Make sure you have proper "libbitcoinconsensus-0.dll" in a folder where OS can find it.

package textui

import (
	"encoding/hex"
	"fmt"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/script"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	DllName = "libbitcoinconsensus-0.dll"
)

/*
EXPORT_SYMBOL int bitcoinconsensus_verify_script(const unsigned char *scriptPubKey, unsigned int scriptPubKeyLen,
                                                 const unsigned char *txTo        , unsigned int txToLen,
                                                 unsigned int nIn, unsigned int flags, bitcoinconsensus_error* err);

EXPORT_SYMBOL int bitcoinconsensus_verify_script_with_amount(const unsigned char *scriptPubKey, unsigned int scriptPubKeyLen, int64_t amount,
                                    const unsigned char *txTo        , unsigned int txToLen,
                                    unsigned int nIn, unsigned int flags, bitcoinconsensus_error* err);

EXPORT_SYMBOL unsigned int bitcoinconsensus_version();
}
*/

var (
	bitcoinconsensus_verify_script_with_amount *syscall.Proc
	bitcoinconsensus_version                   *syscall.Proc

	ConsensusChecks uint64
	ConsensusExpErr uint64
	ConsensusErrors uint64

	mut sync.Mutex
)

func check_consensus(pkScr []byte, amount uint64, i int, tx *btc.Tx, ver_flags uint32, result bool) {
	var tmp []byte
	if len(pkScr) != 0 {
		tmp = make([]byte, len(pkScr))
		copy(tmp, pkScr)
	}
	tx_raw := tx.Raw
	if tx_raw == nil {
		tx_raw = tx.Serialize()
	}
	go func(pkScr []byte, txTo []byte, i int, ver_flags uint32, result bool) {
		var pkscr_ptr, pkscr_len uintptr // default to 0/null
		if pkScr != nil {
			pkscr_ptr = uintptr(unsafe.Pointer(&pkScr[0]))
			pkscr_len = uintptr(len(pkScr))
		}
		r1, _, _ := syscall.Syscall9(bitcoinconsensus_verify_script_with_amount.Addr(), 8,
			pkscr_ptr, pkscr_len, uintptr(amount),
			uintptr(unsafe.Pointer(&txTo[0])), uintptr(len(txTo)),
			uintptr(i), uintptr(ver_flags), 0, 0)

		res := r1 == 1
		atomic.AddUint64(&ConsensusChecks, 1)
		if !result {
			atomic.AddUint64(&ConsensusExpErr, 1)
		}
		if res != result {
			atomic.AddUint64(&ConsensusErrors, 1)
			common.CountSafe("TxConsensusERR")
			mut.Lock()
			println("Compare to consensus failed!")
			println("Gocoin:", result, "   ConsLIB:", res)
			println("pkScr", hex.EncodeToString(pkScr))
			println("txTo", hex.EncodeToString(txTo))
			println("amount:", amount, "  input_idx:", i, "  ver_flags:", ver_flags)
			println()
			mut.Unlock()
		}
	}(tmp, tx_raw, i, ver_flags, result)
}

func verify_script_with_amount(pkScr []byte, amount uint64, i int, tx *btc.Tx, ver_flags uint32) (result bool) {
	var pkscr_ptr, pkscr_len uintptr // default to 0/null
	txTo := tx.Raw
	if txTo == nil {
		txTo = tx.Serialize()
	}
	if pkScr != nil {
		pkscr_ptr = uintptr(unsafe.Pointer(&pkScr[0]))
		pkscr_len = uintptr(len(pkScr))
	}
	r1, _, _ := syscall.Syscall9(bitcoinconsensus_verify_script_with_amount.Addr(), 8,
		pkscr_ptr, pkscr_len, uintptr(amount),
		uintptr(unsafe.Pointer(&txTo[0])), uintptr(len(txTo)),
		uintptr(i), uintptr(ver_flags), 0, 0)

	result = (r1 == 1)
	return
}

func consensus_stats(s string) {
	fmt.Println("Consensus Checks:", atomic.LoadUint64(&ConsensusChecks))
	fmt.Println("Consensus ExpErr:", atomic.LoadUint64(&ConsensusExpErr))
	fmt.Println("Consensus Errors:", atomic.LoadUint64(&ConsensusErrors))
}

func init() {
	dll, er := syscall.LoadDLL(DllName)
	if er != nil {
		//common.Log.Println(er.Error())
		common.Log.Println("WARNING: Not using", DllName, "to cross-check consensus rules")
		return
	}
	bitcoinconsensus_verify_script_with_amount, er = dll.FindProc("bitcoinconsensus_verify_script_with_amount")
	if er == nil {
		bitcoinconsensus_version, er = dll.FindProc("bitcoinconsensus_version")
	}
	if er != nil {
		common.Log.Println(er.Error())
		common.Log.Println(DllName, "is probably too old. Use one of bitcoin-core 0.13.1 or later")
		common.Log.Println("WARNING: Consensus cross-checking disabled")
		return
	}
	r1, _, _ := syscall.Syscall(bitcoinconsensus_version.Addr(), 0, 0, 0, 0)
	common.Log.Println("Using", DllName, "version", r1, "to cross-check consensus rules")
	script.VerifyConsensus = check_consensus
	newUi("cons", false, consensus_stats, "See statistics of the consensus cross-checks")
}
