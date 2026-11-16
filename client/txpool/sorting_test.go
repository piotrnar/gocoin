package txpool

import (
	"bytes"
	"math/rand"
	"sort"
	"strings"
	"testing"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
)

// testRandomPool fills the mempool with a random mix of txs (parents, children with one or more parents),
// also removing some of them. Calls the given function after each change.
// The same seed always gives the same sequence of changes.
func testRandomPool(t *testing.T, seed int64, steps int, after func()) {
	type out struct {
		btc.TxPrevOut
		value uint64
		inmem bool
	}
	rnd := rand.New(rand.NewSource(seed))
	db := testSetup()

	// The mempool's size limit is not configured in the tests (it is zero), so removeExcessiveTxs() would
	// remove everything as soon as we get over 1MB. Holding this ticket keeps it off.
	GetMPInProgressTicket <- true
	defer func() { <-GetMPInProgressTicket }()

	// Without this limit most of the txs would end up in one huge cluster, and building the fee packages
	// for it takes ages.
	common.CFG.TXPool.MaxClusterTxs = 25
	defer func() { common.CFG.TXPool.MaxClusterTxs = 0 }()

	var avail []out
	for i := 1; i <= steps; i++ {
		avail = append(avail, out{TxPrevOut: btc.TxPrevOut{Hash: testAddUtxo(db, i, 1000000)}, value: 1000000})
	}
	for step := 0; step < steps; step++ {
		if step%10 == 9 && len(TransactionsToSend) > 0 {
			// remove a random tx (with its children)
			all := make([]*OneTxToSend, 0, len(TransactionsToSend))
			for _, t2s := range TransactionsToSend {
				all = append(all, t2s)
			}
			sort.Slice(all, func(i, j int) bool { // maps are not ordered, and we need the same sequence each time
				return bytes.Compare(all[i].Hash.Hash[:], all[j].Hash.Hash[:]) < 0
			})
			TxMutex.Lock()
			all[rnd.Intn(len(all))].Delete(true, 0)
			TxMutex.Unlock()
			// forget the outputs that cannot be spent anymore
			n := 0
			for _, o := range avail {
				if _, ok := TransactionsToSend[btc.BIdx(o.Hash[:])]; ok || !o.inmem {
					avail[n] = o
					n++
				}
			}
			avail = avail[:n]
		} else {
			var ins []btc.TxPrevOut
			var value uint64
			for cnt := 1 + rnd.Intn(3); cnt > 0 && len(avail) > 0; cnt-- {
				// often take one of the most recent outputs, to get chains of txs, otherwise any of them
				idx := rnd.Intn(len(avail))
				if rnd.Intn(2) == 0 && len(avail) > 8 {
					idx = len(avail) - 1 - rnd.Intn(8)
				}
				ins = append(ins, avail[idx].TxPrevOut)
				value += avail[idx].value
				avail = append(avail[:idx], avail[idx+1:]...)
			}
			value -= uint64(100 + rnd.Intn(20000)) // the fee
			tx := testTx(value/2, 2, ins...)
			if res := testSubmit(tx); res == TX_REJECTED_CLUSTER_BIG {
				continue // its inputs are just not used anymore
			} else if res != 0 {
				t.Fatal("random tx rejected:", ReasonToString(res), "- seed", seed, "step", step, "pool", len(TransactionsToSend))
			}
			avail = append(avail, out{TxPrevOut: btc.TxPrevOut{Hash: tx.Hash.Hash, Vout: 0}, value: value / 2, inmem: true},
				out{TxPrevOut: btc.TxPrevOut{Hash: tx.Hash.Hash, Vout: 1}, value: value / 2, inmem: true})
		}
		if after != nil {
			after()
		}
	}
}

// The fingers must not change where the txs are placed on the sorted list, only how fast we get there.
func TestSortFingers(t *testing.T) {
	defer func(v int) { sortFingerSpacing = v }(sortFingerSpacing)

	run := func(seed int64, spacing int) (order []string) {
		sortFingerSpacing = spacing
		testRandomPool(t, seed, 1500, func() {
			FeePackagesDirty = true // we do not need the packages here, so do not waste time on updating them
			if SortListDirty {
				t.Fatal("sorted list got dirty")
			}
		})

		var prev *OneTxToSend
		fingers := make(map[*OneTxToSend]bool)
		for t2s := BestT2S; t2s != nil; t2s = t2s.worse {
			if prev != nil && (t2s.SortRank <= prev.SortRank || t2s.better != prev) {
				t.Fatal("sorted list broken")
			}
			if t2s.sortFinger {
				fingers[t2s] = true
			}
			order = append(order, t2s.Hash.String())
			prev = t2s
		}
		if prev != WorstT2S || len(order) != len(TransactionsToSend) {
			t.Fatal("sorted list incomplete:", len(order), len(TransactionsToSend))
		}
		TxMutex.Lock()
		bad := VerifyMempoolSort(GetSortedMempool())
		TxMutex.Unlock()
		if bad {
			t.Fatal("child before its parent on the sorted list")
		}

		if len(fingers) != len(sortFingers) {
			t.Fatal("fingers do not match the flags:", len(fingers), len(sortFingers))
		}
		for i, f := range sortFingers {
			if !fingers[f] || f.MemInputCnt != 0 {
				t.Fatal("finger not on the list or it has unconfirmed inputs")
			}
			if i > 0 && (f.SortRank <= sortFingers[i-1].SortRank || isFirstTxBetter(f, sortFingers[i-1])) {
				t.Fatal("fingers not sorted")
			}
		}
		if spacing < 100 && len(sortFingers) < 10 {
			t.Fatal("only", len(sortFingers), "fingers created")
		}
		testMempoolOK(t)

		// building the list from scratch must give working fingers too
		TxMutex.Lock()
		SortListDirty = true
		buildSortedList()
		cnt := len(sortFingers)
		TxMutex.Unlock()
		if spacing < 100 && cnt < 10 {
			t.Fatal("only", cnt, "fingers after rebuilding")
		}
		return
	}

	for seed := int64(1); seed <= 3; seed++ {
		with := run(seed, 3)
		without := run(seed, 1<<30)
		if len(without) < 500 {
			t.Fatal("too few txs in the mempool:", len(without))
		}
		if strings.Join(with, ",") != strings.Join(without, ",") {
			t.Fatal("the order of txs with the fingers is different than without them, seed", seed)
		}
	}
}

// Fee packages updated on the fly must be the same as when built from scratch.
func TestPackagesUpdate(t *testing.T) {
	snapshot := func() map[string]string {
		res := make(map[string]string, len(FeePackages))
		for _, pkg := range FeePackages {
			if VerifyMempoolSort(pkg.Txs) {
				t.Fatal("child before its parent in a package")
			}
			ids := make([]string, 0, len(pkg.Txs))
			var fee uint64
			var weight int
			for _, t2s := range pkg.Txs {
				ids = append(ids, t2s.Hash.String()[:16])
				fee += t2s.Fee
				weight += t2s.Weight()
			}
			if fee != pkg.Fee || weight != pkg.Weight {
				t.Fatal("package's fee or weight is wrong")
			}
			sort.Strings(ids)
			res[pkg.Txs[0].Hash.String()] = strings.Join(ids, ",")
		}
		if len(res) != len(FeePackages) {
			t.Fatal("two packages starting with the same tx")
		}
		return res
	}

	var compared, bigger int
	testRandomPool(t, 7, 600, func() {

		TxMutex.Lock()
		defer TxMutex.Unlock()
		if FeePackagesDirty {
			// first call, or the packaging turned itself off as nobody was using it
			GetSortedMempoolRBF()
			return
		}
		if checkFeeList() {
			t.Fatal("fee packages inconsistent")
		}
		updated := snapshot()
		FeePackagesDirty = true
		GetSortedMempoolRBF() // this one builds them from scratch
		rebuilt := snapshot()
		if len(updated) != len(rebuilt) {
			t.Fatal("different number of packages:", len(updated), len(rebuilt))
		}
		for k, v := range rebuilt {
			if updated[k] != v {
				t.Fatal("package", k[:16], "differs\n updated:", updated[k], "\n rebuilt:", v)
			}
		}
		compared++
		for _, pkg := range FeePackages {
			if len(pkg.Txs) > 3 {
				bigger++
			}
		}
	})
	if compared < 400 || bigger == 0 {
		t.Fatal("the test did not check enough:", compared, bigger)
	}
	testMempoolOK(t)
}
