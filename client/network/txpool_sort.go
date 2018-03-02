package network

import (
	"fmt"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"sort"
	"time"
)

var (
	expireTxsNow bool = true
	lastTxsExpire time.Time
)

// Return txs sorted by SPB, but with parents first
func GetSortedMempool() (result []*OneTxToSend) {
	all_txs := make([]BIDX, len(TransactionsToSend))
	var idx int
	const MIN_PKB = 200
	for k, _ := range TransactionsToSend {
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
			if rec_i.Hash.Hash[x] != rec_i.Hash.Hash[x] {
				return rec_i.Hash.Hash[x] < rec_i.Hash.Hash[x]
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
		fmt.Println("Get sorted mempool idx:", idx, " result:", len(result), " alreadyin:", len(already_in), " parents:", len(parent_of))
		fmt.Println("DUPA!!!!!!!!!!")
		result = result[:idx]
	}

	return
}

// This must be called with TxMutex locked
func LimitPoolSize(maxlen uint64) {
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
				//fmt.Println("Mempool size low:", TransactionsToSendSize, maxlen, maxlen-2*ticklen, "-", cnt, "rejected purged")
			}
		} else {
			common.CountSafe("TxPoolSizeOK")
			//fmt.Println("Mempool size OK:", TransactionsToSendSize, maxlen, maxlen-2*ticklen)
		}
		return
	}

	//sta := time.Now()

	sorted := GetSortedMempoolNew()
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
		common.Counter["TxPoolSizeHigh"]++
		common.Counter["TxPurgedSizCnt"] += uint64(cnt)
		common.Counter["TxPurgedSizBts"] += old_size - TransactionsToSendSize
		common.CounterMutex.Unlock()
	}
}


func GetSortedRejected() (sorted []*OneTxRejected) {
	var idx int
	sorted = make([]*OneTxRejected, len(TransactionsRejected))
	for _, t := range TransactionsRejected {
		sorted[idx] = t
		idx++
	}
	var now = time.Now()
	sort.Slice(sorted, func(i, j int) bool {
		return int64(sorted[i].Size) * int64(now.Sub(sorted[i].Time)) < int64(sorted[j].Size) * int64(now.Sub(sorted[j].Time))
	})
	println("rejected sorted:", sorted[0].Size, now.Sub(sorted[0].Time), "...", sorted[idx-1].Size, now.Sub(sorted[idx-1].Time))
	return
}


// This must be called with TxMutex locked
func LimitRejectedSize() {
	//ticklen := maxlen >> 5 // 1/32th of the max size = X
	var idx int
	var sorted []*OneTxRejected

	old_cnt := len(TransactionsRejected)
	old_size := TransactionsRejectedSize

	maxlen, maxcnt := common.RejectedTxsLimits()

	if maxcnt > 0 && len(TransactionsRejected) > maxcnt {
		common.CountSafe("TxRejectedCntHigh")
		sorted = GetSortedRejected()
		maxcnt -= maxcnt >> 5
		for idx = maxcnt; idx < len(sorted); idx++ {
			deleteRejected(sorted[idx].Hash.BIdx())
		}
		sorted = sorted[:maxcnt]
	}

	if maxlen > 0 && TransactionsRejectedSize > maxlen {
		common.CountSafe("TxRejectedBtsHigh")
		if sorted == nil {
			sorted = GetSortedRejected()
		}
		maxlen -= maxlen >> 5
		for idx = len(sorted)-1; idx >= 0; idx-- {
			deleteRejected(sorted[idx].Hash.BIdx())
			if TransactionsRejectedSize <= maxlen {
				break
			}
		}
	}

	if old_cnt > len(TransactionsRejected) {
		common.CounterMutex.Lock()
		common.Counter["TxRejectedSizCnt"] += uint64(old_cnt - len(TransactionsRejected))
		common.Counter["TxRejectedSizBts"] += old_size - TransactionsRejectedSize
		println("Removed", uint64(old_cnt - len(TransactionsRejected)), "txs and", old_size - TransactionsRejectedSize,
			"bytes from the rejected poool")
		common.CounterMutex.Unlock()
	}
}


// Verifies Mempool for consistency
func MempoolCheck() (dupa bool) {
	var spent_cnt int
	var totsize uint64

	TxMutex.Lock()

	// First check if t2s.MemInputs fields are properly set
	for _, t2s := range TransactionsToSend {
		var micnt int

		totsize += uint64(t2s.Size)

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
				if unsp, _ := common.BlockChain.Unspent.UnspentGet(&inp.Input); unsp == nil {
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

	TxMutex.Unlock()

	return
}

// Get all first level children of the tx
func (tx *OneTxToSend) GetChildren() (result []*OneTxToSend) {
	var po btc.TxPrevOut
	po.Hash = tx.Hash.Hash

	res := make(map[*OneTxToSend]bool)

	for po.Vout = 0; po.Vout < uint32(len(tx.TxOut)); po.Vout++ {
		uidx := po.UIdx()
		if val, ok := SpentOutputs[uidx]; ok {
			res[TransactionsToSend[val]] = true
		}
	}

	result = make([]*OneTxToSend, len(res))
	var idx int
	for ttx, _ := range res {
		result[idx] = ttx
		idx++
	}
	return
}

// Get all the children (and all of their children...) of the tx
// The result is sorted by the oldest parent
func (tx *OneTxToSend) GetAllChildren() (result []*OneTxToSend) {
	already_included := make(map[*OneTxToSend]bool)
	var idx int
	par := tx
	for {
		chlds := par.GetChildren()
		for _, ch := range chlds {
			if _, ok := already_included[ch]; !ok {
				result = append(result, ch)
			}
		}
		if idx == len(result) {
			break
		}

		par = result[idx]
		already_included[par] = true
		idx++
	}
	return
}

// Get all the parents of the given tx
// The result is sorted by the oldest parent
func (tx *OneTxToSend) GetAllParents() (result []*OneTxToSend) {
	already_in := make(map[*OneTxToSend]bool)
	already_in[tx] = true
	var do_one func(*OneTxToSend)
	do_one = func(tx *OneTxToSend) {
		if tx.MemInputCnt > 0 {
			for idx := range tx.TxIn {
				if tx.MemInputs[idx] {
					do_one(TransactionsToSend[btc.BIdx(tx.TxIn[idx].Input.Hash[:])])
				}
			}
		}
		if _, ok := already_in[tx]; !ok {
			result = append(result, tx)
			already_in[tx] = true
		}
	}
	do_one(tx)
	return
}

func (tx *OneTxToSend) SPW() float64 {
	return float64(tx.Fee) / float64(tx.Weight())
}

func (tx *OneTxToSend) SPB() float64 {
	return tx.SPW() * 4.0
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
		var pkg OneTxsPackage
		parents := tx.GetAllParents()
		if len(parents) > 0 {
			pkg.Txs = append(parents, tx)
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

// It is like GetSortedMempool(), but one uses Child-Pays-For-Parent algo
func GetSortedMempoolNew() (result []*OneTxToSend) {
	txs := GetSortedMempool()
	pkgs := LookForPackages(txs)

	result = make([]*OneTxToSend, len(txs))
	var txs_idx, pks_idx, res_idx int
	already_in := make(map[*OneTxToSend]bool, len(txs))
	for txs_idx < len(txs) {
		tx := txs[txs_idx]

		if pks_idx < len(pkgs) {
			pk := pkgs[pks_idx]
			if pk.Fee * uint64(tx.Weight()) > tx.Fee * uint64(pk.Weight) {
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

// Only take tx/package weight and the fee
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
			if pk.Fee * uint64(tx.Weight()) > tx.Fee * uint64(pk.Weight) {
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
		result[res_idx] = [2]uint64{uint64(tx.Weight()), tx.Fee}
		res_idx++
		weightsofar += uint64(tx.Weight())

		already_in[tx] = true
	}
	result = result[:res_idx]
	return
}

func ExpireTxs() {
	lastTxsExpire = time.Now()
	expireTxsNow = false

	TxMutex.Lock()

	if maxpoolsize := common.MaxMempoolSize(); maxpoolsize != 0 {
		LimitPoolSize(maxpoolsize)
	}

	LimitRejectedSize()

	TxMutex.Unlock()

	common.CountSafe("TxPurgedTicks")
}
