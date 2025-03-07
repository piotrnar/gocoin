package txpool

import (
	"os"
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

	RepackagingSinceLastRedoTime  time.Duration
	RepackagingSinceLastRedoCount uint
	RepackagingSinceLastRedoWhen  time.Time

	ResortingSinceLastRedoTime  time.Duration
	ResortingSinceLastRedoCount uint
	ResortingSinceLastRedoWhen  time.Time
)

type OneTxToSend struct {
	Firstseen, Lastseen, Lastsent time.Time
	*btc.Tx
	better, worse       *OneTxToSend
	MemInputs           []bool           // only use memInputsSet() to set this field
	inPackages          []*OneTxsPackage // only use inPackagesSet() to set this field
	Volume, Fee         uint64
	SigopsCost          uint64
	SortRank            uint64
	VerifyTime          time.Duration
	Invsentcnt, SentCnt uint32
	MemInputCnt         uint32
	Footprint           uint32
	Blocked             byte // if non-zero, it gives you the reason why this tx has not been routed
	Local               bool
	Final               bool // if true RFB will not work on it
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

	if FeePackagesDirty {
		return
	}

	if CheckForErrors() && t2s.inPackages != nil {
		println("ERROR: Add to mempool called for tx that already has InPackages", len(t2s.inPackages))
		FeePackagesDirty = true
		return
	}

	// here we know that FeePackagesDirty is false
	if t2s.MemInputCnt > 0 { // go through all the parents...
		sta := time.Now()
		parents := t2s.getAllTopParents()
		for _, parent := range parents {
			if parent.MemInputCnt != 0 {
				println("ERROR: parent.MemInputCnt!=0 must not happen here")
				continue
			}
			parent.addToPackages(t2s)
		}
		RepackagingSinceLastRedoTime += time.Since(sta)
		RepackagingSinceLastRedoCount++
	}
}

func (tx *OneTxToSend) getAllTopParents() (result []*OneTxToSend) {
	result = make([]*OneTxToSend, 0, 16)
	already_in := make(map[*OneTxToSend]struct{}, 16)
	already_checked := make(map[btc.BIDX]struct{}, 16)
	var do_one_parent func(*OneTxToSend)
	do_one_parent = func(t2s *OneTxToSend) {
		for vout, meminput := range t2s.MemInputs {
			if meminput {
				bidx := btc.BIdx(t2s.TxIn[vout].Input.Hash[:])
				if _, ok := already_checked[bidx]; ok {
					continue
				}
				already_checked[bidx] = struct{}{}
				if parent, has := TransactionsToSend[bidx]; has {
					if parent.MemInputCnt == 0 {
						if _, ok := already_in[parent]; !ok {
							already_in[parent] = struct{}{}
							result = append(result, parent)
						}
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
	if CheckForErrors() {
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
	}

	for _, txin := range tx.TxIn {
		uidx := txin.Input.UIdx()
		delete(SpentOutputs, uidx)
		// Mind that we do not want to check RejectedUsedUTXOs and remove rejected txs
		// ... refering to these iputs. We will do it only later, if this tx is mined.
	}

	delete(TransactionsToSend, tx.Hash.BIdx())

	if !FeePackagesDirty && len(tx.inPackages) != 0 {
		sta := time.Now()
		tx.delFromPackages() // remove it from FeePackages
		RepackagingSinceLastRedoTime += time.Since(sta)
		RepackagingSinceLastRedoCount++
		tx.inPackagesSet(nil) // this one will update tx.Footprint
	}

	tx.DelFromSort()

	TransactionsToSendWeight -= uint64(tx.Weight())
	TransactionsToSendSize -= uint64(tx.Footprint)
	tx.Footprint = 0 // to track if something tries to modify it later

	if reason != 0 {
		rejectTx(tx.Tx, reason, nil)
	}
}

func removeExcessiveTxs() {
	if len(GetMPInProgressTicket) != 0 {
		return // don't do it during mpget
	}
	var cnt, bytes uint64
	if TransactionsToSendSize >= common.MaxMempoolSize()+1e6 { // only remove txs when we are 1MB over the maximum size
		sorted_txs := GetSortedMempoolRBF()
		FeePackagesDirty = true // do not update fee packages while doing this, as it will take forever
		for idx := len(sorted_txs) - 1; idx >= 0; idx-- {
			worst_tx := sorted_txs[idx]
			cnt++
			bytes += uint64(worst_tx.Footprint)
			worst_tx.Delete(false, 0)
			cnt++
			if TransactionsToSendSize <= common.MaxMempoolSize() {
				break
			}
		}
	}
	if cnt > 0 {
		common.CountSafeAdd("TxPurgedSizCnt", cnt)
		common.CountSafeAdd("TxPurgedSizBts", bytes)
		common.SetMinFeePerKB(CurrentFeeAdjustedSPKB)
		lastFeeAdjustedTime = time.Now()
	}
}

func txChecker(tx *btc.Tx) bool {
	bidx := tx.Hash.BIdx()
	TxMutex.Lock()
	rec, ok := TransactionsToSend[bidx]
	defer TxMutex.Unlock()
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
		common.CountSafe("TxScrBoosted-T2S")
	} else {
		if txr, ok := TransactionsRejected[bidx]; ok {
			// Replaced transaction also had their scripts verified OK
			if txr.Reason == TX_REJECTED_REPLACED {
				common.CountSafe("TxScrBoosted-TxR")
			} else {
				common.CountSafePar("TxScrMissed-", txr.Reason)
			}
		} else {
			common.CountSafe("TxScrMissed-???")
		}
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
	already_included := make(map[*OneTxToSend]struct{}, 256)

	result = make([]*OneTxToSend, 1, 256)
	result[0] = tx // our starting (parent) tx shall be the first element of the result
	already_included[tx] = struct{}{}

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
						already_included[prnt] = struct{}{}
					}
				}

				// now we can safely insert the child, as all its parent shall be already included
				result = append(result, ch)
				// ... and mark it as included, for later
				already_included[ch] = struct{}{}
			}
		}
	}
	return
}

// GetAllChildren gets all the children (and all of their children...) of the tx.
// The result is sorted by the oldest parent.
func (tx *OneTxToSend) GetAllChildren() (result []*OneTxToSend) {
	already_included := make(map[*OneTxToSend]struct{})
	var idx int
	par := tx
	for {
		chlds := par.GetChildren()
		for _, ch := range chlds {
			if _, ok := already_included[ch]; !ok {
				already_included[ch] = struct{}{}
				result = append(result, ch)
			}
		}
		if idx == len(result) {
			break
		}

		par = result[idx]
		already_included[par] = struct{}{} // TODO: this line is probably not needed
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

	emptyFeePackages()
	InitTransactionsToSend()
	InitTransactionsRejected()

	BestT2S, WorstT2S = nil, nil
	adjustSortIndexStep()
	SortListDirty = false
	ResortingSinceLastRedoTime = 0
	ResortingSinceLastRedoCount = 0
	ResortingSinceLastRedoWhen = time.Now()

	RepackagingSinceLastRedoTime = 0
	RepackagingSinceLastRedoCount = 0
	RepackagingSinceLastRedoWhen = time.Now()
	FeePackagesDirty = false

	TxMutex.Unlock()
}

func init() {
	chain.TrustedTxChecker = txChecker
}
