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
	"sync/atomic"
	"syscall"
	"unsafe"
	"sync"
)

const (
	DllName = "libbitcoinconsensus-0.dll"
	ProcName = "bitcoinconsensus_verify_script_with_amount"
)


/*
EXPORT_SYMBOL int bitcoinconsensus_verify_script(const unsigned char *scriptPubKey, unsigned int scriptPubKeyLen,
                                                 const unsigned char *txTo        , unsigned int txToLen,
                                                 unsigned int nIn, unsigned int flags, bitcoinconsensus_error* err);

EXPORT_SYMBOL int bitcoinconsensus_verify_script_with_amount(const unsigned char *scriptPubKey, unsigned int scriptPubKeyLen, int64_t amount,
                                    const unsigned char *txTo        , unsigned int txToLen,
                                    unsigned int nIn, unsigned int flags, bitcoinconsensus_error* err);

*/


var (
	bitcoinconsensus_verify_script_with_amount *syscall.Proc

	ConsensusChecks uint64
	ConsensusExpErr uint64
	ConsensusErrors uint64

	mut sync.Mutex
)


func check_consensus(pkScr []byte, amount uint64, i int, tx *btc.Tx, ver_flags uint32, result bool) {
	var tmp []byte
	if len(pkScr)!=0 {
		tmp = make([]byte, len(pkScr))
		copy(tmp, pkScr)
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
			println("Compare to consensus failed!", res, result)
			println("Gocoin", result)
			println("ConsLIB", res)
			println("pkScr", hex.EncodeToString(pkScr))
			println("txTo", hex.EncodeToString(txTo))
			println("i", i)
			println("ver_flags", ver_flags)
			mut.Unlock()
		}
	}(tmp, tx.Serialize(), i, ver_flags, result)
}

func consensus_stats(s string) {
	fmt.Println("Consensus Checks:", atomic.LoadUint64(&ConsensusChecks))
	fmt.Println("Consensus ExpErr:", atomic.LoadUint64(&ConsensusExpErr))
	fmt.Println("Consensus Errors:", atomic.LoadUint64(&ConsensusErrors))
}

func init() {
	dll, er := syscall.LoadDLL(DllName)
	if er!=nil {
		println(er.Error())
		println("WARNING: Consensus verification disabled")
		return
	}
	bitcoinconsensus_verify_script_with_amount, er = dll.FindProc(ProcName)
	if er!=nil {
		println(er.Error())
		println("DllName is probably too old. Use one of bitcoin-core 0.13.1\n");
		println("WARNING: Consensus verification disabled")
		return
	}
	fmt.Println("Using", DllName, "to ensure consensus rules")
	script.VerifyConsensus = check_consensus
	newUi("cons", false, consensus_stats, "See statistics of the consensus checks")
}
