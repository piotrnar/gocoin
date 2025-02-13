package txpool

import (
	"bytes"
	"fmt"
	"sort"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
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
		FeePackages = FeePackages[:0] // to avoid unneeded memory allocation, just reuse the old buffer
		for _, t2s := range TransactionsToSend {
			t2s.inPackagesSet(nil) // we have to do this first run only to reset InPackages field
		}
		common.CountSafe("TxPkgsFreeMemory")
	}
}

// builds FeePackages list, if neccessary
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
		CurrentFeeAdjustedSPKB = 4000 * worst_fee / worst_weight
	}
	return
}

// GetMempoolFees only takes tx/package weight and the fee.
func GetMempoolFees(maxweight uint64) (result [][2]uint64) {
	var pks_idx int
	var weightsofar uint64
	result = make([][2]uint64, 0, len(TransactionsToSend))
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
				result = append(result, [2]uint64{uint64(pk.Weight), pk.Fee})
				weightsofar += uint64(pk.Weight)
				for _, _t := range pk.Txs {
					already_in[_t] = true
				}
				continue
			}
			break
		}
		if _, ok := already_in[tx]; !ok {
			wg := tx.Weight()
			result = append(result, [2]uint64{uint64(wg), tx.Fee})
			weightsofar += uint64(tx.Weight())
			already_in[tx] = true
		}
	}
	return
}
