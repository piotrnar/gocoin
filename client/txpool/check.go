package txpool

import (
	"encoding/hex"
	"fmt"
	"slices"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
)

func (t *OneTxToSend) isInMap() (yes bool) {
	var tt *OneTxToSend
	tt, yes = TransactionsToSend[t.Hash.BIdx()]
	if yes && tt != t {
		println("ERROR: t2x in the map does not point back to itself", t.Hash.String(), "\n  ", tt.Hash.String())
		yes = false
	}
	return
}

func checkFeeList() bool {
	if FeePackagesDirty {
		common.CountSafe("TxPkgsCheckDirty")
		return false
	}

	for _, pkg := range FeePackages {
		if len(pkg.Txs) < 2 {
			println("ERROR: package has only", len(pkg.Txs), "txs")
			return true
		}
		for idx, t := range pkg.Txs {
			if !t.isInMap() {
				println("ERROR: tx in pkg", pkg, "does not point to a valid t2s", idx, len(pkg.Txs))
				println("    ...", t.Hash.String())
				return true
			}
			if !slices.Contains(t.InPackages, pkg) {
				println("ERROR: tx in pkg", pkg, "does not point back to the package", idx, len(pkg.Txs),
					"\n  ...", t.Hash.String())
				for _, p := range t.InPackages {
					println(" -", p)
				}
				return true
			}
		}
	}

	for _, t2s := range TransactionsToSend {
		if t2s.InPackages != nil {
			for _, pkg := range t2s.InPackages {
				if !slices.Contains(pkg.Txs, t2s) {
					println("ERROR: pkg does not have the tx which ir should")
					return true
				}
			}
		}
	}

	common.CountSafe("TxPkgsCheckOK")
	return false
}

func checkPoolSizes() (dupa int) {
	var t2s_size, txr_size, w4i_size int
	for _, t2x := range TransactionsToSend {
		ts := t2x.SysSize()
		if ts != int(t2x.Footprint) {
			dupa++
			fmt.Println(dupa, "ERROR: T2S", t2x.Hash.String(), "mismatch footprint:", t2x.Footprint, ts)
		}
		t2s_size += ts
		if t2x.TxVerVars != nil {
			dupa++
			fmt.Println(dupa, "ERROR: T2S", t2x.Hash.String(), "does not seem to be clean")
		}
	}
	for _, txr := range TransactionsRejected {
		ts := txr.SysSize()
		if ts != int(txr.Footprint) {
			dupa++
			fmt.Println(dupa, "ERROR: TxR", txr.Hash.String(), "mismatch footprint:", txr.Footprint, ts)
		}
		txr_size += ts
		if txr.Waiting4 != nil {
			w4i_size += ts
		}
	}

	if TransactionsToSendSize != uint64(t2s_size) {
		dupa++
		fmt.Println("ERROR: TransactionsToSendSize mismatch:", TransactionsToSendSize, "  real:", t2s_size)
	}

	if TransactionsRejectedSize != uint64(txr_size) {
		dupa++
		fmt.Println("ERROR: TransactionsRejectedSize mismatch:", TransactionsRejectedSize, "  real:", txr_size)
	}

	if WaitingForInputsSize != uint64(w4i_size) {
		dupa++
		fmt.Println("ERROR: WaitingForInputsSize mismatch:", WaitingForInputsSize, "  real:", w4i_size)
	}

	return
}

func CheckPoolSizes() (dupa int) {
	TxMutex.Lock()
	dupa = checkPoolSizes()
	TxMutex.Unlock()
	return
}

func check_the_index(dupa int) int {
	seen := make(map[btc.BIDX]int)
	for idx := TRIdxTail; ; idx = TRIdxNext(idx) {
		bidx := TRIdxArray[idx]
		if txr := TransactionsRejected[bidx]; txr != nil {
			if idx, ok := seen[bidx]; ok {
				dupa++
				fmt.Println(dupa, "TxR", txr.Id.String(), ReasonToString(txr.Reason), "from idx", idx,
					"present again in TRIdxArray at", idx, TRIdxHead, TRIdxTail)
			} else {
				seen[bidx] = idx
			}
		} else {
			if !TRIdIsZeroArrayRec(idx) {
				dupa++
				fmt.Println(dupa, "TRIdxArray index", idx, "is not zero", hex.EncodeToString(bidx[:]),
					"but has not txr in the map", TRIdxHead, TRIdxTail)
				break
			}
		}
		if idx == TRIdxHead {
			break
		}
	}
	return dupa
}

// MempoolCheck verifies the Mempool for consistency.
// Usefull for debuggning as normally there should be no consistencies.
// Make sure to call it with TxMutex Locked.
func MempoolCheck() bool {
	var dupa int
	var spent_cnt int
	var w4i_cnt int
	var totsize uint64
	var w4isize uint64

	// First check if t2s.MemInputs fields are properly set
	for _, t2s := range TransactionsToSend {
		var micnt int

		totsize += uint64(t2s.Footprint)

		if t2s.Weight() == 0 {
			dupa++
			fmt.Println(dupa, "Tx", t2s.Hash.String(), "haz seight 0")
		}

		for i, inp := range t2s.TxIn {
			spent_cnt++
			if outk, ok := SpentOutputs[inp.Input.UIdx()]; ok {
				if outk != t2s.Hash.BIdx() {
					dupa++
					fmt.Println(dupa, "Tx", t2s.Hash.String(), "input", i, "has a mismatch in SpentOutputs record", outk)
				}
			} else {
				dupa++
				fmt.Println(dupa, "Tx", t2s.Hash.String(), "input", i, "is not in SpentOutputs")
			}

			_, ok := TransactionsToSend[btc.BIdx(inp.Input.Hash[:])]

			if t2s.MemInputs == nil {
				if ok {
					dupa++
					fmt.Println(dupa, "Tx", t2s.Hash.String(), "MemInputs==nil but input", i, "is in mempool", inp.Input.String())
				}
			} else {
				if t2s.MemInputs[i] {
					micnt++
					if !ok {
						dupa++
						fmt.Println(dupa, "Tx", t2s.Hash.String(), "MemInput set but input", i, "NOT in mempool", inp.Input.String())
					}
				} else {
					if ok {
						dupa++
						fmt.Println(dupa, "Tx", t2s.Hash.String(), "MemInput NOT set but input", i, "IS in mempool", inp.Input.String())
					}
				}
			}
			if _, ok := TransactionsToSend[btc.BIdx(inp.Input.Hash[:])]; !ok {
				if unsp := common.BlockChain.Unspent.UnspentGet(&inp.Input); unsp == nil {
					dupa++
					fmt.Println(dupa, "Mempool tx", t2s.Hash.String(), "has no input", i)
				}
			}
		}
		if t2s.MemInputs != nil && micnt == 0 {
			dupa++
			fmt.Println(dupa, "Tx", t2s.Hash.String(), "has MemInputs array with all false values")
		}
		if t2s.MemInputCnt != micnt {
			dupa++
			fmt.Println(dupa, "Tx", t2s.Hash.String(), "has incorrect MemInputCnt", t2s.MemInputCnt, micnt)
		}
	}

	for _, so := range SpentOutputs {
		if _, ok := TransactionsToSend[so]; !ok {
			dupa++
			fmt.Println(dupa, "SpentOutput", btc.BIdxString(so), "does not have tx in mempool")
		}
	}
	if spent_cnt != len(SpentOutputs) {
		dupa++
		fmt.Println(dupa, "SpentOutputs length mismatch", spent_cnt, len(SpentOutputs))
	}

	if totsize != TransactionsToSendSize {
		dupa++
		fmt.Println(dupa, "TransactionsToSendSize mismatch", totsize, TransactionsToSendSize)
	}

	totsize = 0
	tot_utxo_used := 0
	for _, tr := range TransactionsRejected {
		if tr.Tx != nil {
			if tr.Tx.Raw == nil {
				dupa++
				fmt.Println(dupa, "TxR", tr.Id.String(), "has Tx but no Raw")
			} else {
				totsize += uint64(tr.Footprint)
				tot_utxo_used += len(tr.Tx.TxIn)
			}
			if tr.Reason < 200 {
				dupa++
				fmt.Println(dupa, "TxR", tr.Id.String(), "reason", ReasonToString(tr.Reason), "but no data")
			}
		} else {
			if tr.Reason >= 200 {
				dupa++
				fmt.Println(dupa, "TxR", tr.Id.String(), "has data, reason", ReasonToString(tr.Reason))
			}
		}

		if tr.Waiting4 != nil {
			if tr.Reason != TX_REJECTED_NO_TXOU {
				dupa++
				fmt.Println(dupa, "TxR", tr.Id.String(), "has w4i and reason", ReasonToString(tr.Reason))
			}
			if tr.Tx == nil || tr.Tx.Raw == nil {
				dupa++
				fmt.Println(dupa, "TxR", tr.Id.String(), tr.Reason, "has w4i but no tx data")
			} else {
				w4i_cnt++
				w4isize += uint64(tr.Footprint)
			}
		} else {
			if tr.Reason == TX_REJECTED_NO_TXOU {
				dupa++
				fmt.Println(dupa, "TxR", tr.Id.String(), "has not w4i but reason", ReasonToString(tr.Reason))
			}
		}
	}
	if totsize != TransactionsRejectedSize {
		dupa++
		fmt.Println(dupa, "TransactionsRejectedSize mismatch", totsize, TransactionsRejectedSize)
	}

	spent_cnt = 0
	for _, rec := range WaitingForInputs {
		if len(rec.Ids) == 0 {
			dupa++
			fmt.Println(dupa, "WaitingForInputs", rec.TxID.String(), "has zero records")
		}
		spent_cnt += len(rec.Ids)
	}
	if w4i_cnt != spent_cnt {
		dupa++
		fmt.Println(dupa, "WaitingForInputs count mismatch", w4i_cnt, spent_cnt)
	}
	if w4isize != WaitingForInputsSize {
		dupa++
		fmt.Println(dupa, "WaitingForInputsSize mismatch", w4isize, WaitingForInputsSize)
	}

	dupa += check_the_index(dupa)
	dupa += checkPoolSizes()

	spent_cnt = 0
	for utxoidx, lst := range RejectedUsedUTXOs {
		spent_cnt += len(lst)
		for _, bidx := range lst {
			if txr, ok := TransactionsRejected[bidx]; ok && txr.Tx != nil {
				var found bool
				for _, inp := range txr.TxIn {
					if _, ok := RejectedUsedUTXOs[inp.Input.UIdx()]; ok {
						found = true
						break
					}
					if !found {
						dupa++
						fmt.Println(dupa, "Tx", txr.Id.String(), "in RejectedUsedUTXOs but without back reference to RejectedUsedUTXOs")
					}
				}

			} else {
				dupa++
				fmt.Println(dupa, "btc.BIDX", btc.BIdxString(bidx), "present in RejectedUsedUTXOs",
					fmt.Sprintf("%016x", utxoidx), "but not in TransactionsRejected")
				if t2s, ok := TransactionsToSend[bidx]; ok {
					fmt.Println("   It is however in T2S", t2s.Hash.String())
				} else {
					fmt.Println("   Not it is in T2S")
				}
			}
		}
	}
	if spent_cnt != tot_utxo_used {
		dupa++
		fmt.Println(dupa, "RejectedUsedUTXOs count mismatch", spent_cnt, tot_utxo_used)

		fmt.Println("Checking which txids are missing...")
		for bidx, txr := range TransactionsRejected {
			if txr.Tx == nil {
				continue
			}
			for _, inp := range txr.TxIn {
				uidx := inp.Input.UIdx()
				if lst, ok := RejectedUsedUTXOs[uidx]; ok {
					var found bool
					for _, bi := range lst {
						if bidx == bi {
							found = true
							break
						}
					}
					if !found {
						fmt.Println(" - Missing on list", inp.Input.String(), "\n  for", txr.Id.String())
					}
				} else {
					fmt.Println(" - Missing record", inp.Input.String(), "\n  for", txr.Id.String())
				}
				RejectedUsedUTXOs[uidx] = append(RejectedUsedUTXOs[uidx], txr.Id.BIdx())
			}
		}
	}

	if checkFeeList() {
		dupa++
		fmt.Println(dupa, "checkFeeList failed")
	}

	return dupa > 0
}

/* --== Let's keep it here for now as it sometimes comes handy for debuging

var first_ = true

// call this one when TxMutex is locked
func MPC_locked() bool {
	if first_ && MempoolCheck() {
		first_ = false
		_, file, line, _ := runtime.Caller(1)
		println("=====================================================")
		println("Mempool first iime seen broken from", file, line)
		return true
	}
	return false
}

func MPC() (res bool) {
	TxMutex.Lock()
	res = MPC_locked()
	TxMutex.Unlock()
	return
}
*/
