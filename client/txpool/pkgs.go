package txpool

import (
	"bytes"
	"fmt"
	"slices"
	"sort"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
)

const (
	AUTO_FEE_PKGS_SUSPEND_AFTER = 10 * time.Minute // auto fee (re)packaging will turn itself off, if not needed for so long
)

var (
	FeePackages       []*OneTxsPackage = make([]*OneTxsPackage, 0, 10e3) // prealloc 10k records, which takes only 80KB of RAM but can save time later
	FeePackagesDirty  bool
	feePackagesReSort bool
)

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
	for _, par := range pk.Txs {
		if _, ok = list[par]; ok {
			return
		}
	}
	return
}

// If t2s belongs to any packages, we add the child at the end of each of them
// If t2s does not belong to any packages, we create a new 2-txs package for it and the child
func (parent *OneTxToSend) addToPackages(new_child *OneTxToSend) {
	if time.Since(LastSortingDone) > AUTO_FEE_PKGS_SUSPEND_AFTER {
		common.CountSafe("TxPkgsSusp-InAdd")
		FeePackagesDirty = true
		return
	}

	common.CountSafe("TxPkgsAdd")
	// Here we go through all the packages that the parent is in and append the new_child at their ends.
	// Mind that rebuild() modifies parent.inPackages, so we need to work on a copy of it.
	var has_own_pkg bool
	for _, pkg := range slices.Clone(parent.inPackages) {
		if pkg.Txs[0] == parent {
			has_own_pkg = true
		}
		if slices.Contains(pkg.Txs, new_child) {
			// this can happen when a new child uses more than one vout from the parent
			// ... or when the package has just been rebuilt for another parent of this child
			common.CountSafe("TxPkgsAddDupTx")
			continue
		}
		if pkg.hasAllTheParentsFor(new_child) {
			pkg.Txs = append(pkg.Txs, new_child)
			pkg.Weight += new_child.Weight()
			pkg.Fee += new_child.Fee
			new_child.inPackagesSet(append(new_child.inPackages, pkg))
			common.CountSafe("TxPkgsAddAppend")
		} else {
			// The child has also other unconfirmed parents, that are not in this package yet.
			// They need to be placed before the child, so build the package again.
			pkg.rebuild()
			common.CountSafe("TxPkgsAddRebuild")
		}
		feePackagesReSort = true
	}

	// The parent's own package is the one that starts with it. Mind that it may not have one yet, even if
	// it is in some packages already - when it got pulled into them as another parent of somebody's child.
	if !has_own_pkg {
		// we create a new package, like in buildListAndPackages()
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
		} else {
			fmt.Println("ERROR: in addToPackages parent's GetItWithAllChildren returned only", len(pandch), "txs")
		}
	}
}

// rebuild makes the package again, starting from its first tx - like buildListAndPackages() would do it.
func (pkg *OneTxsPackage) rebuild() {
	for _, t := range pkg.Txs {
		t.removePkg(pkg)
	}
	pkg.Txs = pkg.Txs[0].GetItWithAllChildren()
	pkg.Weight = 0
	pkg.Fee = 0
	for _, t := range pkg.Txs {
		pkg.Weight += t.Weight()
		pkg.Fee += t.Fee
		t.inPackagesSet(append(t.inPackages, pkg))
	}
}

// removes itself from any grup containing it
func (t2s *OneTxToSend) delFromPackages() {
	if time.Since(LastSortingDone) > AUTO_FEE_PKGS_SUSPEND_AFTER {
		common.CountSafe("TxPkgsSusp-InDel")
		FeePackagesDirty = true
		return
	}

	var records2remove int
	common.CountSafe("TxPkgsDel")

	for _, pkg := range t2s.inPackages {
		if CheckForErrors() && len(pkg.Txs) < 2 {
			fmt.Println("ERROR: delFromPackages called on t2s that has pkg with less than txs", pkg)
			FeePackagesDirty = true
			return
		}

		if pkg.Txs[0] == t2s {
			// This may only happen during block submission.
			// In such case, remove the entire package.
			for _, t := range pkg.Txs {
				if t != t2s {
					t.removePkg(pkg)
				}
			}
			common.CountSafe("TxPkgsDelGrP")
			pkg.Txs = nil
			records2remove++
		} else if len(pkg.Txs) == 2 {
			// Only two txs - remove reference to this pkg from the other tx that owned it.
			pkg.Txs[0].removePkg(pkg) // ... which must be on Txs[0], as we just checked we were not there.
			common.CountSafe("TxPkgsDelGrA")
			pkg.Txs = nil
			records2remove++
		} else {
			pandch := pkg.Txs[0].GetItWithAllChildren()
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
						fmt.Println("ERROR: delFromPackages -> GetItWithAllChildren returned us " + pkg.Txs[0].Hash.String())
						FeePackagesDirty = true
						return
					}
					pkg.Weight += t.Weight()
					pkg.Fee += t.Fee
					t.inPackagesSet(append(t.inPackages, pkg)) // now mark back the tx using our pkg
				}
				feePackagesReSort = true
				common.CountSafe("TxPkgsDelTx")
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
		// remove them in place (this also clears the pointers left at the end of the buffer)
		FeePackages = slices.DeleteFunc(FeePackages, func(pkg *OneTxsPackage) bool {
			return pkg.Txs == nil
		})
		feePackagesReSort = true
	}
}

// removes a reference to a given package from the t2s
func (t2s *OneTxToSend) removePkg(pkg *OneTxsPackage) {
	if CheckForErrors() && len(t2s.inPackages) == 0 {
		fmt.Println("ERROR: removePkg called on txs with no InPackages", t2s.Hash.String())
		return
	}
	if len(t2s.inPackages) == 1 {
		if CheckForErrors() && t2s.inPackages[0] != pkg {
			fmt.Println("ERROR: removePkg called on txs with one pkg, bot not the one")
		}
		t2s.inPackagesSet(nil)
	} else {
		if idx := slices.Index(t2s.inPackages, pkg); idx >= 0 {
			t2s.inPackagesSet(slices.Delete(t2s.inPackages, idx, idx+1))
		} else {
			fmt.Println("ERROR: removePkg cannot find the given pkg in t2s.InPackages", len(t2s.inPackages))
			return
		}
	}
}

// hasAllTheParentsFor returns true if all the unconfirmed parents of the child are in the package.
// The inputs that are already mined do not matter here.
func (pk *OneTxsPackage) hasAllTheParentsFor(child *OneTxToSend) bool {
	if len(pk.Txs)*len(child.TxIn) > 10 {
		m := make(map[btc.BIDX]bool, len(pk.Txs))
		for _, t := range pk.Txs {
			m[t.Hash.BIdx()] = true
		}
		for i, in := range child.TxIn {
			if i < len(child.MemInputs) && child.MemInputs[i] && !m[btc.BIdx(in.Input.Hash[:])] {
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
		for i, in := range child.TxIn {
			if i < len(child.MemInputs) && child.MemInputs[i] && !has_parent_for(in.Input.Hash[:]) {
				return false
			}
		}
	}
	return true
}

func sortFeePackages() {
	if feePackagesReSort {
		feePackagesReSort = false
		common.CountSafe("TxPkgsSortDo")
		sort.Slice(FeePackages, func(i, j int) bool {
			/* use this one if you want to have them sorted in a predictible order (useful for debugging)
			iv := FeePackages[i].Fee * uint64(FeePackages[j].Weight)
			jv := FeePackages[j].Fee * uint64(FeePackages[i].Weight)
			if iv != jv {
				return iv > jv
			}
			return binary.LittleEndian.Uint64(FeePackages[i].Txs[0].Hash.Hash[:8]) >
				binary.LittleEndian.Uint64(FeePackages[j].Txs[0].Hash.Hash[:8])
			*/
			return FeePackages[i].Fee*uint64(FeePackages[j].Weight) > FeePackages[j].Fee*uint64(FeePackages[i].Weight)
		})
	} else {
		common.CountSafe("TxPkgsSortSkip")
	}
}

func emptyFeePackages() {
	if len(FeePackages) > 0 {
		for i := range FeePackages {
			FeePackages[i] = nil // otherwise the old packages (and their txs) would stay in memory
		}
		FeePackages = FeePackages[:0] // to avoid unneeded memory allocation, just reuse the old buffer
		for _, t2s := range TransactionsToSend {
			t2s.inPackagesSet(nil) // we have to do this first run only to reset InPackages field
		}
		common.CountSafe("TxPkgsFreeMemory")
	}
}

// builds FeePackages list, if necessary
func buildListAndPackages() {
	defer sortFeePackages()
	buildSortedList()
	LastSortingDone = time.Now()
	if !FeePackagesDirty {
		common.CountSafe("TxPkgsHaveThem")
		return
	}
	common.CountSafe("TxPkgsNeedThem")
	emptyFeePackages()
	for t2s := BestT2S; t2s != nil; t2s = t2s.worse {
		if t2s.MemInputCnt > 0 {
			continue
		}

		if pandch := t2s.GetItWithAllChildren(); len(pandch) > 1 {
			pkg := &OneTxsPackage{Txs: pandch}
			for _, t := range pandch {
				pkg.Weight += t.Weight()
				pkg.Fee += t.Fee
				t.inPackagesSet(append(t.inPackages, pkg))
			}
			FeePackages = append(FeePackages, pkg)
		}
	}
	feePackagesReSort = true
	FeePackagesDirty = false
	RepackagingSinceLastRedoTime = 0
	RepackagingSinceLastRedoCount = 0
	RepackagingSinceLastRedoWhen = time.Now()
}

// GetSortedMempoolRBF is like GetSortedMempool(), but one uses Child-Pays-For-Parent algo.
func GetSortedMempoolRBF() (result []*OneTxToSend) {
	result, _ = getSortedMempoolRBF()
	return
}

// getSortedMempoolRBF does GetSortedMempoolRBF(), additionally returning the fee rate (in satoshis per
// 1000 virtual bytes) of the worst tx or package that still fits in the mempool's size limit.
func getSortedMempoolRBF() (result []*OneTxToSend, worst_spkb uint64) {
	var pks_idx int
	var worst_fee, worst_weight, cursize uint64
	result = make([]*OneTxToSend, 0, len(TransactionsToSend))
	already_in := make(map[*OneTxToSend]bool, len(TransactionsToSend))
	buildListAndPackages()
	maxsize := common.MaxMempoolSize()
	for tx := BestT2S; tx != nil; tx = tx.worse {
		for pks_idx < len(FeePackages) {
			if pk := FeePackages[pks_idx]; pk.Fee*uint64(tx.Weight()) > tx.Fee*uint64(pk.Weight) {
				pks_idx++
				if pk.anyIn(already_in) {
					continue
				}
				for _, _t := range pk.Txs {
					already_in[_t] = true
					cursize += uint64(_t.Footprint)
				}
				result = append(result, pk.Txs...)
				if cursize < maxsize {
					worst_fee = pk.Fee
					worst_weight = uint64(pk.Weight)
				}
				continue
			}
			break
		}

		if _, ok := already_in[tx]; !ok {
			cursize += uint64(tx.Footprint)
			result = append(result, tx)
			already_in[tx] = true
			if cursize < maxsize {
				worst_fee = tx.Fee
				worst_weight = uint64(tx.Weight())
			}
		}
	}
	if worst_weight > 0 {
		worst_spkb = 4000 * worst_fee / worst_weight
	}
	return
}

type MPFeeRec struct {
	Txs    []*OneTxToSend
	Weight uint64
	Fee    uint64
}

func GetMempoolFees(maxweight uint64) (result []*MPFeeRec) {
	var pks_idx int
	var weightsofar uint64
	result = make([]*MPFeeRec, 0, len(TransactionsToSend))
	already_in := make(map[*OneTxToSend]bool, len(TransactionsToSend))
	buildListAndPackages()
	for tx := BestT2S; tx != nil && weightsofar < maxweight; tx = tx.worse {
		for pks_idx < len(FeePackages) {
			pk := FeePackages[pks_idx]
			if pk.Fee*uint64(tx.Weight()) > tx.Fee*uint64(pk.Weight) {
				pks_idx++
				if pk.anyIn(already_in) {
					continue
				}
				rec := &MPFeeRec{Weight: uint64(pk.Weight), Fee: pk.Fee}
				rec.Txs = pk.Txs
				weightsofar += uint64(pk.Weight)
				for _, _t := range pk.Txs {
					already_in[_t] = true
				}
				result = append(result, rec)
				continue
			}
			break
		}
		if _, ok := already_in[tx]; !ok {
			wg := tx.Weight()
			rec := &MPFeeRec{Weight: uint64(wg), Fee: tx.Fee}
			rec.Txs = []*OneTxToSend{tx}
			result = append(result, rec)
			weightsofar += uint64(tx.Weight())
			already_in[tx] = true
		}
	}
	return
}
