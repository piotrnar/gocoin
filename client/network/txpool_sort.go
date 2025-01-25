package network

import (
	"fmt"
	"sort"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
)

var (
	limitTxpoolSizeNow bool = true
	lastTxsPoolLimit   time.Time
	nextTxsPoolExpire  time.Time = time.Now().Add(time.Hour)
)

func VerifyMempoolSort(txs []*OneTxToSend) {
	idxs := make(map[BIDX]int, len(txs))
	for i, t2s := range txs {
		idxs[t2s.Hash.BIdx()] = i
	}
	var oks int
	for i, t2s := range txs {
		for _, txin := range t2s.TxIn {
			if idx, ok := idxs[btc.BIdx(txin.Input.Hash[:])]; ok {
				if idx > i {
					println("mempool sorting error:", i, "points to", idx)
					return
				} else {
					oks++
				}
			}
		}
	}
	println("mempool sorting OK", oks, len(txs))
}

// GetSortedMempool returns txs sorted by SPB, but with parents first.
func GetSortedMempool() (result []*OneTxToSend) {
	all_txs := make([]BIDX, len(TransactionsToSend))
	var idx int
	for k := range TransactionsToSend {
		all_txs[idx] = k
		idx++
	}
	sort.Slice(all_txs, func(i, j int) bool {
		rec_i := TransactionsToSend[all_txs[i]]
		rec_j := TransactionsToSend[all_txs[j]]
		rate_i := rec_i.Fee * uint64(rec_j.Weight())
		rate_j := rec_j.Fee * uint64(rec_i.Weight())
		if rate_i != rate_j {
			return rate_i > rate_j
		}
		if rec_i.MemInputCnt != rec_j.MemInputCnt {
			return rec_i.MemInputCnt < rec_j.MemInputCnt
		}
		for x := 0; x < 32; x++ {
			if rec_i.Hash.Hash[x] != rec_j.Hash.Hash[x] {
				return rec_i.Hash.Hash[x] < rec_j.Hash.Hash[x]
			}
		}
		return false
	})

	// now put the childrer after the parents
	result = make([]*OneTxToSend, len(all_txs))
	already_in := make(map[BIDX]bool, len(all_txs))
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
		result[idx] = TransactionsToSend[txkey]
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

	for _, txkey := range all_txs {
		if missing, yes := missing_parents(txkey, false); yes {
			for _, kv := range missing {
				parent_of[kv] = append(parent_of[kv], txkey)
			}
			continue
		}
		append_txs(txkey)
	}

	if idx != len(result) || idx != len(already_in) || len(parent_of) != 0 {
		println("ERROR: Get sorted mempool idx:", idx, " result:", len(result), " alreadyin:", len(already_in), " parents:", len(parent_of))
		result = result[:idx]
	}

	return
}

// LimitPoolSize must be called with TxMutex locked.
func LimitPoolSize(maxlen uint64) {
	ticklen := maxlen >> 5 // 1/32th of the max size = X

	if TransactionsToSendSize < maxlen {
		if TransactionsToSendSize < maxlen-2*ticklen {
			if common.SetMinFeePerKB(0) {
				var cnt uint64
				for _, v := range TransactionsRejected {
					if v.Reason == TX_REJECTED_LOW_FEE {
						DeleteRejectedByTxr(v)
						cnt++
					}
				}
				common.CounterMutex.Lock()
				common.Count("TxPoolSizeLow")
				common.CountAdd("TxRejectedFeeUndone", cnt)
				common.CounterMutex.Unlock()
				//fmt.Println("Mempool size low:", TransactionsToSendSize, maxlen, maxlen-2*ticklen, "-", cnt, "rejected purged")
			}
		} else {
			common.CountSafe("TxPoolSizeOK")
			//fmt.Println("Mempool size OK:", TransactionsToSendSize, maxlen, maxlen-2*ticklen)
		}
		return
	}

	//sta := time.Now()

	sorted := GetSortedMempoolRBF()
	idx := len(sorted)

	old_size := TransactionsToSendSize

	maxlen -= ticklen

	for idx > 0 && TransactionsToSendSize > maxlen {
		idx--
		tx := sorted[idx]
		if _, ok := TransactionsToSend[tx.Hash.BIdx()]; !ok {
			// this has already been rmoved
			continue
		}
		tx.Delete(true, TX_REJECTED_LOW_FEE)
	}

	if cnt := len(sorted) - idx; cnt > 0 {
		newspkb := uint64(float64(1000*sorted[idx].Fee) / float64(sorted[idx].VSize()))
		common.SetMinFeePerKB(newspkb)

		/*fmt.Println("Mempool purged in", time.Now().Sub(sta).String(), "-",
		old_size-TransactionsToSendSize, "/", old_size, "bytes and", cnt, "/", len(sorted), "txs removed. SPKB:", newspkb)*/
		common.CounterMutex.Lock()
		common.Count("TxPoolSizeHigh")
		common.CountAdd("TxPurgedSizCnt", uint64(cnt))
		common.CountAdd("TxPurgedSizBts", old_size-TransactionsToSendSize)
		common.CounterMutex.Unlock()
	}
}

type OneTxsPackage struct {
	Txs    []*OneTxToSend
	Weight int
	Fee    uint64
}

func (pk *OneTxsPackage) AnyIn(list map[*OneTxToSend]bool) (ok bool) {
	for _, par := range pk.Txs {
		if _, ok = list[par]; ok {
			return
		}
	}
	return
}

func LookForPackages(txs []*OneTxToSend) (result []*OneTxsPackage) {
	for _, tx := range txs {
		if tx.MemInputCnt > 0 {
			continue
		}
		var pkg OneTxsPackage
		pandch := tx.GetItWithAllChildren()
		if len(pandch) > 1 {
			pkg.Txs = pandch
			for _, t := range pkg.Txs {
				pkg.Weight += t.Weight()
				pkg.Fee += t.Fee
			}
			result = append(result, &pkg)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Fee*uint64(result[j].Weight) > result[j].Fee*uint64(result[i].Weight)
	})
	return
}

// GetSortedMempoolRBF is like GetSortedMempool(), but one uses Child-Pays-For-Parent algo.
func GetSortedMempoolRBF() (result []*OneTxToSend) {
	txs := GetSortedMempool()
	pkgs := LookForPackages(txs)
	//println(len(pkgs), "pkgs from", len(txs), "txs")

	result = make([]*OneTxToSend, len(txs))
	var txs_idx, pks_idx, res_idx int
	already_in := make(map[*OneTxToSend]bool, len(txs))
	for txs_idx < len(txs) {
		tx := txs[txs_idx]

		if pks_idx < len(pkgs) {
			pk := pkgs[pks_idx]
			if pk.Fee*uint64(tx.Weight()) > tx.Fee*uint64(pk.Weight) {
				pks_idx++
				if pk.AnyIn(already_in) {
					continue
				}
				// all package's txs new: incude them all
				copy(result[res_idx:], pk.Txs)
				res_idx += len(pk.Txs)
				for _, _t := range pk.Txs {
					already_in[_t] = true
				}
				continue
			}
		}

		txs_idx++
		if _, ok := already_in[tx]; ok {
			continue
		}
		result[res_idx] = tx
		already_in[tx] = true
		res_idx++
	}
	//println("All sorted.  res_idx:", res_idx, "  txs:", len(txs))
	return
}

// GetMempoolFees only takes tx/package weight and the fee.
func GetMempoolFees(maxweight uint64) (result [][2]uint64) {
	txs := GetSortedMempool()
	pkgs := LookForPackages(txs)

	var txs_idx, pks_idx, res_idx int
	var weightsofar uint64
	result = make([][2]uint64, len(txs))
	already_in := make(map[*OneTxToSend]bool, len(txs))
	for txs_idx < len(txs) && weightsofar < maxweight {
		tx := txs[txs_idx]

		if pks_idx < len(pkgs) {
			pk := pkgs[pks_idx]
			if pk.Fee*uint64(tx.Weight()) > tx.Fee*uint64(pk.Weight) {
				pks_idx++
				if pk.AnyIn(already_in) {
					continue
				}

				result[res_idx] = [2]uint64{uint64(pk.Weight), pk.Fee}
				res_idx++
				weightsofar += uint64(pk.Weight)

				for _, _t := range pk.Txs {
					already_in[_t] = true
				}
				continue
			}
		}

		txs_idx++
		if _, ok := already_in[tx]; ok {
			continue
		}
		wg := tx.Weight()
		if wg == 0 {
			println("ERROR: weigth 0")
			println(tx.Hash.String())
			continue
		}
		result[res_idx] = [2]uint64{uint64(wg), tx.Fee}
		res_idx++
		weightsofar += uint64(tx.Weight())

		already_in[tx] = true
	}
	result = result[:res_idx]
	return
}

func ExpireOldTxs() {
	dur := common.Get(&common.TxExpireAfter)
	if dur == 0 {
		// tx expiting disabled
		//fmt.Print("ExpireOldTxs() - disabled\n> ")
		return
	}
	//fmt.Print("ExpireOldTxs()... ")
	expire_before := time.Now().Add(-dur)
	var todel []*OneTxToSend
	TxMutex.Lock()
	for _, v := range TransactionsToSend {
		if v.Lastseen.Before(expire_before) {
			todel = append(todel, v)
		}
	}
	if len(todel) > 0 {
		totcnt := len(TransactionsToSend)
		for _, vtx := range todel {
			// remove with all the children
			vtx.Delete(true, 0) // reason 0 does nont add it to the rejected list
		}
		totcnt -= len(TransactionsToSend)
		common.CountAdd("TxPoolExpParent", uint64(len(todel)))
		common.CountAdd("TxPoolExpChild", uint64(totcnt-len(todel)))
		fmt.Print("ExpireOldTxs: ", len(todel), " -> ", totcnt, " txs expired from mempool\n> ")
	} else {
		common.CountSafe("TxPoolExpireNone")
		//fmt.Println("nothing expired\n> ")
	}
	TxMutex.Unlock()
	common.CountSafe("TxPoolExpireTicks")
}

func LimitTxpoolSize() {
	lastTxsPoolLimit = time.Now()
	limitTxpoolSizeNow = false
	TxMutex.Lock()
	if maxpoolsize := common.MaxMempoolSize(); maxpoolsize != 0 {
		LimitPoolSize(maxpoolsize)
	}
	TxMutex.Unlock()
	common.CountSafe("TxPooLimitTicks")
}
