package network

import (
	"fmt"
	"sort"
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

	TransactionsRejectedIdx  []BIDX
	TransactionsRejectedHead int
	TransactionsRejectedTail int

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

func nextIdx(idx int) int {
	if idx == len(TransactionsRejectedIdx)-1 {
		return 0
	}
	return idx + 1
}

// Make sure to call it with locked TxMutex.
func AddRejectedTx(txr *OneTxRejected) {
	bidx := txr.Id.BIdx()
	if _, ok := TransactionsRejected[bidx]; ok {
		println("ERROR in AddRejectedTx: TxR", txr.Id.String(), "is already on the list")
		common.CountSafe("Tx**RejAddConflict")
		return
	}
	next_head := nextIdx(TransactionsRejectedHead)
	if next_head == TransactionsRejectedTail {
		DeleteRejected(TransactionsRejectedIdx[next_head])
		common.CountSafe("TxRIdxNextTail")
		TransactionsRejectedTail = nextIdx(TransactionsRejectedTail)
	}
	TransactionsRejectedIdx[TransactionsRejectedHead] = bidx
	TransactionsRejectedHead = next_head
	TransactionsRejected[bidx] = txr
	if txr.Tx != nil {
		TransactionsRejectedSize += uint64(len(txr.Raw))
	}
}

// Make sure to call it with locked TxMutex
func DeleteRejected(bidx BIDX) {
	if tr, ok := TransactionsRejected[bidx]; ok {
		common.CountSafe(fmt.Sprint("TxRIdxDel-", tr.Reason))
		if tr.Tx != nil {
			tr.cleanup()
			TransactionsRejectedSize -= uint64(TransactionsRejected[bidx].Size)
		}
		delete(TransactionsRejected, bidx)
	} else {
		common.CountSafe("TxRIdxNull")
		//println("ERROR: DeleteRejected called for non-existing txr")
	}
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
		for _, inp := range tx.TxIn {
			uidx := inp.Input.UIdx()
			RejectedUsedUTXOs[uidx] = append(RejectedUsedUTXOs[uidx], rec.Hash.BIdx())
		}
	} else {
		rec.Id = new(btc.Uint256)
		rec.Id.Hash = tx.Hash.Hash
	}
	AddRejectedTx(rec)
	return rec
}

// Make sure to call it with locked TxMutex
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

func GetSortedRejectedOld() (sorted []*OneTxRejected) {
	var idx int
	sorted = make([]*OneTxRejected, len(TransactionsRejected))
	for _, t := range TransactionsRejected {
		sorted[idx] = t
		idx++
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[j].Time.Before(sorted[i].Time)
	})
	return
}

func GetSortedRejected() (sorted []*OneTxRejected) {
	sorted = make([]*OneTxRejected, 0, len(TransactionsRejected))
	idx := TransactionsRejectedHead
	for {
		if idx == TransactionsRejectedTail {
			return
		}
		if idx == 0 {
			idx = len(TransactionsRejectedIdx) - 1
		} else {
			idx--
		}
		if txr, ok := TransactionsRejected[TransactionsRejectedIdx[idx]]; ok {
			if txr == nil {
				println("ERROR: TransactionsRejected record is nil - this must not happen!!!")
				continue
			}
			sorted = append(sorted, txr)
		}
	}
}

// Make sure to call it with locked TxMutex.
func InitTransactionsRejected() {
	common.LockCfg()
	cnt := common.CFG.TXPool.MaxRejectCnt
	common.UnlockCfg()
	TransactionsRejected = make(map[BIDX]*OneTxRejected, cnt)
	TransactionsRejectedSize = 0

	TransactionsRejectedIdx = make([]BIDX, cnt)
	TransactionsRejectedHead = 0
	TransactionsRejectedTail = 0

	WaitingForInputs = make(map[BIDX]*OneWaitingList)
	WaitingForInputsSize = 0
	RejectedUsedUTXOs = make(map[uint64][]BIDX)
}
