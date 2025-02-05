package txpool

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"slices"
	"sort"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
)

const (
	SORT_START_INDEX     = 0x1000000000000000 // 1/16th of max uint64 value
	SORT_INDEX_STEP      = 1e10
	POOL_EXPIRE_INTERVAL = time.Hour
)

var (
	nextTxsPoolExpire time.Time = time.Now().Add(POOL_EXPIRE_INTERVAL)
	BestT2S, WorstT2S *OneTxToSend

	SortingSupressed bool
	SortListDirty    bool

	FeePackages       []*OneTxsPackage = make([]*OneTxsPackage, 0, 10e3) // prealloc 10k records, which takes only 80KB of RAM but can save time later
	FeePackagesDirty  bool
	FeePackagesReSort bool
)

// call it with false to restore sorting
func BlockCommitInProgress(yes bool) {
	TxMutex.Lock()
	SortingSupressed = yes
	TxMutex.Unlock()
}

// insertes given tx into sorted list at its proper position
func (t2s *OneTxToSend) AddToSort() {
	if SortingSupressed {
		SortListDirty = true
		return
	}
	//fmt.Printf("adding %p / %s with spb %.2f  %p/%p\n", t2s, btc.BIdxString(t2s.Hash.BIdx()), t2s.SPB(), BestT2S, WorstT2S)

	if WorstT2S == nil || BestT2S == nil {
		if common.Get(&common.CFG.TXPool.CheckErrors) && (WorstT2S != nil || BestT2S != nil) {
			println("ERROR: if WorstT2S is nil BestT2S should be nil too", WorstT2S, BestT2S)
			WorstT2S, BestT2S = nil, nil
		}
		WorstT2S, BestT2S = t2s, t2s
		t2s.Better, t2s.Worse = nil, nil
		return
	}

	if wpr := t2s.findWorstParent(); wpr == nil {
		t2s.insertDownFromHere(BestT2S)
	} else {
		t2s.insertDownFromHere(wpr.Worse)
	}
}

func (t2s *OneTxToSend) insertDownFromHere(wpr *OneTxToSend) {
	for wpr != nil {
		if isFirstTxBetter(t2s, wpr) {
			t2s.insertBefore(wpr)
			return
		}
		wpr = wpr.Worse
	}
	// we reached the worst element - append it at the end
	WorstT2S.Worse = t2s
	t2s.Better = WorstT2S
	t2s.Worse = nil
	t2s.SortIndex = WorstT2S.SortIndex + SORT_INDEX_STEP
	WorstT2S = t2s
}

// this function is only called from withing BlockChain.CommitBlock()
func (t2s *OneTxToSend) ResortWithChildren() {
	// ... same for keeping mempool sorted, so we always suppress sorting before comitting a block
	if SortingSupressed { // But this check is here just in case
		SortListDirty = true
		return
	}

	// Normally we should not get here as blocks are precessed with SortingSupressed
	// So the code below is pretty much unused, although it has been seen working fine (but slow).
	common.CountSafe("TxMined**Resort")
	// now get the new worst parent
	wpr := t2s.findWorstParent()
	if wpr == t2s.Better {
		goto do_the_children
	}
	// we may have to move it. first let's remove it from the index
	if wpr == nil {
		wpr = BestT2S // if there is no parent, we can go all the way to the top
	} else {
		wpr = wpr.Worse // we must insert it after the worst parent (not before it)
	}
	if wpr.SortIndex > t2s.SortIndex {
		// we have to move it down the list as our parent is now below us
		t2s.DelFromSort()
		t2s.insertDownFromHere(wpr)
		common.CountSafe("TxSortDonwgr")
	} else {
		// our parent is above us - we can only move up the list
		// first check if we can move it at all
		one_above_us := t2s.Better
		if common.Get(&common.CFG.TXPool.CheckErrors) && one_above_us == nil {
			println("ERROR: we have a parent but we are on top")
			goto do_the_children
		}
		if !isFirstTxBetter(t2s, one_above_us) {
			common.CountSafe("TxSortAdvNO")
			goto do_the_children // we cannot move even by one, so stop trying
		}

		// we will move by at least one, so we can delete the record now
		t2s.DelFromSort()
		if common.Get(&common.CFG.TXPool.CheckErrors) && (BestT2S == nil || WorstT2S == nil) {
			println("ERROR: we have a parent but the list is empty after we removed ourselves")
			return // we dont need to check for children as there obviously arent any records left
		}

		// this is version 2 - from top to bottom:
		common.CountSafe("TxSortAdvDOWN")
		for wpr != one_above_us {
			if isFirstTxBetter(t2s, wpr) {
				t2s.insertBefore(wpr)
				common.CountSafe("TxSortImporveA")
				goto do_the_children // we cannot move even by one, so stop trying
			}
			wpr = wpr.Worse
		}
		// we reached one above os which we already know that we can skip
		common.CountSafe("TxSortImporveB")
		t2s.insertBefore(wpr)
		goto do_the_children // we cannot move even by one, so stop trying
	}

do_the_children:
	// now do the children
	var po btc.TxPrevOut
	po.Hash = t2s.Hash.Hash
	for po.Vout = 0; po.Vout < uint32(len(t2s.TxOut)); po.Vout++ {
		uidx := po.UIdx()
		if val, ok := SpentOutputs[uidx]; ok {
			if rec, ok := TransactionsToSend[val]; ok {
				rec.ResortWithChildren()
				//println("Resorted", btc.BIdxString(val), "becase of", btc.BIdxString(t2s.Hash.BIdx()))
			}
		}
	}
}

// removes given tx from the sorted list
func (t2s *OneTxToSend) DelFromSort() {
	if SortingSupressed {
		SortListDirty = true
		return
	}
	if t2s == BestT2S {
		if t2s == WorstT2S {
			BestT2S, WorstT2S = nil, nil
		} else {
			BestT2S = BestT2S.Worse
			BestT2S.Better = nil
		}
		return
	}
	if t2s == WorstT2S {
		if t2s == BestT2S {
			BestT2S, WorstT2S = nil, nil
		} else {
			WorstT2S = WorstT2S.Better
			WorstT2S.Worse = nil
		}
		return
	}
	if common.Get(&common.CFG.TXPool.CheckErrors) {
		if t2s.Worse == nil {
			println("ERROR: t2s.Worse is nil but t2s was not WorstT2S", WorstT2S, BestT2S, t2s.Worse)
			debug.PrintStack()
			os.Exit(1)
		}
		if t2s.Worse.Better != t2s {
			println("ERROR: t2s.Worse.Better is not pointing to t2s", WorstT2S, BestT2S, t2s, t2s.Worse, t2s.Worse.Better)
			debug.PrintStack()
			os.Exit(1)
		}
	}
	t2s.Worse.Better = t2s.Better

	if common.Get(&common.CFG.TXPool.CheckErrors) {
		if t2s.Better == nil {
			println("ERROR: t2s.Better is nil but t2s was not BestT2S", WorstT2S, BestT2S, t2s.Better)
			debug.PrintStack()
			os.Exit(1)
		}
		if t2s.Better.Worse != t2s {
			println("ERROR: t2s.Better.Worse is not pointing to t2s", WorstT2S, BestT2S, t2s, t2s.Better, t2s.Better.Worse)
			debug.PrintStack()
			os.Exit(1)
		}
	}
	t2s.Better.Worse = t2s.Worse
}

func VerifyMempoolSort(txs []*OneTxToSend) bool {
	idxs := make(map[btc.BIDX]int, len(txs))
	for i, t2s := range txs {
		if t2s == nil {
			println("tx at idx", i, len(txs), len(TransactionsToSend), "is nil")
			return true
		}
		idxs[t2s.Hash.BIdx()] = i
	}
	var oks int
	for i, t2s := range txs {
		if t2s.Weight() == 0 {
			println("ERROR: in mempool sorting:", i, "has weight 0", t2s.Hash.String())
			return true
		}
		for _, txin := range t2s.TxIn {
			if idx, ok := idxs[btc.BIdx(txin.Input.Hash[:])]; ok {
				if idx > i {
					println("ERROR: in mempool sorting:", i, "points to", idx, "\n",
						"    ", i, t2s.Hash.String(), "\n",
						" -> ", idx, btc.NewUint256(txin.Input.Hash[:]).String())
					return true
				} else {
					oks++
				}
			}
		}
	}
	//println("mempool sorting OK", oks, len(txs))
	return false
}

func (t2s *OneTxToSend) findWorstParent() (wpr *OneTxToSend) {
	for i, mi := range t2s.MemInputs {
		if mi {
			parent_bidx := btc.BIdx(t2s.Tx.TxIn[i].Input.Hash[:])
			parent := TransactionsToSend[parent_bidx]
			if common.Get(&common.CFG.TXPool.CheckErrors) && parent == nil {
				println("ERROR: not existing parent", btc.BIdxString(parent_bidx), "for", t2s.Hash.String())
				return
			}
			if wpr == nil || parent.SortIndex > wpr.SortIndex {
				wpr = parent
			}
		}
	}
	return
}

func (t2s *OneTxToSend) insertBefore(wpr *OneTxToSend) {
	if wpr == BestT2S {
		BestT2S = t2s
		t2s.Better = nil
	} else {
		wpr.Better.Worse = t2s
		t2s.Better = wpr.Better
	}
	t2s.Worse = wpr
	wpr.Better = t2s
	t2s.fixIndex()
}

func (t2s *OneTxToSend) fixIndex() {
	if t2s.Better == nil {
		if t2s.Worse == nil {
			t2s.SortIndex = SORT_START_INDEX
			return
		}
		if t2s.Worse.SortIndex > SORT_INDEX_STEP {
			t2s.SortIndex = t2s.Worse.SortIndex - SORT_INDEX_STEP
			return
		}
		t2s.SortIndex = t2s.Worse.SortIndex / 2
		if t2s.SortIndex == t2s.Worse.SortIndex {
			t2s.SortIndex = SORT_START_INDEX
			cnt, _ := t2s.reindexDown(SORT_INDEX_STEP)
			common.CountSafeAdd("TxSortReindexALL", cnt)
			return
		}
	}

	better_idx := t2s.Better.SortIndex
	if t2s.Worse == nil {
		t2s.SortIndex = better_idx + SORT_INDEX_STEP
		return
	}

	diff := t2s.Worse.SortIndex - better_idx
	if diff >= 2 {
		t2s.SortIndex = better_idx + diff/2
		return
	}

	// we will have tp reindex down
	cnt, end := t2s.Better.reindexDown(SORT_INDEX_STEP / 4)
	if end {
		common.CountSafeAdd("TxSortReindexEnd", cnt)
	} else {
		common.CountSafeAdd("TxSortReindexMid", cnt)
	}
}

func (t *OneTxToSend) reindexDown(step uint64) (cnt uint64, toend bool) {
	index := t.SortIndex
	for t = t.Worse; t != nil; t = t.Worse {
		index += step
		if t.SortIndex >= index {
			return
		}
		t.SortIndex = index
		cnt++
	}
	toend = true
	return
}

func isFirstTxBetter(rec_i, rec_j *OneTxToSend) bool {
	rate_i := rec_i.Fee * uint64(rec_j.Weight())
	rate_j := rec_j.Fee * uint64(rec_i.Weight())
	if rate_i != rate_j {
		return rate_i > rate_j
	}
	if rec_i.MemInputCnt != rec_j.MemInputCnt {
		return rec_i.MemInputCnt < rec_j.MemInputCnt
	}
	return binary.LittleEndian.Uint64(rec_i.Hash.Hash[:btc.Uint256IdxLen]) >
		binary.LittleEndian.Uint64(rec_j.Hash.Hash[:btc.Uint256IdxLen])
}

// GetSortedMempool returns txs sorted by SPB, but with parents first.
// Make sure to call it with TxMutex locked
func GetSortedMempoolSlow() (result []*OneTxToSend) {
	all_txs := make([]btc.BIDX, len(TransactionsToSend))
	var idx int
	for k := range TransactionsToSend {
		all_txs[idx] = k
		idx++
	}
	sort.Slice(all_txs, func(i, j int) bool {
		rec_i := TransactionsToSend[all_txs[i]]
		rec_j := TransactionsToSend[all_txs[j]]
		return isFirstTxBetter(rec_i, rec_j)
	})

	// now put the childrer after the parents
	result = make([]*OneTxToSend, len(all_txs))
	already_in := make(map[btc.BIDX]bool, len(all_txs))
	parent_of := make(map[btc.BIDX][]btc.BIDX)

	idx = 0

	var missing_parents = func(txkey btc.BIDX, is_any bool) (res []btc.BIDX, yes bool) {
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

	var append_txs func(txkey btc.BIDX)
	append_txs = func(txkey btc.BIDX) {
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

	if common.Get(&common.CFG.TXPool.CheckErrors) && (idx != len(result) || idx != len(already_in) || len(parent_of) != 0) {
		println("ERROR: Get sorted mempool idx:", idx, " result:", len(result), " alreadyin:", len(already_in), " parents:", len(parent_of))
		result = result[:idx]
	}

	return
}

type OneTxsPackage struct {
	Txs    []*OneTxToSend
	Weight int
	Fee    uint64
}

func (pk *OneTxsPackage) String() (res string) {
	var id string
	if len(pk.Txs) == 0 {
		id = "xxxxxxxxxxxxxxxx"
	} else {
		id = pk.Txs[0].Hash.String()[:16]
	}
	res = fmt.Sprintf("Id:%s  SPB:%.5f  Txs:%d", id, 4.0*float64(pk.Fee)/float64(pk.Weight), len(pk.Txs))
	return
}

func (pk *OneTxsPackage) anyIn(list map[*OneTxToSend]bool) (ok bool) {
	if CheckForErrors() {
		pk.checkForDups()
	}
	for _, par := range pk.Txs {
		if _, ok = list[par]; ok {
			return
		}
	}
	return
}

func (pk *OneTxsPackage) hasAllTheParentsFor(child *OneTxToSend) bool {
	if len(pk.Txs)*len(child.TxIn) > 10 {
		m := make(map[btc.BIDX]bool, len(pk.Txs))
		for _, t := range pk.Txs {
			m[t.Hash.BIdx()] = true
		}
		for _, in := range child.TxIn {
			if !m[btc.BIdx(in.Input.Hash[:])] {
				return false
			}
		}
	} else {
		has_parent_for := func(txid []byte) bool {
			for _, t := range pk.Txs {
				if bytes.Equal(t.Hash.Hash[:], txid) {
					return true
				}
			}
			return false
		}
		for _, in := range child.TxIn {
			if !has_parent_for(in.Input.Hash[:]) {
				return false
			}
		}
	}
	return true
}

func (pk *OneTxsPackage) checkForDups() bool {
	for i, t := range pk.Txs[:len(pk.Txs)-1] {
		if slices.Contains(pk.Txs[i+1:], t) {
			println("ERROR: pkg", pk.String(), "contains the same tx twice:", t.Hash.String())
			if rdbg != nil {
				println(rdbg.String())
			}
			debug.PrintStack()
			os.Exit(1)
			return true
		}
	}
	return false
}

var (
	SortFeePackagesTime  time.Duration
	SortFeePackagesCount uint

	LookForPackagesTime  time.Duration
	LookForPackagesCount uint
)

func sortFeePackages() {
	if FeePackagesReSort {
		FeePackagesReSort = false
		common.CountSafe("TxPkgsSortDo")
		sta := time.Now()
		sort.Slice(FeePackages, func(i, j int) bool {
			iv := FeePackages[i].Fee * uint64(FeePackages[j].Weight)
			jv := FeePackages[j].Fee * uint64(FeePackages[i].Weight)
			if iv != jv {
				return iv > jv
			}
			return binary.LittleEndian.Uint64(FeePackages[i].Txs[0].Hash.Hash[:8]) >
				binary.LittleEndian.Uint64(FeePackages[j].Txs[0].Hash.Hash[:8])
			//return FeePackages[i].Fee*uint64(FeePackages[j].Weight) > FeePackages[j].Fee*uint64(FeePackages[i].Weight)
		})
		SortFeePackagesTime += time.Since(sta)
		SortFeePackagesCount++
	} else {
		common.CountSafe("TxPkgsSortSkip")
	}
}

func dumpPkgList(fn string) {
	f, _ := os.Create(fn)
	dumpPkgListHere(f)
	f.Close()
	println("pkg list stored in", fn)
}

func dumpPkgListHere(f io.Writer) {
	for _, pkg := range FeePackages {
		fmt.Fprintln(f, "package", pkg.String(), "with", len(pkg.Txs), "txs:")
		for _, t := range pkg.Txs {
			fmt.Fprintln(f, "   *", t.Hash.String(), "  mic:", t.MemInputCnt, "  inpkgs:", len(t.InPackages))
			for idx, pkg := range t.InPackages {
				fmt.Fprintln(f, "   ", idx, pkg.String())
			}
		}
		fmt.Fprintln(f)
	}
}

// builds FeePackages list, if neccessary
func lookForPackages() {
	defer sortFeePackages()
	buildSortedList()
	if !FeePackagesDirty {
		common.CountSafe("TxPkgsHaveThem")
		return
	}
	common.CountSafe("TxPkgsNeedThem")
	sta := time.Now()
	FeePackages = FeePackages[:0] // to avoid unneeded memory allocation, just reuse the old buffer
	for t2s := BestT2S; t2s != nil; t2s = t2s.Worse {
		t2s.InPackages = nil // we have to do this first run only to reset InPackages field
	}
	for t2s := BestT2S; t2s != nil; t2s = t2s.Worse {
		if t2s.MemInputCnt > 0 {
			continue
		}

		if pandch := t2s.GetItWithAllChildren(); len(pandch) > 1 {
			pkg := &OneTxsPackage{Txs: pandch}
			for _, t := range pandch {
				pkg.Weight += t.Weight()
				pkg.Fee += t.Fee
				t.InPackages = append(t.InPackages, pkg)
			}
			FeePackages = append(FeePackages, pkg)
		}
	}
	FeePackagesReSort = true
	FeePackagesDirty = false
	LookForPackagesTime += time.Since(sta)
	LookForPackagesCount++
}

// GetSortedMempoolRBF is like GetSortedMempool(), but one uses Child-Pays-For-Parent algo.
func GetSortedMempoolRBF() (result []*OneTxToSend) {
	lookForPackages()
	result = make([]*OneTxToSend, len(TransactionsToSend))
	var pks_idx, res_idx, cnt int
	already_in := make(map[*OneTxToSend]bool, len(TransactionsToSend))
	for tx := BestT2S; tx != nil; tx = tx.Worse {
		cnt++
		for pks_idx < len(FeePackages) {
			if pk := FeePackages[pks_idx]; pk.Fee*uint64(tx.Weight()) > tx.Fee*uint64(pk.Weight) {
				pks_idx++
				if pk.anyIn(already_in) {
					continue
				}
				for _, _t := range pk.Txs {
					already_in[_t] = true
				}
				copy(result[res_idx:], pk.Txs)
				res_idx += len(pk.Txs)
				continue
			}
			break
		}

		if _, ok := already_in[tx]; !ok {
			result[res_idx] = tx
			already_in[tx] = true
			res_idx++
		}
	}
	return
}

// GetMempoolFees only takes tx/package weight and the fee.
func GetMempoolFees(maxweight uint64) (result [][2]uint64) {
	lookForPackages() // it will do buildSortedList() if needed

	var txs_idx, pks_idx, res_idx int
	var weightsofar uint64
	result = make([][2]uint64, len(TransactionsToSend))
	already_in := make(map[*OneTxToSend]bool, len(TransactionsToSend))
	for tx := BestT2S; tx != nil && weightsofar < maxweight; tx = tx.Worse {
		for pks_idx < len(FeePackages) {
			pk := FeePackages[pks_idx]
			if pk.Fee*uint64(tx.Weight()) > tx.Fee*uint64(pk.Weight) {
				pks_idx++
				if pk.anyIn(already_in) {
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
			break
		}
		txs_idx++
		if _, ok := already_in[tx]; !ok {
			wg := tx.Weight()
			result[res_idx] = [2]uint64{uint64(wg), tx.Fee}
			res_idx++
			weightsofar += uint64(tx.Weight())
			already_in[tx] = true
		}
	}
	result = result[:res_idx]
	return
}

func ExpireOldTxs() {
	if time.Now().Before(nextTxsPoolExpire) {
		return
	}
	nextTxsPoolExpire = time.Now().Add(POOL_EXPIRE_INTERVAL)

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
			// make sure it was not deleted as a child of one of the previous txs
			if _, ok := TransactionsToSend[vtx.Hash.BIdx()]; !ok {
				common.CountSafe("TxPoolExpSkept")
				continue
			}
			// remove with all the children
			vtx.Delete(true, 0) // reason 0 does nont add it to the rejected list
		}
		totcnt -= len(TransactionsToSend)
		common.CountSafeAdd("TxPoolExpParent", uint64(len(todel)))
		common.CountSafeAdd("TxPoolExpChild", uint64(totcnt-len(todel)))
		//fmt.Print("ExpireOldTxs: ", len(todel), " -> ", totcnt, " txs expired from mempool\n> ")
	} else {
		common.CountSafe("TxPoolExpireNone")
		//fmt.Println("nothing expired\n> ")
	}
	TxMutex.Unlock()
	common.CountSafe("TxPoolExpireTicks")
}

// Make sure to call it with TxMutex locked
func GetSortedMempool() (result []*OneTxToSend) {
	if SortListDirty {
		return GetSortedMempoolSlow()
	}

	result = make([]*OneTxToSend, 0, len(TransactionsToSend))
	var prv_idx uint64
	for t2s := BestT2S; t2s != nil; t2s = t2s.Worse {
		if common.Get(&common.CFG.TXPool.CheckErrors) && (prv_idx != 0 && prv_idx >= t2s.SortIndex) {
			println("ERROR: GetSortedMempool corupt sort index", len(TransactionsToSend), prv_idx, t2s.SortIndex)
		}
		prv_idx = t2s.SortIndex
		result = append(result, t2s)
	}
	return
}

// call it with the mutex locked
func buildSortedList() {
	common.CountSafePar("TxSortBuild-", SortingSupressed)
	if !SortListDirty {
		common.CountSafe("TxSortBuildSkept")
		return
	}
	common.CountSafe("TxSortBuildNeeded")
	SortListDirty = false
	ts := GetSortedMempoolSlow()
	if len(ts) == 0 {
		BestT2S, WorstT2S = nil, nil
		//fmt.Println("BuildSortedList: Mempool empty")
		return
	}
	var SortIndex uint64
	BestT2S, WorstT2S = ts[0], ts[0]
	BestT2S.Better, BestT2S.Worse = nil, nil
	WorstT2S.Better, WorstT2S.Worse = nil, nil
	SortIndex = SORT_START_INDEX
	BestT2S.SortIndex = SortIndex
	for _, t2s := range ts[1:] {
		SortIndex += SORT_INDEX_STEP
		t2s.SortIndex = SortIndex
		t2s.Better = WorstT2S
		WorstT2S.Worse = t2s
		WorstT2S = t2s
	}
	WorstT2S.Worse = nil
}
