package txpool

import (
	"os"
	"reflect"
	"runtime/debug"
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
	Better, Worse                 *OneTxToSend
	Invsentcnt, SentCnt           uint32
	Firstseen, Lastseen, Lastsent time.Time
	Volume, Fee                   uint64
	*btc.Tx
	MemInputs   []bool // transaction is spending inputs from other unconfirmed tx(s)
	MemInputCnt int
	SigopsCost  uint64
	VerifyTime  time.Duration
	SortIndex   uint64
	Footprint   uint32
	Local       bool
	Blocked     byte // if non-zero, it gives you the reason why this tx nas not been routed
	Final       bool // if true RFB will not work on it
}

func (t2s *OneTxToSend) Add(bidx btc.BIDX) {
	TransactionsToSend[bidx] = t2s
	t2s.AddToSort()
	TransactionsToSendWeight += uint64(t2s.Weight())
	TransactionsToSendSize += uint64(t2s.Footprint)

	if !FeePackagesDirty && t2s.MemInputCnt > 0 {
		common.CountSafe("TxPkgsPlus")
		t2s.updateAllPackages()
	}

	if !SortingSupressed && !SortListDirty && removeExcessiveTxs() == 0 {
		common.SetMinFeePerKB(0) // nothing removed so set the minimal fee
	}
}

// Delete deletes the tx from the mempool.
// Deletes all the children as well if with_children is true.
// If reason is not zero, add the deleted txs to the rejected list.
// Make sure to call it with locked TxMutex.
func (tx *OneTxToSend) Delete(with_children bool, reason byte) {
	if common.Get(&common.CFG.TXPool.CheckErrors) {
		if _, ok := TransactionsToSend[tx.Hash.BIdx()]; !ok {
			println("ERROR: Trying to delete and already deleted tx", tx.Hash.String())
			debug.PrintStack()
			os.Exit(1)
		}
	}
	if !FeePackagesDirty && tx.MemInputCnt > 0 {
		tx.removeFromPackages() // remove it from FeePackages
	}

	TransactionsToSendSize -= uint64(tx.Footprint)

	if with_children {
		// remove all the children that are spending from tx
		var po btc.TxPrevOut
		po.Hash = tx.Hash.Hash
		for po.Vout = 0; po.Vout < uint32(len(tx.TxOut)); po.Vout++ {
			if so, ok := SpentOutputs[po.UIdx()]; ok {
				if child, ok := TransactionsToSend[so]; ok {
					child.Delete(true, reason)
				}
			}
		}
	}

	for _, txin := range tx.TxIn {
		delete(SpentOutputs, txin.Input.UIdx())
	}

	TransactionsToSendWeight -= uint64(tx.Weight())
	delete(TransactionsToSend, tx.Hash.BIdx())
	tx.DelFromSort()

	if reason != 0 {
		rejectTx(tx.Tx, reason, nil)
	}
}

func removeExcessiveTxs() (cnt int) {
	var worst_fee, worst_weight uint64
	if TransactionsToSendSize >= common.MaxMempoolSize()+1e6 { // only remove txs when we are 1MB over the maximum size
		sorted_txs := GetSortedMempoolRBF()
		for idx := len(sorted_txs) - 1; idx >= 0; idx-- {
			worst_tx := sorted_txs[idx]
			common.CountSafe("TxPurgedSizCnt")
			common.CountSafeAdd("TxPurgedSizBts", uint64(worst_tx.Footprint))
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
		newspkb := 4000 * worst_fee / worst_weight
		common.SetMinFeePerKB(newspkb)
	}
	return
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

func (t2s *OneTxToSend) getAllAncestors() (ancestors map[*OneTxToSend]bool) {
	ancestors = make(map[*OneTxToSend]bool)
	var add_ancestors func(t *OneTxToSend)
	add_ancestors = func(t *OneTxToSend) {
		for idx, inp := range t.Tx.TxIn {
			if t.MemInputs[idx] {
				if partx, ok := TransactionsToSend[btc.BIdx(inp.Input.Hash[:])]; ok {
					if len(partx.MemInputs) != 0 {
						add_ancestors(partx)
						ancestors[partx] = true
					}
				} else {
					println("ERROR: meminput missing for t2s", t.Hash.String(),
						"\n  <-  inp:", btc.NewUint256(inp.Input.Hash[:]).String())
					debug.PrintStack()
					os.Exit(1)
				}
			}
		}
	}
	add_ancestors(t2s)
	return
}

func (t2s *OneTxToSend) updateAllPackages() {
	if common.Get(&common.CFG.Net.MaxInCons) == 666 {
		FeePackagesDirty = true
	} else {
		var resort bool
		//ancestors := t2s.getAllAncestors()
		for _, pkg := range FeePackages {
			if idx := pkg.FindIn(t2s); idx != -1 {
				//if ancestors[pkg.Txs[0]] {
				pandch := t2s.GetItWithAllChildren()
				if common.Get(&common.CFG.TXPool.CheckErrors) {
					if len(pandch) < 2 {
						println("ERROR: GetItWithAllChildren returns only", len(pandch), "txs", t2s.Hash.String())
					}
					if len(pkg.Txs) == len(pandch) {
						println("ERROR: GetItWithAllChildren returns same len", len(pandch), t2s.Hash.String(),
							" identical:", reflect.DeepEqual(pkg.Txs, pandch))
					}
				}
				pkg.Txs = pandch
				pkg.Weight = 0
				pkg.Fee = 0
				for _, t := range pkg.Txs {
					pkg.Weight += t.Weight()
					pkg.Fee += t.Fee
				}
				resort = true
				common.CountSafe("TxPkgsUpdated")
			}
		}
		if resort {
			common.CountSafe("TxPkgsResortA")
			sortFeePackages()
		}
	}
}

// its looking for all the ancestors of the t2s and then removes itself from any group starting with them
func (t2s *OneTxToSend) removeFromPackages() {
	common.CountSafe("TxPkgsRemove")

	var records2remove int
	var resort bool

	ancestors := t2s.getAllAncestors()
	if len(ancestors) == 0 {
		common.CountSafe("TxPkgsRemove**Emp")
		return
	}

	for _, pkg := range FeePackages {
		if idx := pkg.FindIn(t2s); idx != -1 {
			println("RFP:", t2s.Hash.String(), "found @", idx+1, "/", len(pkg.Txs), ancestors[pkg.Txs[0]], len(ancestors))
			if len(pkg.Txs) == 2 {
				pkg.Txs = nil
				records2remove++
			} else {
				common.CountSafe("TxPkgsRemoveTx")
				pkg.Fee -= t2s.Fee
				pkg.Weight -= t2s.Weight()
				copy(pkg.Txs[idx:], pkg.Txs[idx+1:])
				pkg.Txs = pkg.Txs[:len(pkg.Txs)-1]
				resort = true
			}
		}
	}

	if records2remove > 0 {
		common.CountSafeAdd("TxPkgsRemoveGr", uint64(records2remove))
		new_pkgs_list := make([]*OneTxsPackage, 0, len(FeePackages)-records2remove)
		for _, pkg := range FeePackages {
			if pkg.Txs != nil {
				new_pkgs_list = append(new_pkgs_list, pkg)
			}
		}
		FeePackages = new_pkgs_list
		resort = true
	}

	if resort {
		common.CountSafe("TxPkgsResortD")
		sortFeePackages()
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
