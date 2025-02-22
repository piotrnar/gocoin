package txpool

import (
	"bytes"
	"fmt"
	"runtime"
	"slices"
	"sync/atomic"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
)

var (
	// Transactions that we downloaded, but rejected:
	TransactionsRejected     map[btc.BIDX]*OneTxRejected = make(map[btc.BIDX]*OneTxRejected)
	TransactionsRejectedSize uint64                      // only include those that have *Tx pointer set

	TRIdxArray []btc.BIDX
	TRIdxHead  int
	TRIdxTail  int

	// Transactions that are waiting for inputs:
	// Each record points to a list of transactions that are waiting for the transaction from the index of the map
	// This way when a new tx is received, we can quickly find all the txs that have been waiting for it
	WaitingForInputs     map[btc.BIDX]*OneWaitingList = make(map[btc.BIDX]*OneWaitingList)
	WaitingForInputsSize uint64

	// Inputs that are being used by TransactionsRejected
	// Each record points to one TransactionsRejected with Reason of 200 or more
	RejectedUsedUTXOs map[uint64][]btc.BIDX = make(map[uint64][]btc.BIDX)
)

type OneTxRejected struct {
	Id       btc.Uint256
	Waiting4 *btc.Uint256
	*btc.Tx
	time.Time
	Size      uint32
	Footprint uint32
	ArrIndex  uint16
	Reason    byte
}

type OneWaitingList struct {
	TxID *btc.Uint256
	Ids  []btc.BIDX // List of pending tx ids
}

const (
	TX_REJECTED_DISABLED    = 1 // Only used for transactions in TransactionsToSend for Blocked field
	TX_REJECTED_NOT_PENDING = 2

	TX_REJECTED_TOO_BIG      = 101
	TX_REJECTED_FORMAT       = 102
	TX_REJECTED_LEN_MISMATCH = 103
	TX_REJECTED_EMPTY_INPUT  = 104

	TX_REJECTED_OVERSPEND   = 154
	TX_REJECTED_BAD_INPUT   = 157
	TX_REJECTED_SCRIPT_FAIL = 158

	TX_REJECTED_DATA_PURGED = 200

	// Anything from the list below might eventually get mined
	TX_REJECTED_NO_TXOU     = 202
	TX_REJECTED_BAD_PARENT  = 203
	TX_REJECTED_LOW_FEE     = 205
	TX_REJECTED_NOT_MINED   = 208
	TX_REJECTED_CB_INMATURE = 209
	TX_REJECTED_RBF_LOWFEE  = 210
	TX_REJECTED_RBF_FINAL   = 211
	TX_REJECTED_RBF_100     = 212
	TX_REJECTED_REPLACED    = 213
)

func TRIdxNext(idx int) int {
	if idx == len(TRIdxArray)-1 {
		return 0
	}
	return idx + 1
}

func TRIdxPrev(idx int) int {
	if idx == 0 {
		return len(TRIdxArray) - 1
	}
	return idx - 1
}

func TRIdZeroArrayRec(idx int) {
	for i := range TRIdxArray[0] {
		TRIdxArray[idx][i] = 0
	}
}

func TRIdIsZeroArrayRec(idx int) bool {
	for i := range TRIdxArray[0] {
		if TRIdxArray[idx][i] != 0 {
			return false
		}
	}
	return true
}

// Make sure to call it with locked TxMutex.
func AddRejectedTx(txr *OneTxRejected) {
	bidx := txr.Id.BIdx()
	if CheckForErrors() {
		if _, ok := TransactionsRejected[bidx]; ok {
			println("ERROR: AddRejectedTx: TxR", txr.Id.String(), "is already on the list")
			return
		}
	}
	txr.ArrIndex = uint16(TRIdxHead)
	TRIdxArray[TRIdxHead] = bidx
	TransactionsRejected[bidx] = txr
	TRIdxHead = TRIdxNext(TRIdxHead)
	if TRIdxHead == TRIdxTail {
		// we're touching the tail
		if !TRIdIsZeroArrayRec(TRIdxTail) { // remove the oldest record
			if txr, ok := TransactionsRejected[TRIdxArray[TRIdxTail]]; ok {
				if int(txr.ArrIndex) != TRIdxTail {
					println("ERROR: txr.ArrIndex != TRIdxTail", int(txr.ArrIndex), TRIdxTail)
				}
				DeleteRejectedByTxr(txr) // this should zero the record and advance the tail to the 1st non-empty slot
			} else {
				panic(fmt.Sprint("TRIdxArray[", TRIdxTail, "] not found in TransactionsRejected"))
			}
		} else {
			for {
				TRIdxTail = TRIdxNext(TRIdxTail) // advance the tail to the 1st non-empty slot
				if !TRIdIsZeroArrayRec(TRIdxTail) {
					break
				}
			}
		}
	}
	if txr.Tx != nil {
		for _, inp := range txr.TxIn {
			uidx := inp.Input.UIdx()
			RejectedUsedUTXOs[uidx] = append(RejectedUsedUTXOs[uidx], bidx)
		}
		if txr.Waiting4 != nil {
			var rec *OneWaitingList
			if rec = WaitingForInputs[txr.Waiting4.BIdx()]; rec == nil {
				rec = new(OneWaitingList)
				rec.TxID = txr.Waiting4
			}
			rec.Ids = append(rec.Ids, txr.Id.BIdx())
			WaitingForInputs[txr.Waiting4.BIdx()] = rec
			WaitingForInputsSize += uint64(txr.Footprint)
		}
	}
	TransactionsRejectedSize += uint64(txr.Footprint)
	verTxrCnt()
	limitRejectedSizeIfNeeded()
}

// Make sure to call it with locked TxMutex
func DeleteRejectedByTxr(txr *OneTxRejected) {
	/*if ns := uint32(txr.SysSize()); txr.Footprint != ns {
		println("Footprint of txr", txr.Id.String(), "has been fucked up:", txr.Footprint, "=>", ns)
	}*/
	common.CountSafePar("TxRejectedDel-", txr.Reason)
	TransactionsRejectedSize -= uint64(txr.Footprint)
	if txr.Tx != nil {
		txr.cleanup()
	}
	TRIdZeroArrayRec(int(txr.ArrIndex))
	if TRIdxTail == int(txr.ArrIndex) {
		for { // advance tail to the nearest non-zero index or to the head
			TRIdxTail = TRIdxNext(TRIdxTail)
			if TRIdxTail == TRIdxHead || !TRIdIsZeroArrayRec(TRIdxTail) {
				break
			}
		}

	}
	deleteTransactionsRejected(txr.Id.BIdx())
}

// TODO: get rid of it after debugging finished
func deleteTransactionsRejected(bidx btc.BIDX) {
	txr := TransactionsRejected[bidx]
	if txr == nil {
		panic("trying to remove not existing TransactionsRejected")
	}
	if txr.Waiting4 != nil {
		panic("trying to remove TransactionsRejected that still has Waiting4")
	}
	delete(TransactionsRejected, bidx)
}

// Make sure to call it with locked TxMutex
func DeleteRejectedByIdx(bidx btc.BIDX, musthave bool) {
	if txr, ok := TransactionsRejected[bidx]; ok {
		DeleteRejectedByTxr(txr)
	} else if musthave {
		panic("DeleteRejectedByIdx " + btc.BIdxString(bidx) + " not found in TransactionsRejected")
	}
}

// Remove any references to WaitingForInputs and RejectedUsedUTXOs
func (tr *OneTxRejected) cleanup() {
	bidx := tr.Id.BIdx()
	// remove references to this tx from RejectedUsedUTXOs
	for _, inp := range tr.TxIn {
		uidx := inp.Input.UIdx()
		if ref := RejectedUsedUTXOs[uidx]; ref != nil {
			newref := make([]btc.BIDX, 0, len(ref)-1)
			for _, bi := range ref {
				if bi != bidx {
					newref = append(newref, bi)
				}
			}
			if len(newref) != len(ref) {
				if len(newref) == 0 {
					delete(RejectedUsedUTXOs, uidx)
					common.CountSafe("TxUsedUTXOdel")
				} else {
					RejectedUsedUTXOs[uidx] = newref
					common.CountSafe("TxUsedUTXOrem")
				}
			} else {
				println("ERROR: TxR", tr.Id.String(), "was in RejectedUsedUTXOs, but not on the list. PLEASE REPORT!")
			}
		}
	}

	// remove references to this tx from WaitingForInputs
	if tr.Waiting4 != nil {
		w4idx := tr.Waiting4.BIdx()
		if w4i := WaitingForInputs[w4idx]; w4i != nil {
			if len(w4i.Ids) == 1 {
				if w4i.Ids[0] != bidx {
					println("ERROR: WaitingForInputs record does not have us at the only txr\n  txr:", tr.Waiting4.String(), tr.Id.String())
				} else {
					delete(WaitingForInputs, w4idx)
				}
			} else {
				idx := slices.Index(w4i.Ids, bidx)
				if idx < 0 {
					println("ERROR: WaitingForInputs record len", len(w4i.Ids), "does nnot have us\n  ", tr.Waiting4.String(), tr.Id.String())
				} else {
					w4i.Ids = slices.Delete(w4i.Ids, idx, idx+1)
				}
			}
		} else {
			println("ERROR: WaitingForInputs record not found for", tr.Waiting4.String(), "from txr", tr.Id.String())
		}
		WaitingForInputsSize -= uint64(tr.Footprint)
		tr.Waiting4 = nil
		// note that this will affect Footprint
	}
}

func RejectTx(tx *btc.Tx, why byte, missingid *btc.Uint256) {
	TxMutex.Lock()
	rejectTx(tx, why, missingid)
	TxMutex.Unlock()
}

// rejectTx adds a transaction to the rejected list or not, if it has been mined already.
// Make sure to call it with locked TxMutex.
// Returns the OneTxRejected or nil if it has not been added.
func rejectTx(tx *btc.Tx, why byte, missingid *btc.Uint256) {
	txr := new(OneTxRejected)
	txr.Id.Hash = tx.Hash.Hash
	txr.Time = time.Now()
	txr.Size = uint32(len(tx.Raw))
	txr.Reason = why
	// only store tx for selected reasons
	if why >= 200 {
		txr.Tx = tx
		txr.Waiting4 = missingid
		// Note: WaitingForInputs and RejectedUsedUTXOs will be updated in AddRejectedTx
	}
	tx.Clean()
	txr.Footprint = uint32(txr.SysSize())
	common.CountSafePar("TxRejected-", txr.Reason)
	AddRejectedTx(txr)
	//return rec
}

// call this function after the tx has been accepted,
// to re-submit all txs that had been waiting for it
func txAccepted(bidx btc.BIDX) (ok bool, cnt int) {
	var wtg *OneWaitingList
	if wtg, ok = WaitingForInputs[bidx]; !ok {
		return
	}

	w4idone := []btc.BIDX{bidx}
	wtg_ids := make([]btc.BIDX, len(wtg.Ids), 4*len(wtg.Ids))
	copy(wtg_ids, wtg.Ids)

	// save the entry conditions so we can print them later
	var e *bytes.Buffer
	if CheckForErrors() {
		e = bytes.NewBuffer(make([]byte, 0, 2048))
		fmt.Fprintln(e, "W4Input txid:", wtg.TxID.String())
		fmt.Fprintln(e, "", len(wtg_ids), "recs at entry")
		_, file, line, _ := runtime.Caller(1)
		fmt.Fprintln(e, " called from file:", file, "  line:", line)
		for ii, rr := range wtg_ids {
			re, ok := TransactionsRejected[rr]
			fmt.Fprintln(e, "  - txr_idx", ii, "  bidx:", btc.BIdxString(rr), ok)
			if ok {
				fmt.Fprintln(e, "      ->", re.Id.String(), re.Reason, re.Tx)
			}
		}
	}

	for idx := 0; idx < len(wtg_ids); idx++ {
		k := wtg_ids[idx]
		txr := TransactionsRejected[k]
		if txr == nil {
			common.CountSafe("Tx**W4InMissing") // this happens if processTx() in this loop removed the tx from our wtg_ids
			println("ERROR: WaitingForInput not found in rejected", wtg.TxID.String(), btc.BIdxString(k), idx)
			if CheckForErrors() {
				println("all list:", len(wtg_ids))
				for _idx, _k := range wtg_ids {
					_, ok := TransactionsRejected[_k]
					println(" ", _idx, btc.BIdxString(_k), ok)
				}
				if e != nil {
					print(e.String())
				}
			}
			continue
		}
		//if CheckForErrors() { // TODO: always check it, as it's not time consuming and there have been issues here
		if txr.Tx == nil || txr.Reason != TX_REJECTED_NO_TXOU {
			// this should never happen
			println("ERROR: WaitingForInput found in rejected, but bad data or reason:", txr.Id.String(), txr.Tx, txr.Reason)
			continue
		}
		DeleteRejectedByTxr(txr)
		if CheckForErrors() {
			if w, ok := WaitingForInputs[bidx]; ok {
				if slices.Contains(w.Ids, txr.Id.BIdx()) {
					println("ERROR: txr has just been removed but it it still in w4r record")
					println("  txr:", txr.Id.String(), txr.Reason, txr.Tx != nil)
					print("  w4i: ", w.TxID.String(), "  ids:")
					for _, bb := range w.Ids {
						println("  ", btc.BIdxString(bb))
					}
					println()
				}
			}
		}
		pendtxrcv := &TxRcvd{Tx: txr.Tx}
		if res, t2s := processTx(pendtxrcv); res == 0 {
			cnt++
			// if res was 0, t2s is not nil
			if wtg, ok := WaitingForInputs[t2s.Hash.BIdx()]; ok {
				wtg_ids = append(wtg_ids, wtg.Ids...)
				w4idone = append(w4idone, t2s.Hash.BIdx())
			}
			common.CountSafe("TxRetryAccepted")
		} else {
			common.CountSafePar("TxRetryRjctd-", res)
		}
	}
	if CheckForErrors() {
		for id, wd := range w4idone {
			if _, yes := WaitingForInputs[wd]; !yes {
				println("ERROR: WaitingForInputs not completely removed -", id+1, "of", len(w4idone))
				print(e.String())
				return
			}
		}
	}
	return
}

// Make sure to call it with locked TxMutex
func (tr *OneTxRejected) Discard() {
	TransactionsRejectedSize -= uint64(tr.Footprint)
	tr.cleanup()
	tr.Tx = nil
	tr.Footprint = uint32(tr.SysSize())
	TransactionsRejectedSize += uint64(tr.Footprint)
}

func ReasonToString(reason byte) string {
	switch reason {
	case 0:
		return ""
	case TX_REJECTED_DISABLED:
		return "RELAY_OFF"
	case TX_REJECTED_NOT_PENDING:
		return "NOT_PENDING"
	case TX_REJECTED_TOO_BIG:
		return "TOO_BIG"
	case TX_REJECTED_FORMAT:
		return "FORMAT"
	case TX_REJECTED_LEN_MISMATCH:
		return "LEN_MISMATCH"
	case TX_REJECTED_EMPTY_INPUT:
		return "EMPTY_INPUT"
	case TX_REJECTED_DATA_PURGED:
		return "PURGED"
	case TX_REJECTED_OVERSPEND:
		return "OVERSPEND"
	case TX_REJECTED_BAD_INPUT:
		return "BAD_INPUT"
	case TX_REJECTED_NO_TXOU:
		return "NO_TXOU"
	case TX_REJECTED_BAD_PARENT:
		return "BAD_PARENT"
	case TX_REJECTED_LOW_FEE:
		return "LOW_FEE"
	case TX_REJECTED_NOT_MINED:
		return "NOT_MINED"
	case TX_REJECTED_CB_INMATURE:
		return "CB_INMATURE"
	case TX_REJECTED_RBF_LOWFEE:
		return "RBF_LOWFEE"
	case TX_REJECTED_RBF_FINAL:
		return "RBF_FINAL"
	case TX_REJECTED_RBF_100:
		return "RBF_100"
	case TX_REJECTED_REPLACED:
		return "REPLACED"
	}
	return fmt.Sprint("UNKNOWN_", reason)
}

func limitRejectedSizeIfNeeded() {
	if len(GetMPInProgressTicket) != 0 {
		return // don't do it during mpget as there may be many short lived NO_TXOU
	}

	max := atomic.LoadUint64(&common.MaxNoUtxoSizeBytes)
	if WaitingForInputsSize > max {
		//fmt.Println("Limiting NoUtxo cached txs from", WaitingForInputsSize, "to", max, TRIdxTail, TRIdxHead)
		start_cnt := len(WaitingForInputs)
		start_siz := WaitingForInputsSize
		for idx := TRIdxTail; idx != TRIdxHead; idx = TRIdxNext(idx) {
			if TRIdIsZeroArrayRec(idx) {
				continue
			}
			if txr, ok := TransactionsRejected[TRIdxArray[idx]]; ok && txr.Waiting4 != nil {
				DeleteRejectedByTxr(txr) // this should do TRIdZeroArrayRec and (may) advance TRIdxTail
				if WaitingForInputsSize <= max {
					break
				}
			}
		}
		common.CountSafeAdd("TxRLimNoUtxoBytes", start_siz-WaitingForInputsSize)
		common.CountSafeAdd("TxRLimNoUtxoCount", uint64(start_cnt-len(WaitingForInputs)))
		//fmt.Println("Deleted", start_cnt-len(WaitingForInputs), "NoUtxo.  New size:", WaitingForInputsSize, "  new_tail:", first_valid_tail)
		verTxrCnt()
	}

	max = atomic.LoadUint64(&common.MaxRejectedSizeBytes)
	if TransactionsRejectedSize <= max {
		return
	}
	//fmt.Println("Limiting rejected size from", TransactionsRejectedSize, "to", max)
	start_cnt := len(TransactionsRejected)
	start_siz := TransactionsRejectedSize
	for TRIdxTail != TRIdxHead {
		if !TRIdIsZeroArrayRec(TRIdxTail) {
			if txr, ok := TransactionsRejected[TRIdxArray[TRIdxTail]]; ok {
				if TRIdxTail != int(txr.ArrIndex) {
					panic("txr's ArrIndex does not point to the tail")
				}
				DeleteRejectedByTxr(txr) // this should do TRIdZeroArrayRec and advance TRIdxTail
			} else {
				panic(fmt.Sprint("TRIdxArray[", TRIdxTail, "] not found in TransactionsRejected"))
			}
			if TransactionsRejectedSize <= max {
				break
			}
		} else {
			TRIdxTail = TRIdxNext(TRIdxTail) // advance TRIdxTail manually
		}
	}
	verTxrCnt()
	common.CountSafeAdd("TxRLimSizBytes", start_siz-TransactionsRejectedSize)
	common.CountSafeAdd("TxRLimSizCount", uint64(start_cnt-len(TransactionsRejected)))
	//fmt.Println("Deleted", start_cnt-len(TransactionsRejected), "txrs.   New size:", TransactionsRejectedSize, "in", len(TransactionsRejected))
}

func resizeTransactionsRejectedCount(newcnt int) {
	if checkRejectedTxs() > 0 || checkRejectedUsedUTXOs() > 0 {
		panic("failed  before resizeTransactionsRejectedCount")
	}
	old_txrs := make([]*OneTxRejected, 0, len(TransactionsRejected))
	for {
		if !TRIdIsZeroArrayRec(TRIdxTail) {
			if txr, ok := TransactionsRejected[TRIdxArray[TRIdxTail]]; ok {
				old_txrs = append(old_txrs, txr)
			} else {
				println("ERROR: TRIdxArray cointains bad pointer on non-zero record", TRIdxTail)
			}
		}
		if TRIdxTail == TRIdxHead {
			break
		}
		TRIdxTail = TRIdxNext(TRIdxTail)
	}

	TRIdxArray = make([]btc.BIDX, newcnt)
	TRIdxHead = 0
	TRIdxTail = 0

	var from_idx int
	if (newcnt - 1) < len(old_txrs) { // maximum number of txs we can fit is the array size minus 1
		from_idx = len(old_txrs) - (newcnt - 1)
	}

	for idx, txr := range old_txrs {
		bidx := txr.Id.BIdx()
		if idx < from_idx {
			TransactionsRejectedSize -= uint64(txr.Footprint)
			if txr.Tx != nil {
				txr.cleanup()
			}
			deleteTransactionsRejected(bidx)
		} else {
			txr.ArrIndex = uint16(TRIdxHead)
			TRIdxArray[TRIdxHead] = bidx
			TransactionsRejected[bidx] = txr
			TRIdxHead = TRIdxNext(TRIdxHead)
			if TRIdxHead == TRIdxTail {
				TRIdxTail = TRIdxNext(TRIdxTail)
			}
		}
	}
	if checkRejectedTxs() > 0 || checkRejectedUsedUTXOs() > 0 {
		panic("resizeTransactionsRejectedCount failed")
	}
}

func limitRejected() {
	if cnt := int(common.Get(&common.CFG.TXPool.RejectRecCnt)); cnt != len(TRIdxArray) {
		resizeTransactionsRejectedCount(cnt)
		return
	}
	verTxrCnt()
	limitRejectedSizeIfNeeded()
}

// Make sure to call it with locked TxMutex.
func InitTransactionsRejected() {
	cnt := common.Get(&common.CFG.TXPool.RejectRecCnt)
	TransactionsRejected = make(map[btc.BIDX]*OneTxRejected, cnt)
	TransactionsRejectedSize = 0

	TRIdxArray = make([]btc.BIDX, cnt)
	TRIdxHead = 0
	TRIdxTail = 0

	WaitingForInputs = make(map[btc.BIDX]*OneWaitingList)
	WaitingForInputsSize = 0
	RejectedUsedUTXOs = make(map[uint64][]btc.BIDX)
}
