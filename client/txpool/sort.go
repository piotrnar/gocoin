package txpool

import (
	"os"
	"runtime/debug"
	"sort"
	"sync/atomic"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
)

const (
	SORT_START_INDEX     = uint64(1 << 62) // 1/4th of max uint64 value
	POOL_EXPIRE_INTERVAL = time.Hour
)

var (
	nextTxsPoolExpire time.Time = time.Now().Add(POOL_EXPIRE_INTERVAL)
	BestT2S, WorstT2S *OneTxToSend
	SortListDirty     bool   // means the BestT2S <--> WorstT2S list is useless and needs rebuilding
	sortIndexStep     uint64 // this is dynamically calculated in adjustSortIndexStep()
	sortingSupressed  bool
	LastSortingDone   time.Time

	// statistics of how far the SortRank values have moved around the uint64 space:
	SortRankRangeValid       bool
	SortRankMin, SortRankMax uint64
)

func updateSortWidthStats() {
	if BestT2S == nil || WorstT2S == nil {
		return
	}
	if !SortRankRangeValid {
		SortRankMin = BestT2S.SortRank
		SortRankMax = WorstT2S.SortRank
		SortRankRangeValid = true
		return
	}
	if BestT2S.SortRank < SortRankMin {
		SortRankMin = BestT2S.SortRank
	} else if WorstT2S.SortRank > SortRankMax {
		SortRankMax = WorstT2S.SortRank
	}
}

func adjustSortIndexStep() {
	cnt := len(TransactionsToSend)
	if cnt < 100e3 {
		cnt = 100e3
	}
	sortIndexStep = (1 << 60) / uint64(2*cnt)
}

// make sure to call it with the mutex locked
func SortingDisabled() bool {
	if sortingSupressed {
		return true
	}
	stopafter := atomic.LoadUint64(&common.StopAutoSortAfter)
	return stopafter != 0 && time.Since(LastSortingDone) > time.Duration(stopafter)
}

// call it with false to restore sorting
func BlockCommitInProgress(yes bool) {
	TxMutex.Lock()
	sortingSupressed = yes
	if !yes && FeePackagesDirty && SortingDisabled() {
		emptyFeePackages() // this should free all the memory used by packages
	}
	TxMutex.Unlock()
}

// insertes given tx into sorted list at its proper position
func (t2s *OneTxToSend) AddToSort() {
	if SortListDirty {
		return
	}
	if SortingDisabled() {
		SortListDirty = true
		return
	}
	//fmt.Printf("adding %p / %s with spb %.2f  %p/%p\n", t2s, btc.BIdxString(t2s.Hash.BIdx()), t2s.SPB(), BestT2S, WorstT2S)
	sta := time.Now()
	defer func() {
		ResortingSinceLastRedoTime += time.Since(sta)
		ResortingSinceLastRedoCount++
	}()
	if WorstT2S == nil || BestT2S == nil {
		if CheckForErrors() && (WorstT2S != nil || BestT2S != nil) {
			println("ERROR: if WorstT2S is nil BestT2S should be nil too", WorstT2S, BestT2S)
			WorstT2S, BestT2S = nil, nil
		}
		WorstT2S, BestT2S = t2s, t2s
		t2s.better, t2s.worse = nil, nil
		return
	}

	if wpr := t2s.findWorstParent(); wpr == nil {
		t2s.insertDownFromHere(BestT2S)
	} else {
		t2s.insertDownFromHere(wpr.worse)
	}
}

func (t2s *OneTxToSend) insertDownFromHere(wpr *OneTxToSend) {
	for wpr != nil { // we have to do it from top-down
		if isFirstTxBetter(t2s, wpr) {
			t2s.insertBefore(wpr)
			return
		}
		wpr = wpr.worse
	}
	// we reached the worst element - append it at the end
	WorstT2S.worse = t2s
	t2s.better = WorstT2S
	t2s.worse = nil
	t2s.SortRank = WorstT2S.SortRank + sortIndexStep
	WorstT2S = t2s
}

// this function is only called from withing BlockChain.CommitBlock()
func (t2s *OneTxToSend) resortWithChildren() {
	if SortingDisabled() {
		SortListDirty = true
		return
	}
	if SortListDirty {
		return
	}

	common.CountSafe("TxMinedResort")

	// now get the new worst parent
	wpr := t2s.findWorstParent()
	if wpr == t2s.better {
		goto do_the_children
	}
	// we may have to move it. first let's remove it from the index
	if wpr == nil {
		wpr = BestT2S // if there is no parent, we can go all the way to the top
	} else if wpr.worse != nil {
		wpr = wpr.worse // we must insert it after the worst parent (not before it)
	}
	if wpr.SortRank > t2s.SortRank {
		// we have to move it down the list as our parent is now below us
		t2s.DelFromSort()
		t2s.insertDownFromHere(wpr)
		common.CountSafe("TxSortDonwgr")
	} else {
		// our parent is above us - we can only move up the list
		// first check if we can move it at all
		one_above_us := t2s.better
		if CheckForErrors() && one_above_us == nil {
			println("ERROR: we have a parent but we are on top")
			goto do_the_children
		}
		if !isFirstTxBetter(t2s, one_above_us) {
			common.CountSafe("TxSortAdvNO")
			goto do_the_children // we cannot move even by one, so stop trying
		}

		// we will move by at least one, so we can delete the record now
		t2s.DelFromSort()
		if CheckForErrors() && (BestT2S == nil || WorstT2S == nil) {
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
			wpr = wpr.worse
		}
		// we reached one above os which we already know that we can skip
		common.CountSafe("TxSortImporveB")
		t2s.insertBefore(wpr)
		goto do_the_children // we cannot move even by one, so stop trying
	}

do_the_children:
	common.CountSafe("TxSortDoChildren")
	// now do the children
	for vout := range t2s.TxOut {
		uidx := btc.UIdx(t2s.Hash.Hash[:], uint32(vout))
		if val, ok := SpentOutputs[uidx]; ok {
			if rec, ok := TransactionsToSend[val]; ok {
				rec.resortWithChildren()
			}
		}
	}
}

// removes given tx from the sorted list
func (t2s *OneTxToSend) DelFromSort() {
	if SortListDirty {
		return
	}
	if SortingDisabled() {
		SortListDirty = true
		return
	}
	sta := time.Now()
	defer func() {
		ResortingSinceLastRedoTime += time.Since(sta)
		ResortingSinceLastRedoCount++
	}()
	if t2s == BestT2S {
		if t2s == WorstT2S {
			BestT2S, WorstT2S = nil, nil
		} else {
			BestT2S = BestT2S.worse
			BestT2S.better = nil
		}
		return
	}
	if t2s == WorstT2S {
		if t2s == BestT2S {
			BestT2S, WorstT2S = nil, nil
		} else {
			WorstT2S = WorstT2S.better
			WorstT2S.worse = nil
		}
		return
	}
	if CheckForErrors() {
		if t2s.worse == nil {
			println("ERROR: t2s.Worse is nil but t2s was not WorstT2S", WorstT2S, BestT2S, t2s.worse)
			debug.PrintStack()
			os.Exit(1)
		}
		if t2s.worse.better != t2s {
			println("ERROR: t2s.Worse.Better is not pointing to t2s", WorstT2S, BestT2S, t2s, t2s.worse, t2s.worse.better)
			debug.PrintStack()
			os.Exit(1)
		}
	}
	t2s.worse.better = t2s.better

	if CheckForErrors() {
		if t2s.better == nil {
			println("ERROR: t2s.Better is nil but t2s was not BestT2S", WorstT2S, BestT2S, t2s.better)
			debug.PrintStack()
			os.Exit(1)
		}
		if t2s.better.worse != t2s {
			println("ERROR: t2s.Better.Worse is not pointing to t2s", WorstT2S, BestT2S, t2s, t2s.better, t2s.better.worse)
			debug.PrintStack()
			os.Exit(1)
		}
	}
	t2s.better.worse = t2s.worse
}

func (t2s *OneTxToSend) findWorstParent() (wpr *OneTxToSend) {
	for i, mi := range t2s.MemInputs {
		if mi {
			parent_bidx := btc.BIdx(t2s.Tx.TxIn[i].Input.Hash[:])
			parent := TransactionsToSend[parent_bidx]
			if CheckForErrors() && parent == nil {
				println("ERROR: not existing parent", btc.BIdxString(parent_bidx), "for", t2s.Hash.String())
				return
			}
			if wpr == nil || parent.SortRank > wpr.SortRank {
				wpr = parent
			}
		}
	}
	return
}

func (t2s *OneTxToSend) insertBefore(wpr *OneTxToSend) {
	if wpr == BestT2S {
		BestT2S = t2s
		t2s.better = nil
	} else {
		wpr.better.worse = t2s
		t2s.better = wpr.better
	}
	t2s.worse = wpr
	wpr.better = t2s
	t2s.fixIndex()
	updateSortWidthStats()
}

/* leave it - may be useful for debugging
func (t2s *OneTxToSend) idx() (cnt int) {
	for t := BestT2S; t != nil; t = t.Worse {
		if t == t2s {
			break
		}
		cnt++
	}
	return
}
*/

func (t2s *OneTxToSend) fixIndex() {
	if t2s.better == nil {
		if t2s.worse == nil {
			t2s.SortRank = SORT_START_INDEX
			common.CountSafe("TxSortHadLot-A1")
			return
		}
		if t2s.worse.SortRank > sortIndexStep {
			t2s.SortRank = t2s.worse.SortRank - sortIndexStep
			common.CountSafe("TxSortHadLot-A2")
			return
		}
		t2s.SortRank = t2s.worse.SortRank / 2
		if t2s.SortRank == t2s.worse.SortRank {
			common.CountSafe("TxSortHNoLot-A3")
			reindexEverything()
			return
		}
	}

	better_idx := t2s.better.SortRank
	if t2s.worse == nil {
		t2s.SortRank = better_idx + sortIndexStep
		common.CountSafe("TxSortHadLot-B")
		return
	}

	diff := t2s.worse.SortRank - better_idx
	if diff >= 2 {
		t2s.SortRank = better_idx + diff/2
		//common.CountSafe("TxSortHadLot-C")  <-- there would be too many of them
		return
	}

	t2s.better.reindexDown(sortIndexStep / 16)
}

func (t *OneTxToSend) reindexDown(step uint64) {
	var cnt uint64
	var toend, overflow bool
	defer func() {
		if overflow {
			common.CountSafe("TxSortReinCnt-OFW")
			common.CountSafeAdd("TxSortReinRec-OFW", cnt)
			reindexEverything()
			return
		}
		if toend {
			common.CountSafe("TxSortReinCnt-End")
			common.CountSafeAdd("TxSortReinRec-End", cnt)
		} else {
			common.CountSafe("TxSortReinCnt-Mid")
			common.CountSafeAdd("TxSortReinRec-Mid", cnt)
		}
	}()
	index := t.SortRank
	for t = t.worse; t != nil; t = t.worse {
		new_index := index + step
		if new_index < index {
			overflow = true
			return
		}
		index += step
		if t.SortRank >= index {
			return
		}
		t.SortRank = index
		cnt++
	}
	toend = true
}

func reindexEverything() {
	common.CountSafe("TxSortReindexAll")
	adjustSortIndexStep()
	index := uint64(SORT_START_INDEX)
	for t := BestT2S; t != nil; t = t.worse {
		t.SortRank = index
		index += sortIndexStep
	}
}

func isFirstTxBetter(rec_i, rec_j *OneTxToSend) bool {
	// this method of sorting is faster, but harder for debugging
	return rec_i.Fee*uint64(rec_j.Weight()) > rec_j.Fee*uint64(rec_i.Weight())
	/* this one is slower but may be useful for debugging
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
	*/
}

// call it with the mutex locked
func buildSortedList() {
	if SortingDisabled() {
		common.CountSafePar("TxSortBuildInSusp-", SortListDirty)
	}
	if !SortListDirty {
		common.CountSafe("TxSortBuildSkept")
		return
	}
	common.CountSafe("TxSortBuildNeeded")
	SortListDirty = false
	ResortingSinceLastRedoTime = 0
	ResortingSinceLastRedoCount = 0
	ResortingSinceLastRedoWhen = time.Now()
	ts := GetSortedMempoolSlow()
	if len(ts) == 0 {
		BestT2S, WorstT2S = nil, nil
		//fmt.Println("BuildSortedList: Mempool empty")
		return
	}
	var SortIndex uint64
	BestT2S, WorstT2S = ts[0], ts[0]
	BestT2S.better, BestT2S.worse = nil, nil
	WorstT2S.better, WorstT2S.worse = nil, nil
	SortIndex = SORT_START_INDEX
	BestT2S.SortRank = SortIndex
	adjustSortIndexStep()
	for _, t2s := range ts[1:] {
		SortIndex += sortIndexStep
		t2s.SortRank = SortIndex
		t2s.better = WorstT2S
		WorstT2S.worse = t2s
		WorstT2S = t2s
	}
	WorstT2S.worse = nil
	updateSortWidthStats()
}

func expireOldTxs() {
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
	common.CountSafe("TxPoolExpireTicks")
}

// GetSortedMempoolSlow returns txs sorted by SPB, but with parents first.
// It does not use the sort list and does all the sorting inside the function.
// Make sure to call it with TxMutex locked
func GetSortedMempoolSlow() (result []*OneTxToSend) {
	all_txs := make([]*OneTxToSend, 0, len(TransactionsToSend))
	result = make([]*OneTxToSend, 0, len(TransactionsToSend))
	already_in := make(map[*OneTxToSend]bool, len(TransactionsToSend))
	parent_of := make(map[*OneTxToSend][]*OneTxToSend)

	for _, tx := range TransactionsToSend {
		all_txs = append(all_txs, tx)
	}
	sort.Slice(all_txs, func(i, j int) bool {
		return isFirstTxBetter(all_txs[i], all_txs[j])
	})

	// now put the children after the parents
	missing_parents := func(tx *OneTxToSend, is_any bool) (res []*OneTxToSend, yes bool) {
		if tx.MemInputs == nil {
			return
		}
		var cnt_ok uint32
		for idx, inp := range tx.TxIn {
			if tx.MemInputs[idx] {
				txx := TransactionsToSend[btc.BIdx(inp.Input.Hash[:])]
				if _, ok := already_in[txx]; !ok {
					yes = true
					if is_any {
						return
					}
					res = append(res, txx)
				}

				cnt_ok++
				if cnt_ok == tx.MemInputCnt {
					return
				}
			}
		}
		return
	}

	var append_txs func(txkey *OneTxToSend)
	append_txs = func(t2s *OneTxToSend) {
		result = append(result, t2s)
		already_in[t2s] = true

		if toretry, ok := parent_of[t2s]; ok {
			for _, tx_ := range toretry {
				if _, in := already_in[tx_]; in {
					continue
				}
				if _, yes := missing_parents(tx_, true); !yes {
					append_txs(tx_)
				}
			}
			delete(parent_of, t2s)
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

	if CheckForErrors() && (len(result) != cap(result) || len(result) != len(already_in) || len(parent_of) != 0) {
		println("ERROR: Get sorted mempool cap:", cap(result), " result:", len(result), " alreadyin:", len(already_in), " parents:", len(parent_of))
	}

	return
}

// GetSortedMempool returns txs sorted by SPB, but with parents first.
// It uses the sort list for speedin up the process
// Make sure to call it with TxMutex locked
func GetSortedMempool() (result []*OneTxToSend) {
	if SortListDirty {
		return GetSortedMempoolSlow()
	}

	result = make([]*OneTxToSend, 0, len(TransactionsToSend))
	var prv_idx uint64
	for t2s := BestT2S; t2s != nil; t2s = t2s.worse {
		if CheckForErrors() && (prv_idx != 0 && prv_idx >= t2s.SortRank) {
			println("ERROR: GetSortedMempool corupt sort index", len(TransactionsToSend), prv_idx, t2s.SortRank)
		}
		prv_idx = t2s.SortRank
		result = append(result, t2s)
	}
	return
}
