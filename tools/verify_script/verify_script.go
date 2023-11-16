package main

import (
	"fmt"
	"unsafe"
	"syscall"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/script"
)

const (
	DllName = "libbitcoinconsensus-0.dll"
	ProcName = "bitcoinconsensus_verify_script_with_amount"
)


var (
	bitcoinconsensus_verify_script_with_amount *syscall.Proc
)


func call_consensus_lib(pkScr []byte, amount uint64, i int, tx *btc.Tx, ver_flags uint32) bool {
	var tmp []byte
	if len(pkScr)!=0 {
		tmp = make([]byte, len(pkScr))
		copy(tmp, pkScr)
	}
	txTo := tx.Serialize()
	var pkscr_ptr, pkscr_len uintptr // default to 0/null
	if pkScr != nil {
		pkscr_ptr = uintptr(unsafe.Pointer(&pkScr[0]))
		pkscr_len = uintptr(len(pkScr))
	}
	r1, _, _ := syscall.Syscall9(bitcoinconsensus_verify_script_with_amount.Addr(), 8,
		pkscr_ptr, pkscr_len, uintptr(amount),
		uintptr(unsafe.Pointer(&txTo[0])), uintptr(len(txTo)),
		uintptr(i), uintptr(ver_flags), 0, 0)

	return r1 == 1
}


func init() {
	dll, er := syscall.LoadDLL(DllName)
	if er!=nil {
		println(er.Error())
		println("WARNING: Consensus verificatrion disabled")
		return
	}
	bitcoinconsensus_verify_script_with_amount, er = dll.FindProc(ProcName)
	if er!=nil {
		println(er.Error())
		println("WARNING: Consensus verificatrion disabled")
		return
	}
	fmt.Println("Using", DllName, "to ensure consensus rules")
}
/*
*/

func main() {
	pkscript, _ := hex.DecodeString("a9143d98738ba9013a53acc34686cd8e8b2ebc3612e587")
	d, _ := hex.DecodeString("01000000010c3e18ff26e98ba39381c84d2fb9e8e198e63d0b3697f9bd57f63577c96da23f00000000d5483045022100fc4f7bfa3c536e743b02af8b7de5d4052f43db54f59692478b25c585b9df211a02203cfcfc0ed618fae6aa49b11e803ec4e5654551fb52fe2d026929f4a307fe2ef0012103d7c6052544bc42eb2bc0d27c884016adb933f15576a1a2d21cd4dd0f2de0c37d004c67635221025e37e03703f001de34123b513beaf0e4044a2dd39a1dd92ec1706f184920031a2103d7c6052544bc42eb2bc0d27c884016adb933f15576a1a2d21cd4dd0f2de0c37d52ae67010ab27576a914937fe2ee82229d282edec2606c70e755875334c088ac680f0000000130750000000000001976a914937fe2ee82229d282edec2606c70e755875334c088ac0f000000")
	tx, _ := btc.NewTx(d)
	tx.Hash.Calc(d)
	println("txid", tx.Hash.String())
	i := 0
	flags := uint32(script.STANDARD_VERIFY_FLAGS) //& ^uint32(script.VER_MINDATA)
	amount := uint64(1000000)
	//script.DBG_SCR = true
	//script.DBG_ERR = true
	res := script.VerifyTxScript(pkscript, &script.SigChecker{Amount:amount, Idx:i, Tx:tx}, flags)
	if bitcoinconsensus_verify_script_with_amount!=nil {
		resc := call_consensus_lib(pkscript, amount, i, tx, flags)
		println(res, resc)
	} else {
		println(res)
	}
}
