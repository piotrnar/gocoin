package network

import (
	"fmt"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"runtime"
	"sort"
	"time"
)

// returns true if the given tx has not memory inputs or if all of the memory inputs are in already_in
func missing_parents(txkey BIDX, already_in map[BIDX]bool) (res []BIDX) {
	tx := TransactionsToSend[txkey]
	if tx.MemInputs == nil {
		return
	}
	var cnt_ok int
	for idx, inp := range tx.TxIn {
		if tx.MemInputs[idx] {
			txk := btc.BIdx(inp.Input.Hash[:])
			if _, ok := already_in[txk]; ok {
			} else {
				res = append(res, txk)
			}

			cnt_ok++
			if cnt_ok == tx.MemInputCnt {
				return
			}
		}
	}
	return
}

func GetSortedMempool() (result []BIDX) {
	inBlock := make([]BIDX, len(TransactionsToSend))
	var idx int
	const MIN_PKB = 200
	for k, _ := range TransactionsToSend {
		inBlock[idx] = k
		idx++
	}
	sort.Slice(inBlock, func(i, j int) bool {
		rec_i := TransactionsToSend[inBlock[i]]
		rec_j := TransactionsToSend[inBlock[j]]
		rate_i := rec_i.Fee * uint64(rec_j.Weight())
		rate_j := rec_j.Fee * uint64(rec_i.Weight())
		if rate_i != rate_j {
			return rate_i > rate_j
		}
		if rec_i.MemInputCnt != rec_j.MemInputCnt {
			return rec_i.MemInputCnt < rec_j.MemInputCnt
		}
		for x := 0; x < 32; x++ {
			if rec_i.Hash.Hash[x] != rec_i.Hash.Hash[x] {
				return rec_i.Hash.Hash[x] < rec_i.Hash.Hash[x]
			}
		}
		return false
	})

	// now put the childrer after the parents
	result = make([]BIDX, len(inBlock))
	already_in := make(map[BIDX]bool, len(inBlock))
	parent_of := make(map[BIDX][]BIDX)

	idx = 0

	var missing_parents = func(txkey BIDX, is_any bool) (res []BIDX, yes bool) {
		tx := TransactionsToSend[txkey]
		if tx.MemInputs == nil {
			return
		}
		var cnt_ok int
		for idx, inp := range tx.TxIn {
			if tx.MemInputs[idx] {
				txk := btc.BIdx(inp.Input.Hash[:])
				if _, ok := already_in[txk]; ok {
				} else {
					yes = true
					if is_any {
						return
					}
					res = append(res, txk)
				}

				cnt_ok++
				if cnt_ok == tx.MemInputCnt {
					return
				}
			}
		}
		return
	}

	var append_txs func(txkey BIDX)
	append_txs = func(txkey BIDX) {
		result[idx] = txkey
		idx++
		already_in[txkey] = true

		if toretry, ok := parent_of[txkey]; ok {
			for _, kv := range toretry {
				if _, in := already_in[kv]; in {
					continue
				}
				if _, yes := missing_parents(kv, true); !yes {
					append_txs(kv)
				}
			}
			delete(parent_of, txkey)
		}
	}

	for _, txkey := range inBlock {
		if missing, yes := missing_parents(txkey, false); yes {
			for _, kv := range missing {
				parent_of[kv] = append(parent_of[kv], txkey)
			}
			continue
		}
		append_txs(txkey)
	}

	if idx != len(result) || idx != len(already_in) || len(parent_of) != 0 {
		fmt.Println("Get sorted mempool idx:", idx, " result:", len(result), " alreadyin:", len(already_in), " parents:", len(parent_of))
		fmt.Println("DUPA!!!!!!!!!!")
		result = result[:idx]
	}

	return
}

func GetSortedMempoolPtrs() (sorted SortedTxToSend) {
	srt := GetSortedMempool()
	sorted = make(SortedTxToSend, len(srt))
	for idx, k := range srt {
		sorted[idx] = TransactionsToSend[k]
	}
	return
}

var (
	poolenabled   bool
	expireperbyte float64
	maxexpiretime time.Duration
	lastTxsExpire time.Time
)

// delete specified tx and all of its children from mempool
func DeleteAllToSend(t2s *OneTxToSend) {
	if _, ok := TransactionsToSend[t2s.Hash.BIdx()]; !ok {
		// Transaction already removed
		return
	}

	// remove all the children that are spending from t2s
	var po btc.TxPrevOut
	po.Hash = t2s.Hash.Hash
	for po.Vout = 0; po.Vout < uint32(len(t2s.TxOut)); po.Vout++ {
		if so, ok := SpentOutputs[po.UIdx()]; ok {
			if child, ok := TransactionsToSend[so]; ok {
				DeleteAllToSend(child)
			}
		}
	}

	// Remove the t2s itself
	RejectTx(t2s.Hash, len(t2s.Data), TX_REJECTED_LOW_FEE)
	DeleteToSend(t2s)
}

// This must be called with TxMutex locked
func limitPoolSize(maxlen uint64) {
	ticklen := maxlen >> 5 // 1/32th of the max size = X

	if TransactionsToSendSize < maxlen {
		if TransactionsToSendSize < maxlen-2*ticklen {
			if common.SetMinFeePerKB(0) {
				var cnt uint64
				for k, v := range TransactionsRejected {
					if v.Reason == TX_REJECTED_LOW_FEE {
						deleteRejected(k)
						cnt++
					}
				}
				common.CounterMutex.Lock()
				common.Counter["TxPoolSizeLow"]++
				common.Counter["TxRejectedFeeUndone"] += cnt
				common.CounterMutex.Unlock()
				fmt.Println("Mempool size low:", TransactionsToSendSize, maxlen, maxlen-2*ticklen, "-", cnt, "rejected purged")
			}
		} else {
			common.CountSafe("TxPoolSizeOK")
			//fmt.Println("Mempool size OK:", TransactionsToSendSize, maxlen, maxlen-2*ticklen)
		}
		return
	}

	sta := time.Now()

	sorted := GetSortedMempoolPtrs()
	idx := len(sorted)

	old_size := TransactionsToSendSize

	maxlen -= ticklen

	for idx > 0 && TransactionsToSendSize > maxlen {
		idx--
		DeleteAllToSend(sorted[idx])
	}

	newspkb := uint64(float64(1000*sorted[idx].Fee) / float64(sorted[idx].VSize()))
	common.SetMinFeePerKB(newspkb)

	cnt := len(sorted) - idx

	fmt.Println("Mempool purged in", time.Now().Sub(sta).String(), "-",
		old_size-TransactionsToSendSize, "/", old_size, "bytes and", cnt, "/", len(sorted), "txs removed. SPKB:", newspkb)
	common.CounterMutex.Lock()
	common.Counter["TxPoolSizeHigh"]++
	common.Counter["TxPurgedSizCnt"] += uint64(cnt)
	common.Counter["TxPurgedSizBts"] += old_size - TransactionsToSendSize
	common.CounterMutex.Unlock()
}

var stop_mempool_checks bool

func MPC() {
	if er := MempoolCheck(); er {
		_, file, line, _ := runtime.Caller(1)
		println("MempoolCheck() first failed in", file, line)
		stop_mempool_checks = true
	}
}

func MempoolCheck() (dupa bool) {
	var spent_cnt int

	TxMutex.Lock()

	// First check if t2s.MemInputs fields are properly set
	for _, t2s := range TransactionsToSend {
		var micnt int

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

	TxMutex.Unlock()

	return
}
