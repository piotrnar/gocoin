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
		fmt.Println("Get sorted mempool idx:", idx, " result:", len(result), " alreadyin:", len(already_in), " parents:", len(parent_of))
		fmt.Println("DUPA!!!!!!!!!!")
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
				for k, v := range TransactionsRejected {
					if v.Reason == TX_REJECTED_LOW_FEE {
						deleteRejected(k)
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
		common.Count("TxPoolSizeHigh")
		common.CountAdd("TxPurgedSizCnt", uint64(cnt))
		common.CountAdd("TxPurgedSizBts", old_size-TransactionsToSendSize)
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
		return int64(sorted[i].Size)*int64(now.Sub(sorted[i].Time)) < int64(sorted[j].Size)*int64(now.Sub(sorted[j].Time))
	})
	return
}

// LimitRejectedSize must be called with TxMutex locked.
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
			deleteRejected(sorted[idx].Id.BIdx())
		}
		sorted = sorted[:maxcnt]
	}

	if maxlen > 0 && TransactionsRejectedSize > maxlen {
		common.CountSafe("TxRejectedBtsHigh")
		if sorted == nil {
			sorted = GetSortedRejected()
		}
		maxlen -= maxlen >> 5
		for idx = len(sorted) - 1; idx >= 0; idx-- {
			deleteRejected(sorted[idx].Hash.BIdx())
			if TransactionsRejectedSize <= maxlen {
				break
			}
		}
	}

	if old_cnt > len(TransactionsRejected) {
		common.CounterMutex.Lock()
		common.CountAdd("TxRejectedSizCnt", uint64(old_cnt-len(TransactionsRejected)))
		common.CountAdd("TxRejectedSizBts", old_size-TransactionsRejectedSize)
		if common.GetBool(&common.CFG.TXPool.Debug) {
			println("Removed", uint64(old_cnt-len(TransactionsRejected)), "txs and", old_size-TransactionsRejectedSize,
				"bytes from the rejected poool")
		}
		common.CounterMutex.Unlock()
	}
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

// MempoolCheck verifies the Mempool for consistency.
// Make sure to call it with TxMutex Locked.
func MempoolCheck() (dupa bool) {
	var spent_cnt int
	var totsize uint64

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
		totsize += uint64(tr.Size)
	}
	if totsize != TransactionsRejectedSize {
		fmt.Println("TransactionsRejectedSize mismatch", totsize, TransactionsRejectedSize)
		dupa = true
	}

	return
}

// GetChildren gets all first level children of the tx.
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
	for ttx := range res {
		result[idx] = ttx
		idx++
	}
	return
}

// GetItWithAllChildren gets all the children (and all of their children...) of the tx.
// If any of the children has other unconfirmed parents, they are also included in the result.
// The result is sorted with the input parent first and always with parents before their children.
func (tx *OneTxToSend) GetItWithAllChildren() (result []*OneTxToSend) {
	already_included := make(map[*OneTxToSend]bool)

	result = []*OneTxToSend{tx} // out starting (parent) tx shall be the first element of the result
	already_included[tx] = true

	for idx := 0; idx < len(result); idx++ {
		par := result[idx]
		for _, ch := range par.GetChildren() {
			// do it for each returned child,

			// but only if it has not been included yet ...
			if _, ok := already_included[ch]; !ok {

				// first make sure we have all of its parents...
				for _, prnt := range ch.GetAllParentsExcept(par) {
					if _, ok := already_included[prnt]; !ok {
						// if we dont have a parent, just insert it here into the result
						result = append(result, prnt)
						// ... and mark it as included, for later
						already_included[prnt] = true
					}
				}

				// now we can safely insert the child, as all its parent shall be already included
				result = append(result, ch)
				// ... and mark it as included, for later
				already_included[ch] = true
			}
		}
	}
	return
}

// GetAllChildren gets all the children (and all of their children...) of the tx.
// The result is sorted by the oldest parent.
func (tx *OneTxToSend) GetAllChildren() (result []*OneTxToSend) {
	already_included := make(map[*OneTxToSend]bool)
	var idx int
	par := tx
	for {
		chlds := par.GetChildren()
		for _, ch := range chlds {
			if _, ok := already_included[ch]; !ok {
				already_included[ch] = true
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

// GetAllParents gets all the unconfirmed parents of the given tx.
// The result is sorted by the oldest parent.
func (tx *OneTxToSend) GetAllParents() (result []*OneTxToSend) {
	already_in := make(map[*OneTxToSend]bool)
	already_in[tx] = true
	var do_one func(*OneTxToSend)
	do_one = func(tx *OneTxToSend) {
		if tx.MemInputCnt > 0 {
			for idx := range tx.TxIn {
				if tx.MemInputs[idx] {
					par_tx := TransactionsToSend[btc.BIdx(tx.TxIn[idx].Input.Hash[:])]
					if _, ok := already_in[par_tx]; !ok {
						do_one(par_tx)
					}
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

// GetAllParents gets all the unconfirmed parents of the given tx, except for the input tx (and its parents).
// The result is sorted by the oldest parent.
func (tx *OneTxToSend) GetAllParentsExcept(except *OneTxToSend) (result []*OneTxToSend) {
	already_in := make(map[*OneTxToSend]bool)
	already_in[tx] = true
	var do_one func(*OneTxToSend)
	do_one = func(tx *OneTxToSend) {
		if tx.MemInputCnt > 0 {
			for idx := range tx.TxIn {
				if tx.MemInputs[idx] {
					if par_tx := TransactionsToSend[btc.BIdx(tx.TxIn[idx].Input.Hash[:])]; par_tx != except {
						if _, ok := already_in[par_tx]; !ok {
							do_one(par_tx)
						}
					}
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

/* This one uses the old method, which turned out to be very slow sometimes
func LookForPackages(txs []*OneTxToSend) (result []*OneTxsPackage) {
	for _, tx := range txs {
		if tx.MemInputCnt == 0 {
			continue
		}
		var pkg OneTxsPackage
		childs := tx.GetAllParents()
		if len(childs) > 0 {
			pkg.Txs = append(childs, tx)
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
*/

// GetSortedMempoolNew is like GetSortedMempool(), but one uses Child-Pays-For-Parent algo.
func GetSortedMempoolNew() (result []*OneTxToSend) {
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
		result[res_idx] = [2]uint64{uint64(tx.Weight()), tx.Fee}
		res_idx++
		weightsofar += uint64(tx.Weight())

		already_in[tx] = true
	}
	result = result[:res_idx]
	return
}

func ExpireOldTxs() {
	dur := common.GetDuration(&common.TxExpireAfter)
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

	LimitRejectedSize()

	TxMutex.Unlock()

	common.CountSafe("TxPollLImitTicks")
}
