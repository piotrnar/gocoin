package network

import (
	"fmt"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
)

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

		totsize += uint64(len(t2s.Raw))

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
				totsize += uint64(len(tr.Raw))
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
				w4isize += uint64(len(tr.Raw))
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
				fmt.Println(dupa, "BIDX", btc.BIdxString(bidx), "present in RejectedUsedUTXOs",
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
