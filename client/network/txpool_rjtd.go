package network

import (
	"fmt"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
)

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
	TX_REJECTED_LOW_FEE     = 205
	TX_REJECTED_NOT_MINED   = 208
	TX_REJECTED_CB_INMATURE = 209
	TX_REJECTED_RBF_LOWFEE  = 210
	TX_REJECTED_RBF_FINAL   = 211
	TX_REJECTED_RBF_100     = 212
	TX_REJECTED_REPLACED    = 213
)

var (
	// Transactions that we downloaded, but rejected:
	TransactionsRejected     map[BIDX]*OneTxRejected = make(map[BIDX]*OneTxRejected)
	TransactionsRejectedSize uint64                  // only include those that have *Tx pointer set

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
	Id *btc.Uint256
	time.Time
	Size     uint32
	Waiting4 *btc.Uint256
	*btc.Tx
	Reason byte
}

type OneWaitingList struct {
	TxID *btc.Uint256
	Ids  []BIDX // List of pending tx ids
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

// RejectTx adds a transaction to the rejected list or not, if it has been mined already.
// Make sure to call it with locked TxMutex.
// Returns the OneTxRejected or nil if it has not been added.
func RejectTx(tx *btc.Tx, why byte) *OneTxRejected {
	rec := new(OneTxRejected)
	rec.Time = time.Now()
	rec.Size = uint32(len(tx.Raw))
	rec.Reason = why

	// only store tx for selected reasons
	if why >= 200 {
		tx.Clean()
		rec.Tx = tx
		rec.Id = &tx.Hash
		TransactionsRejectedSize += uint64(rec.Size)
		for _, inp := range tx.TxIn {
			uidx := inp.Input.UIdx()
			RejectedUsedUTXOs[uidx] = append(RejectedUsedUTXOs[uidx], rec.Hash.BIdx())
		}
	} else {
		rec.Id = new(btc.Uint256)
		rec.Id.Hash = tx.Hash.Hash
	}

	bidx := tx.Hash.BIdx()
	TransactionsRejected[bidx] = rec

	return rec
}
func RetryWaitingForInput(wtg *OneWaitingList) {
	for _, k := range wtg.Ids {
		txr := TransactionsRejected[k]
		if txr.Tx == nil {
			fmt.Printf("ERROR: txr %s %d in w4i rec %16x, but has not data (its w4prt:%p)\n",
				txr.Id.String(), txr.Reason, k, txr.Waiting4)
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
	tr.cleanup()
	TransactionsRejectedSize -= uint64(tr.Size)
	tr.Tx = nil
}

// Make sure to call it with locked TxMutex
func DeleteRejected(bidx BIDX) {
	if tr, ok := TransactionsRejected[bidx]; ok {
		if tr.Tx != nil {
			tr.cleanup()
			TransactionsRejectedSize -= uint64(TransactionsRejected[bidx].Size)
		}
		delete(TransactionsRejected, bidx)
	} else {
		println("ERROR: DeleteRejected called for non-existing txr")
	}
}

// LimitRejectedSize must be called with TxMutex locked.
func LimitRejectedSize() {
	//ticklen := maxlen >> 5 // 1/32th of the max size = X
	var idx int
	var sorted []*OneTxRejected

	old_cnt := len(TransactionsRejected)
	old_size := TransactionsRejectedSize

	maxcnt, maxlen, maxw4ilen := common.RejectedTxsLimits()

	if maxcnt > 0 && len(TransactionsRejected) > maxcnt {
		common.CountSafe("TxRLimitCnt")
		sorted = GetSortedRejected()
		maxcnt -= maxcnt >> 5
		for idx = maxcnt; idx < len(sorted); idx++ {
			DeleteRejected(sorted[idx].Id.BIdx())
		}
		sorted = sorted[:maxcnt]
		common.CountSafeAdd("TxRLimitCntCnt", uint64(old_cnt-len(TransactionsRejected)))
		common.CountSafeAdd("TxRLimitCntBts", old_size-TransactionsRejectedSize)
		old_cnt = len(TransactionsRejected)
		old_size = TransactionsRejectedSize
	}

	var removed map[int]bool
	if maxw4ilen > 0 && WaitingForInputsSize > maxw4ilen {
		common.CountSafe("TxRLimitUtxo")
		if sorted == nil {
			sorted = GetSortedRejected()
		}
		maxw4ilen -= maxw4ilen >> 5
		removed = make(map[int]bool, len(sorted))
		for idx = len(sorted) - 1; idx >= 0; idx-- {
			if sorted[idx].Waiting4 == nil {
				continue
			}
			DeleteRejected(sorted[idx].Hash.BIdx())
			removed[idx] = true
			if WaitingForInputsSize <= maxw4ilen {
				break
			}
		}
		common.CountSafeAdd("TxRLimitUtxoCnt", uint64(old_cnt-len(TransactionsRejected)))
		common.CountSafeAdd("TxRLimitUtxoBts", old_size-TransactionsRejectedSize)
		old_cnt = len(TransactionsRejected)
		old_size = TransactionsRejectedSize
	}

	if maxlen > 0 && TransactionsRejectedSize > maxlen {
		common.CountSafe("TxRLimitSize")
		if sorted == nil {
			sorted = GetSortedRejected()
		} else if len(removed) > 0 {
			sorted_new := make([]*OneTxRejected, 0, len(sorted))
			for i := range sorted {
				if !removed[i] {
					sorted_new = append(sorted_new, sorted[i])
				}
			}
			sorted = sorted_new
		}
		maxlen -= maxlen >> 5
		for idx = len(sorted) - 1; idx >= 0; idx-- {
			if _, ok := TransactionsRejected[sorted[idx].Hash.BIdx()]; ok {
				println("ERROR in LimitRejectedSize - txr in sorted but not in Rejected", idx, len(sorted))
				println("  txid:", sorted[idx].Hash.String())
				common.CountSafe("Tx**RLimitPipa")
				continue
			}
			DeleteRejected(sorted[idx].Hash.BIdx())
			if TransactionsRejectedSize <= maxlen {
				break
			}
		}
		common.CountSafeAdd("TxRLimitSizeCnt", uint64(old_cnt-len(TransactionsRejected)))
		common.CountSafeAdd("TxRLimitSizeBts", old_size-TransactionsRejectedSize)
	}
}
