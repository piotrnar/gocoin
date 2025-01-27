package network

import (
	"fmt"
	"sync/atomic"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
)

var (
	// Transactions that we downloaded, but rejected:
	TransactionsRejected     map[BIDX]*OneTxRejected = make(map[BIDX]*OneTxRejected)
	TransactionsRejectedSize uint64                  // only include those that have *Tx pointer set

	TRIdxArray []BIDX
	TRIdxHead  int
	TRIdxTail  int

	// Transactions that are waiting for inputs:
	// Each record points to a list of transactions that are waiting for the transaction from the index of the map
	// This way when a new tx is received, we can quickly find all the txs that have been waiting for it
	WaitingForInputs     map[BIDX]*OneWaitingList = make(map[BIDX]*OneWaitingList)
	WaitingForInputsSize uint64

	// Inputs that are being used by TransactionsRejected
	// Each record points to one TransactionsRejected with Reason of 200 or more
	RejectedUsedUTXOs map[uint64][]BIDX = make(map[uint64][]BIDX)
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
	Ids  []BIDX // List of pending tx ids
}

const (
	TX_REJECTED_DISABLED = 1 // Only used for transactions in TransactionsToSend for Blocked field

	TX_REJECTED_TOO_BIG      = 101
	TX_REJECTED_FORMAT       = 102
	TX_REJECTED_LEN_MISMATCH = 103
	TX_REJECTED_EMPTY_INPUT  = 104

	TX_REJECTED_OVERSPEND = 154
	TX_REJECTED_BAD_INPUT = 157

	TX_REJECTED_DATA_PURGED = 199

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
		if TRIdxArray[idx][i] == 0 {
			return true
		}
	}
	return false
}

// Make sure to call it with locked TxMutex.
func AddRejectedTx(txr *OneTxRejected) {
	bidx := txr.Id.BIdx()
	if _, ok := TransactionsRejected[bidx]; ok {
		println("ERROR: AddRejectedTx: TxR", txr.Id.String(), "is already on the list")
		return
	}
	txr.ArrIndex = uint16(TRIdxHead)
	if !TRIdIsZeroArrayRec(TRIdxHead) {
		DeleteRejectedByIdx(TRIdxArray[TRIdxHead])
	}
	TRIdxArray[TRIdxHead] = bidx
	TransactionsRejected[bidx] = txr
	TRIdxHead = TRIdxNext(TRIdxHead)
	if TRIdxHead == TRIdxTail {
		TRIdxTail = TRIdxNext(TRIdxTail)
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
		TransactionsRejectedSize += uint64(txr.Footprint)
	}

	limitRejectedSizeIfNeeded()
}

// Make sure to call it with locked TxMutex
func DeleteRejectedByTxr(txr *OneTxRejected) {
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
	delete(TransactionsRejected, txr.Id.BIdx())
}

// Make sure to call it with locked TxMutex
func DeleteRejectedByIdx(bidx BIDX) {
	if txr, ok := TransactionsRejected[bidx]; ok {
		DeleteRejectedByTxr(txr)
	} else {
		println("ERROR: DeleteRejectedByIdx called with bidx which does not point to any txr", btc.BIdxString(bidx))
	}
}

// Remove any references to WaitingForInputs and RejectedUsedUTXOs
func (tr *OneTxRejected) cleanup() {
	bidx := tr.Id.BIdx()
	// remove references to this tx from RejectedUsedUTXOs
	for _, inp := range tr.TxIn {
		uidx := inp.Input.UIdx()
		if ref := RejectedUsedUTXOs[uidx]; ref != nil {
			newref := make([]BIDX, 0, len(ref)-1)
			for _, bi := range ref {
				if bi != bidx {
					newref = append(newref, bi)
				}
			}
			if len(newref) == len(ref) {
				println("ERROR: TxR", tr.Id.String(), "was in RejectedUsedUTXOs, but not on the list. PLEASE REPORT!")
			} else {
				if len(newref) == 0 {
					delete(RejectedUsedUTXOs, uidx)
					common.CountSafe("TxUsedUTXOdel")
				} else {
					RejectedUsedUTXOs[uidx] = newref
					common.CountSafe("TxUsedUTXOrem")
				}
			}
		}
	}

	// remove references to this tx from WaitingForInputs
	if tr.Waiting4 != nil {
		w4idx := tr.Waiting4.BIdx()
		if w4i := WaitingForInputs[w4idx]; w4i != nil {
			newlist := make([]BIDX, 0, len(w4i.Ids)-1)
			for _, x := range w4i.Ids {
				if x != bidx {
					newlist = append(newlist, x)
				}
			}
			if len(newlist) == len(w4i.Ids) {
				println("ERROR: WaitingForInputs record", tr.Waiting4.String(), "did not point back to txr", tr.Id.String())
			} else {
				if len(newlist) == 0 {
					delete(WaitingForInputs, w4idx)
				} else {
					w4i.Ids = newlist
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

// RejectTx adds a transaction to the rejected list or not, if it has been mined already.
// Make sure to call it with locked TxMutex.
// Returns the OneTxRejected or nil if it has not been added.
func RejectTx(tx *btc.Tx, why byte, missingid *btc.Uint256) {
	txr := new(OneTxRejected)
	txr.Id.Hash = tx.Hash.Hash
	txr.Time = time.Now()
	txr.Size = uint32(len(tx.Raw))
	txr.Reason = why
	// only store tx for selected reasons
	if why >= 200 {
		//tx.Clean()
		txr.Tx = tx
		txr.Waiting4 = missingid
		// Note: WaitingForInputs and RejectedUsedUTXOs will be updated in AddRejectedTx
	}
	txr.Footprint = uint32(txr.SysSize())
	common.CountSafePar("TxRejected-", txr.Reason)
	AddRejectedTx(txr)
	//return rec
}

// Make sure to call it with locked TxMutex
func RetryWaitingForInput(wtg *OneWaitingList) {
	for _, k := range wtg.Ids {
		txr := TransactionsRejected[k]
		if txr.Tx == nil {
			println(fmt.Sprintf("ERROR: txr %s %d in w4i rec %16x, but data is nil (its w4prt:%p)", txr.Id.String(), txr.Reason, k, txr.Waiting4))
			continue
		}
		pendtxrcv := &TxRcvd{Tx: txr.Tx}
		if HandleNetTx(pendtxrcv, true) {
			common.CountSafe("TxRetryAccepted")
			if txr, ok := TransactionsRejected[k]; ok {
				println("ERROR: tx", txr.Id.String(), "accepted but still in rejected")
			}
		} else {
			common.CountSafe("TxRetryRejected")
		}
	}
}

// Make sure to call it with locked TxMutex
func (tr *OneTxRejected) Discard() {
	if tr.Tx == nil {
		panic("OneTxRejected.Discard() called, but it's already empty")
	}
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
		return // don't do it during mpget as there always are many short lived NO_TXOU
	}

	max := atomic.LoadUint64(&common.MaxNoUtxoSizeBytes)
	if WaitingForInputsSize > max {
		//fmt.Println("Limiting NoUtxo cached txs from", WaitingForInputsSize, "to", max, TRIdxTail, TRIdxHead)
		start_cnt := len(WaitingForInputs)
		start_siz := WaitingForInputsSize
		first_valid_tail := -1
		var stop_moving_tail bool
		for idx := TRIdxTail; idx != TRIdxHead; idx = TRIdxNext(idx) {
			if txr, ok := TransactionsRejected[TRIdxArray[idx]]; ok {
				if txr.Waiting4 != nil {
					DeleteRejectedByTxr(txr)
					TRIdZeroArrayRec(idx)
					if !stop_moving_tail {
						first_valid_tail = idx
					}
				} else {
					stop_moving_tail = true
				}
			}
			if WaitingForInputsSize <= max {
				break
			}
		}
		common.CountSafeAdd("TxRLimNoUtxoBytes", start_siz-WaitingForInputsSize)
		common.CountSafeAdd("TxRLimNoUtxoCount", uint64(start_cnt-len(WaitingForInputs)))
		if first_valid_tail >= 0 {
			cnt := first_valid_tail - TRIdxTail
			if cnt < 0 {
				cnt += len(TRIdxArray)
			}
			common.CountSafeAdd("TxRLimNoUtxoMoveTail", uint64(cnt))
			TRIdxTail = first_valid_tail
		}
		//fmt.Println("Deleted", start_cnt-len(WaitingForInputs), "NoUtxo.  New size:", WaitingForInputsSize, "  new_tail:", first_valid_tail)
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
			DeleteRejectedByIdx(TRIdxArray[TRIdxTail])
		}
		TRIdZeroArrayRec(TRIdxTail)
		TRIdxTail = TRIdxNext(TRIdxTail)
		if TransactionsRejectedSize <= max {
			break
		}
	}
	common.CountSafeAdd("TxRLimSizBytes", start_siz-TransactionsRejectedSize)
	common.CountSafeAdd("TxRLimSizCount", uint64(start_cnt-len(TransactionsRejected)))
	//fmt.Println("Deleted", start_cnt-len(TransactionsRejected), "txrs.   New size:", TransactionsRejectedSize)
}

func resizeTransactionsRejectedCount(newcnt int) {
	old_txrs := make([]*OneTxRejected, 0, len(TransactionsRejected))
	for {
		if txr := TransactionsRejected[TRIdxArray[TRIdxTail]]; txr != nil {
			old_txrs = append(old_txrs, txr)
			DeleteRejectedByTxr(txr)
		}
		if TRIdxTail == TRIdxHead {
			break
		}
		TRIdxTail = TRIdxNext(TRIdxTail)
	}

	TransactionsRejected = make(map[BIDX]*OneTxRejected, newcnt)
	TRIdxArray = make([]BIDX, newcnt)
	TRIdxHead = 0
	TRIdxTail = 0

	var from_idx int
	if newcnt < len(old_txrs) {
		from_idx = len(old_txrs) - newcnt
	}

	for _, txr := range old_txrs[from_idx:] {
		AddRejectedTx(txr)
	}
}

func doRejected() {
	TxMutex.Lock()
	defer TxMutex.Unlock()
	if cnt := int(common.Get(&common.CFG.TXPool.RejectRecCnt)); cnt != len(TRIdxArray) {
		resizeTransactionsRejectedCount(cnt)
		return
	}
	limitRejectedSizeIfNeeded()
}

// Make sure to call it with locked TxMutex.
func InitTransactionsRejected() {
	cnt := common.Get(&common.CFG.TXPool.RejectRecCnt)
	TransactionsRejected = make(map[BIDX]*OneTxRejected, cnt)
	TransactionsRejectedSize = 0

	TRIdxArray = make([]BIDX, cnt)
	TRIdxHead = 0
	TRIdxTail = 0

	WaitingForInputs = make(map[BIDX]*OneWaitingList)
	WaitingForInputsSize = 0
	RejectedUsedUTXOs = make(map[uint64][]BIDX)
}
