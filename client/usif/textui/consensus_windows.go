// On Windows OS copy this file to gocoin\client\usif\textui to enable consensus checking
// Make sure you have proper "libbitcoinconsensus-0.dll" in a folder where OS can find it.

package textui

import (
	"fmt"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/script"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	DllName = "libbitcoinconsensus-0.dll"
	ProcName = "bitcoinconsensus_verify_script"
)


var (
	bitcoinconsensus_verify_script *syscall.Proc

	ConsensusChecks uint64
	ConsensusExpErr uint64
	ConsensusErrors uint64
)


func check_consensus(pkScr []byte, i int, tx *btc.Tx, ver_flags uint32, result bool) {
	var pkscr_ptr uintptr
	if pkScr != nil {
		pkscr_ptr = uintptr(unsafe.Pointer(&pkScr[0]))
	}
	txTo := tx.Serialize()

	go func(pkscr_ptr, pklen, txto, txto_len, i, ver_flags uintptr) {
		r1, _, _ := syscall.Syscall9(bitcoinconsensus_verify_script.Addr(), 7,
			pkscr_ptr, pklen, txto, txto_len, i, ver_flags, 0, 0, 0)

		res := r1 == 1
		atomic.AddUint64(&ConsensusChecks, 1)
		if !result {
			atomic.AddUint64(&ConsensusExpErr, 1)
		}
		if res != result {
			atomic.AddUint64(&ConsensusErrors, 1)
			println("Compare to consensus failed!", res, result)
		}
	}(pkscr_ptr, uintptr(len(pkScr)), uintptr(unsafe.Pointer(&txTo[0])), uintptr(len(txTo)),
		uintptr(i), uintptr(ver_flags))
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
		println("WARNING: Consensus verificatrion disabled.")
		return
	}
	bitcoinconsensus_verify_script, er = dll.FindProc(ProcName)
	if er!=nil {
		println(er.Error())
		println("WARNING: Consensus verificatrion disabled.")
		return
	}
	fmt.Println("Using", DllName, "to verify consensus rules.")
	script.VerifyConsensus = check_consensus
	newUi("cons", false, consensus_stats, "See statistics of the consensus checks")
}
