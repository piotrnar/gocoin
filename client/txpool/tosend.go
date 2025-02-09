package txpool

import (
	"os"
	"runtime/debug"
	"slices"
	"sync"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
)

var (
	TxMutex sync.Mutex

	// The actual memory pool:
	TransactionsToSend       map[btc.BIDX]*OneTxToSend
	TransactionsToSendSize   uint64
	TransactionsToSendWeight uint64

	// All the outputs that are currently spent in TransactionsToSend:
	// Each record is indexed by 64-bit-coded(TxID:Vout) and points to list of txs (from T2S)
	SpentOutputs map[uint64]btc.BIDX

	// Transactions that are received from network (via "tx"), but not yet processed:
	TransactionsPending map[btc.BIDX]bool = make(map[btc.BIDX]bool)
)

type OneTxToSend struct {
	better, worse                 *OneTxToSend
	inPackages                    []*OneTxsPackage // only use inPackagesSet() to set this field
	Invsentcnt, SentCnt           uint32
	Firstseen, Lastseen, Lastsent time.Time
	Volume, Fee                   uint64
	*btc.Tx
	MemInputs   []bool // only use memInputsSet() to set this field
	SigopsCost  uint64
	VerifyTime  time.Duration
	SortRank    uint64
	MemInputCnt uint32
	Footprint   uint32
	Local       bool
	Blocked     byte // if non-zero, it gives you the reason why this tx has not been routed
	Final       bool // if true RFB will not work on it
}

func (t2s *OneTxToSend) Add(bidx btc.BIDX) {
	for _, inp := range t2s.TxIn {
		SpentOutputs[inp.Input.UIdx()] = bidx
	}
	t2s.Footprint = uint32(t2s.SysSize())
	TransactionsToSend[bidx] = t2s
	TransactionsToSendWeight += uint64(t2s.Weight())
	TransactionsToSendSize += uint64(t2s.Footprint)
	t2s.AddToSort()

	if !FeePackagesDirty && CheckForErrors() {
		if t2s.inPackages != nil {
			println("ERROR: Add to mempool called for tx that already has InPackages", len(t2s.inPackages))
			FeePackagesDirty = true
			return
		}
		if !t2s.HasNoChildren() {
			println("ERROR: Add to mempool called for tx that already has some children", t2s.Hash.String())
			FeePackagesDirty = true
			return
		}
	}

	if !FeePackagesDirty && t2s.MemInputCnt > 0 { // go through all the parents...
		parents := t2s.getAllTopParents()
		for _, parent := range parents {
			if parent.MemInputCnt != 0 {
				println("ERROR: parent.MemInputCnt!=0 must not happen here")
				continue
			}
			parent.addToPackages(t2s)
		}
	}

	removeExcessiveTxs()
}

func (tx *OneTxToSend) getAllTopParents() (result []*OneTxToSend) {
	var do_one_parent func(t2s *OneTxToSend)
	do_one_parent = func(t2s *OneTxToSend) {
		for vout, meminput := range t2s.MemInputs {
			if meminput { // and add yoursef to their packages
				if parent, has := TransactionsToSend[btc.BIdx(t2s.TxIn[vout].Input.Hash[:])]; has {
					if parent.MemInputCnt == 0 {
						result = append(result, parent)
					} else {
						do_one_parent(parent)
					}
				} else {
					println("ERROR: getAllTopParents t2s being added has mem input which does not exist")
				}
			}
		}
	}
	do_one_parent(tx)
	return
}

// Delete deletes the tx from the mempool.
// Deletes all the children as well if with_children is true.
// If reason is not zero, add the deleted txs to the rejected list.
// Make sure to call it with locked TxMutex.
func (tx *OneTxToSend) Delete(with_children bool, reason byte) {
	if common.Get(&common.CFG.TXPool.CheckErrors) {
		if _, ok := TransactionsToSend[tx.Hash.BIdx()]; !ok {
			println("ERROR: Trying to delete already deleted tx", tx.Hash.String())
			debug.PrintStack()
			os.Exit(1)
		}
	}

	if with_children {
		// remove all the children that are spending from tx
		for vout := range tx.TxOut {
			uidx := btc.UIdx(tx.Hash.Hash[:], uint32(vout))
			if so, ok := SpentOutputs[uidx]; ok {
				if child, ok := TransactionsToSend[so]; ok {
					child.Delete(true, reason)
				}
			}
		}
	} else if CheckForErrors() {
		for vout := range tx.TxOut {
			uidx := btc.UIdx(tx.Hash.Hash[:], uint32(vout))
			if so, ok := SpentOutputs[uidx]; ok {
				child_missing := ""
				if child, ok := TransactionsToSend[so]; ok {
					// extra check to exclude children that use just-mined inputs
					if child.MemInputCnt > 0 {
						for vo, ti := range child.TxIn {
							if ti.Input.UIdx() == uidx {
								if child.MemInputs[vo] {
									child_missing = child.Id()
									break
								}
							}
						}
					}
				} else {
					println("ERROR: deleting t2s", tx.Id(), "but we have its spent_out that points nowhere!!")
				}
				if child_missing != "" {
					println("ERROR: deleting t2s", tx.Id(), "but we still have it's child:", child_missing)
				}
			}
		}
	}

	for _, txin := range tx.TxIn {
		uidx := txin.Input.UIdx()
		delete(SpentOutputs, uidx)

		//  remove data of any rejected txs that use this input
		if lst, ok := RejectedUsedUTXOs[uidx]; ok {
			for _, bidx := range lst {
				if txr, ok := TransactionsRejected[bidx]; ok {
					common.CountSafePar("TxPurgeRjctUTXO-", txr.Reason)
					DeleteRejectedByTxr(txr)
				} else if CheckForErrors() {
					println("ERROR: txr marked for removal but not present in TransactionsRejected")
				}
			}
			delete(RejectedUsedUTXOs, uidx) // this record will not be needed anymore
		}
	}

	delete(TransactionsToSend, tx.Hash.BIdx())

	if !FeePackagesDirty {
		tx.delFromPackages() // remove it from FeePackages
	}
	tx.inPackagesSet(nil) // this one will update tx.Footprint

	tx.DelFromSort()

	TransactionsToSendWeight -= uint64(tx.Weight())
	TransactionsToSendSize -= uint64(tx.Footprint)
	tx.Footprint = 0 // to track if something tries to modify it later

	if reason != 0 {
		rejectTx(tx.Tx, reason, nil)
	}
}

func removeExcessiveTxs() {
	var worst_fee, worst_weight uint64
	var cnt, bytes uint64
	if TransactionsToSendSize >= common.MaxMempoolSize()+1e6 { // only remove txs when we are 1MB over the maximum size
		sorted_txs := GetSortedMempoolRBF()
		for idx := len(sorted_txs) - 1; idx >= 0; idx-- {
			worst_tx := sorted_txs[idx]
			cnt++
			bytes += uint64(worst_tx.Footprint)
			worst_fee = worst_tx.Fee // we do not do the division here, as it may be more expensive
			worst_weight = uint64(worst_tx.Weight())
			worst_tx.Delete(true, 0)
			cnt++
			if TransactionsToSendSize <= common.MaxMempoolSize() {
				break
			}
		}
	}
	if cnt > 0 {
		common.CountSafeAdd("TxPurgedSizCnt", cnt)
		common.CountSafeAdd("TxPurgedSizBts", bytes)
		currentFeeAdjustedSPKB = 4000 * worst_fee / worst_weight
		common.SetMinFeePerKB(currentFeeAdjustedSPKB)
		feeAdjustDecrementSPKB = currentFeeAdjustedSPKB / 20
		lastFeeAdjustedTime = time.Now()
	}
}

func txChecker(tx *btc.Tx) bool {
	TxMutex.Lock()
	rec, ok := TransactionsToSend[tx.Hash.BIdx()]
	TxMutex.Unlock()
	if ok && rec.Local {
		common.CountSafe("TxScrOwn")
		return false // Assume own txs as non-trusted
	}
	if ok {
		ok = tx.WTxID().Equal(rec.WTxID())
		if !ok {
			//println("wTXID mismatch at", tx.Hash.String(), tx.WTxID().String(), rec.WTxID().String())
			common.CountSafe("TxScrSWErr")
		}
	}
	if ok {
		common.CountSafe("TxScrBoosted")
	} else {
		common.CountSafe("TxScrMissed")
	}
	return ok
}

// GetChildren gets all first level children of the tx.
func (tx *OneTxToSend) HasNoChildren() bool {
	for vout := range tx.TxOut {
		uidx := btc.UIdx(tx.Hash.Hash[:], uint32(vout))
		if _, ok := SpentOutputs[uidx]; ok {
			return false
		}
	}
	return true
}

// GetChildren gets all first level children of the tx.
func (tx *OneTxToSend) GetChildren() (result []*OneTxToSend) {
	res := make(map[*OneTxToSend]bool)
	for vout := range tx.TxOut {
		uidx := btc.UIdx(tx.Hash.Hash[:], uint32(vout))
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
		already_included[par] = true // TODO: this line is probably not needed
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

func (tx *OneTxToSend) Id() string {
	return tx.Hash.String()
}

// If t2s belongs to any packages, we add the child at the end of each of them
// If t2s does not belong to any packages, we create a new 2-txs package for it and the child
func (parent *OneTxToSend) addToPackages(new_child *OneTxToSend) {
	common.CountSafe("TxPkgsAdd")

	if len(parent.inPackages) == 0 {
		// we create a new package, like in lookForPackages()
		if pandch := parent.GetItWithAllChildren(); len(pandch) > 1 {
			pkg := &OneTxsPackage{Txs: pandch}
			for _, t := range pandch {
				pkg.Weight += t.Weight()
				pkg.Fee += t.Fee
				t.inPackagesSet(append(t.inPackages, pkg))
			}
			FeePackages = append(FeePackages, pkg)
			feePackagesReSort = true
			common.CountSafe("TxPkgsAddNew")
			if CheckForErrors() {
				pkg.checkForDups()
			}
		} else {
			println("ERROR: in addToPackages parent's GetItWithAllChildren returned only", len(pandch), "txs")
		}
	} else {
		// here we go through all the packages and append the new_child at their ends
		for _, pkg := range parent.inPackages {
			if !pkg.hasAllTheParentsFor(new_child) {
				// this is not out package
				continue
			}
			if slices.Contains(pkg.Txs, new_child) {
				// this can happen when a new child uses more tha one vout from the parent
				common.CountSafe("TxPkgsAddDupTx")
				continue
			}
			pkg.Txs = append(pkg.Txs, new_child)
			pkg.Weight += new_child.Weight()
			pkg.Fee += new_child.Fee
			new_child.inPackagesSet(append(new_child.inPackages, pkg))
			feePackagesReSort = true
			common.CountSafe("TxPkgsAddAppend")
			if CheckForErrors() {
				pkg.checkForDups()
			}
		}
	}
}

// removes itself from any grup containing it
func (t2s *OneTxToSend) delFromPackages() {
	common.CountSafe("TxPkgsDel")

	var records2remove int
	if len(t2s.inPackages) == 0 {
		return
	}

	for _, pkg := range t2s.inPackages {
		common.CountSafe("TxPkgsDelTick")
		if CheckForErrors() && len(pkg.Txs) < 2 {
			println("ERROR: delFromPackages called on t2s that has pkg with less than txs", pkg)
			FeePackagesDirty = true
			return
		}

		if len(pkg.Txs) == 2 {
			// remove reference to this pkg from the other txs that owned it
			if pkg.Txs[0] == t2s {
				pkg.Txs[1].removePkg(pkg)
				// TODO: check if it prints
				println("ERROR: delFromPackages our tx if first on the two list!")
			} else {
				pkg.Txs[0].removePkg(pkg)
			}
			pkg.Txs = nil
			records2remove++
			common.CountSafe("TxPkgsDelGrA")
		} else {
			common.CountSafe("TxPkgsDelTx")
			pandch := pkg.Txs[0].GetItWithAllChildren()
			/*not quite sure why this is happening, but seem to be a normal case so just ignore it
			if len(pandch) >= len(pkg.Txs) {
				println("ERROR: delFromPackages -> GetItWithAllChildren returned cnt", len(pandch), pkg.Txs)
			}*/
			// first unmark all txs using this pkg (we may mark them back later)
			for _, t := range pkg.Txs {
				if t != t2s {
					t.removePkg(pkg)
				}
			}
			if len(pandch) > 1 {
				pkg.Txs = pandch
				pkg.Weight = 0
				pkg.Fee = 0
				for _, t := range pandch {
					if CheckForErrors() && t == t2s {
						println("ERROR: delFromPackages -> GetItWithAllChildren returned us")
						FeePackagesDirty = true
						return
					}
					pkg.Weight += t.Weight()
					pkg.Fee += t.Fee
					t.inPackagesSet(append(t.inPackages, pkg)) // now mark back the tx using our pkg
				}
				feePackagesReSort = true
				common.CountSafe("TxPkgsDelTx")
				if CheckForErrors() {
					pkg.checkForDups()
				}
			} else {
				pkg.Txs = nil
				records2remove++
				common.CountSafe("TxPkgsDelGrB")
			}
			feePackagesReSort = true
		}
	}

	if records2remove > 0 {
		common.CountSafe("TxPkgsDelGrCnt")
		common.CountSafeAdd("TxPkgsDelGroup", uint64(records2remove))
		new_pkgs_list := make([]*OneTxsPackage, 0, cap(FeePackages))
		for _, pkg := range FeePackages {
			if pkg.Txs != nil {
				new_pkgs_list = append(new_pkgs_list, pkg)
			}
		}
		FeePackages = new_pkgs_list
		feePackagesReSort = true
	}
}

// removes a reference to a given package from the t2s
func (t2s *OneTxToSend) removePkg(pkg *OneTxsPackage) {
	if common.Get(&common.CFG.TXPool.CheckErrors) && len(t2s.inPackages) == 0 {
		println("ERROR: removePkg called on txs with no InPackages", t2s.Hash.String())
		return
	}
	if len(t2s.inPackages) == 1 {
		if common.Get(&common.CFG.TXPool.CheckErrors) && t2s.inPackages[0] != pkg {
			println("ERROR: removePkg called on txs with one pkg, bot not the one")
		}
		t2s.inPackagesSet(nil)
	} else {
		if idx := slices.Index(t2s.inPackages, pkg); idx >= 0 {
			t2s.inPackagesSet(slices.Delete(t2s.inPackages, idx, idx+1))
		} else if common.Get(&common.CFG.TXPool.CheckErrors) {
			println("ERROR: removePkg cannot find the given pkg in t2s.InPackages", len(t2s.inPackages))
			return
		}
	}
}

func (tx *OneTxToSend) SPW() float64 {
	return float64(tx.Fee) / float64(tx.Weight())
}

func (tx *OneTxToSend) SPB() float64 {
	return tx.SPW() * 4.0
}

func InitTransactionsToSend() {
	TransactionsToSend = make(map[btc.BIDX]*OneTxToSend)
	TransactionsToSendSize = 0
	TransactionsToSendWeight = 0
	SpentOutputs = make(map[uint64]btc.BIDX, 10e3)
}

func InitMempool() {
	TxMutex.Lock()
	InitTransactionsToSend()
	InitTransactionsRejected()
	TxMutex.Unlock()
}

func init() {
	chain.TrustedTxChecker = txChecker
}
