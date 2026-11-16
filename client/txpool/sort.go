package txpool

import (
	"fmt"
	"slices"
	"sort"
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
	SortingDisabled   bool
	LastSortingDone   time.Time

	// Fingers are pointers to every n-th tx (counting only those with no unconfirmed inputs) of the sorted list.
	// As such txs are always ordered by their fee rate, we can use a binary search to find the place from where
	// to start looking for the new tx's position, instead of walking through the list from its very top.
	sortFingers       []*OneTxToSend
	sortFingerSpacing int = 128

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

// call it with false to restore sorting
func BlockCommitInProgress(yes bool) {
	TxMutex.Lock()
	SortingDisabled = yes
	if !yes && FeePackagesDirty && SortListDirty {
		emptyFeePackages() // this should free all the memory used by packages
	}
	TxMutex.Unlock()
}

// insertes given tx into sorted list at its proper position
func (t2s *OneTxToSend) AddToSort() {
	if SortListDirty {
		return
	}
	if SortingDisabled {
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
			fmt.Println("ERROR: if WorstT2S is nil BestT2S should be nil too", WorstT2S, BestT2S)
			WorstT2S, BestT2S = nil, nil
		}
		t2s.SortRank = SORT_START_INDEX
		WorstT2S, BestT2S = t2s, t2s
		t2s.better, t2s.worse = nil, nil
		resetSortFingers()
		return
	}

	// the tx must be placed below all its parents
	start := BestT2S
	wpr := t2s.findWorstParent()
	if wpr != nil {
		start = wpr.worse
	}

	// Find the lowest finger that is not worse than the tx - it lets us skip everything above it,
	// as there cannot be anything worse than the tx up there.
	fidx := sort.Search(len(sortFingers), func(i int) bool {
		return isFirstTxBetter(t2s, sortFingers[i])
	})
	if fidx > 0 {
		if finger := sortFingers[fidx-1]; wpr == nil || finger.SortRank > wpr.SortRank {
			start = finger.worse
		}
	}

	if steps := t2s.insertDownFromHere(start); steps > 2*sortFingerSpacing && wpr == nil && t2s.MemInputCnt == 0 {
		// it was a long walk, so make this tx a new finger (it is located between fidx-1 and fidx)
		t2s.sortFinger = true
		sortFingers = slices.Insert(sortFingers, fidx, t2s)
	}
}

func resetSortFingers() {
	for i := range sortFingers {
		sortFingers[i].sortFinger = false
		sortFingers[i] = nil // do not keep the txs in memory
	}
	sortFingers = sortFingers[:0]
}

// call it after the sorted list has been built from scratch
func buildSortFingers() {
	var cnt int
	resetSortFingers()
	for t2s := BestT2S; t2s != nil; t2s = t2s.worse {
		t2s.sortFinger = false
		if t2s.MemInputCnt == 0 {
			if cnt++; cnt >= sortFingerSpacing {
				cnt = 0
				t2s.sortFinger = true
				sortFingers = append(sortFingers, t2s)
			}
		}
	}
}

// inserts the tx above the first tx that is worse, starting from the given one
// returns the number of the txs it had to go through
func (t2s *OneTxToSend) insertDownFromHere(wpr *OneTxToSend) (steps int) {
	for wpr != nil { // we have to do it from top-down
		if isFirstTxBetter(t2s, wpr) {
			t2s.insertBefore(wpr)
			return
		}
		wpr = wpr.worse
		steps++
	}
	// we reached the worst element - append it at the end
	WorstT2S.worse = t2s
	t2s.better = WorstT2S
	t2s.worse = nil
	t2s.SortRank = WorstT2S.SortRank + sortIndexStep
	WorstT2S = t2s
	return
}

// removes given tx from the sorted list
func (t2s *OneTxToSend) DelFromSort() {
	if SortListDirty {
		return
	}
	if SortingDisabled {
		SortListDirty = true
		return
	}
	sta := time.Now()
	defer func() {
		ResortingSinceLastRedoTime += time.Since(sta)
		ResortingSinceLastRedoCount++
	}()
	if t2s.sortFinger {
		t2s.sortFinger = false
		if idx := slices.Index(sortFingers, t2s); idx >= 0 {
			sortFingers = slices.Delete(sortFingers, idx, idx+1)
		}
	}
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
			fmt.Println("ERROR: t2s.Worse is nil but t2s was not WorstT2S", WorstT2S, BestT2S, t2s.worse)
			panic("This should not happen")
		}
		if t2s.worse.better != t2s {
			fmt.Println("ERROR: t2s.Worse.Better is not pointing to t2s", WorstT2S, BestT2S, t2s, t2s.worse, t2s.worse.better)
			panic("This should not happen")
		}
	}
	t2s.worse.better = t2s.better

	if CheckForErrors() {
		if t2s.better == nil {
			fmt.Println("ERROR: t2s.Better is nil but t2s was not BestT2S", WorstT2S, BestT2S, t2s.better)
			panic("This should not happen")
		}
		if t2s.better.worse != t2s {
			fmt.Println("ERROR: t2s.Better.Worse is not pointing to t2s", WorstT2S, BestT2S, t2s, t2s.better, t2s.better.worse)
			panic("This should not happen")
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
				fmt.Println("ERROR: not existing parent", btc.BIdxString(parent_bidx), "for", t2s.Hash.String())
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
		}
		return
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
	if SortingDisabled {
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
		resetSortFingers()
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
	buildSortFingers()
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
		fmt.Println("ERROR: Get sorted mempool cap:", cap(result), " result:", len(result), " alreadyin:", len(already_in), " parents:", len(parent_of))
		panic("This should not happen")
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
	for t2s := BestT2S; t2s != nil; t2s = t2s.worse {
		result = append(result, t2s)
	}
	return
}
