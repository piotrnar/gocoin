package network

import (
	"fmt"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
)

// MempoolCheck verifies the Mempool for consistency.
// Make sure to call it with TxMutex Locked.
func MempoolCheck() (dupa bool) {
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

			outk, ok := SpentOutputs[inp.Input.UIdx()]
			if ok {
				if outk != t2s.Hash.BIdx() {
					fmt.Println("Tx", t2s.Hash.String(), "input", i, "has a mismatch in SpentOutputs record", outk)
					dupa = true
				}
			} else {
				fmt.Println("Tx", t2s.Hash.String(), "input", i, "is not in SpentOutputs")
				dupa = true
			}

			_, ok = TransactionsToSend[btc.BIdx(inp.Input.Hash[:])]

			if t2s.MemInputs == nil {
				if ok {
					fmt.Println("Tx", t2s.Hash.String(), "MemInputs==nil but input", i, "is in mempool", inp.Input.String())
					dupa = true
				}
			} else {
				if t2s.MemInputs[i] {
					micnt++
					if !ok {
						fmt.Println("Tx", t2s.Hash.String(), "MemInput set but input", i, "NOT in mempool", inp.Input.String())
						dupa = true
					}
				} else {
					if ok {
						fmt.Println("Tx", t2s.Hash.String(), "MemInput NOT set but input", i, "IS in mempool", inp.Input.String())
						dupa = true
					}
				}
			}

			if _, ok := TransactionsToSend[btc.BIdx(inp.Input.Hash[:])]; !ok {
				if unsp := common.BlockChain.Unspent.UnspentGet(&inp.Input); unsp == nil {
					fmt.Println("Mempool tx", t2s.Hash.String(), "has no input", i)
					dupa = true
				}
			}
		}
		if t2s.MemInputs != nil && micnt == 0 {
			fmt.Println("Tx", t2s.Hash.String(), "has MemInputs array with all false values")
			dupa = true
		}
		if t2s.MemInputCnt != micnt {
			fmt.Println("Tx", t2s.Hash.String(), "has incorrect MemInputCnt", t2s.MemInputCnt, micnt)
			dupa = true
		}
	}

	if spent_cnt != len(SpentOutputs) {
		fmt.Println("SpentOutputs length mismatch", spent_cnt, len(SpentOutputs))
		dupa = true
	}

	if totsize != TransactionsToSendSize {
		fmt.Println("TransactionsToSendSize mismatch", totsize, TransactionsToSendSize)
		dupa = true
	}

	totsize = 0
	for _, tr := range TransactionsRejected {
		if tr.Tx != nil {
			if tr.Tx.Raw == nil {
				fmt.Println("RejectedTx", tr.Id.String(), "has Tx but no Raw")
				dupa = true
			} else {
				totsize += uint64(len(tr.Raw))
			}
			if tr.Reason < 200 {
				fmt.Println("RejectedTx", tr.Id.String(), "reason", tr.Reason, "but no data")
			}
		} else {
			if tr.Reason >= 200 {
				fmt.Println("RejectedTx", tr.Id.String(), "has sata, reason", tr.Reason)
			}
		}

		if tr.Waiting4 != nil {
			if tr.Tx == nil || tr.Tx.Raw == nil {
				fmt.Println("RejectedTx", tr.Id.String(), "has w4i but no tx data")
				dupa = true
			} else {
				w4i_cnt++
				w4isize += uint64(len(tr.Raw))
			}
		}
	}
	if totsize != TransactionsRejectedSize {
		fmt.Println("TransactionsRejectedSize mismatch", totsize, TransactionsRejectedSize)
		dupa = true
	}

	spent_cnt = 0
	for _, rec := range WaitingForInputs {
		spent_cnt += len(rec.Ids)
	}
	if w4i_cnt != spent_cnt {
		fmt.Println("WaitingForInputs count mismatch", w4i_cnt, spent_cnt)
		dupa = true
	}
	if w4isize != WaitingForInputsSize {
		fmt.Println("WaitingForInputsSize mismatch", w4isize, totsize)
		dupa = true
	}

	return
}
