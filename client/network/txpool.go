package network

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/lib/script"
	"io"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

const (
	TX_REJECTED_DISABLED = 1

	TX_REJECTED_TOO_BIG      = 101
	TX_REJECTED_FORMAT       = 102
	TX_REJECTED_LEN_MISMATCH = 103
	TX_REJECTED_EMPTY_INPUT  = 104

	TX_REJECTED_DOUBLE_SPEND = 201
	TX_REJECTED_NO_TXOU      = 202
	//TX_REJECTED_DUST         = 203 - I made this one deprecated as "dust" was a stupid concept in the first place
	TX_REJECTED_OVERSPEND   = 204
	TX_REJECTED_LOW_FEE     = 205
	TX_REJECTED_SCRIPT_FAIL = 206
	TX_REJECTED_BAD_INPUT   = 207
	TX_REJECTED_NOT_MINED   = 208
	TX_REJECTED_CB_INMATURE = 209
	TX_REJECTED_RBF_LOWFEE  = 210
	TX_REJECTED_RBF_FINAL   = 211
	TX_REJECTED_RBF_100     = 212

	MEMPOOL_FILE_NAME = "mempool.bin"
	MEMPOOL_FILE_NAME2 = "mempool.dmp"
)

var (
	TxMutex sync.Mutex

	// The actual memory pool:
	TransactionsToSend     map[BIDX]*OneTxToSend = make(map[BIDX]*OneTxToSend)
	TransactionsToSendSize uint64

	// All the outputs that are currently spent in TransactionsToSend:
	SpentOutputs map[uint64]BIDX = make(map[uint64]BIDX)

	// Transactions that we downloaded, but rejected:
	TransactionsRejected     map[BIDX]*OneTxRejected = make(map[BIDX]*OneTxRejected)
	TransactionsRejectedSize uint64

	// Transactions that are received from network (via "tx"), but not yet processed:
	TransactionsPending map[BIDX]bool = make(map[BIDX]bool)

	// Transactions that are waiting for inputs:
	WaitingForInputs map[BIDX]*OneWaitingList = make(map[BIDX]*OneWaitingList)

	END_MARKER = []byte("END_OF_FILE")
)

type OneTxToSend struct {
	Data                []byte
	Invsentcnt, SentCnt uint32
	Firstseen, Lastsent time.Time
	Own                 byte     // 0-not own, 1-own and OK, 2-own but with UNKNOWN input
	Spent               []uint64 // Which records in SpentOutputs this TX added
	Volume, Fee         uint64
	*btc.Tx
	Blocked    byte // if non-zero, it gives you the reason why this tx nas not been routed
	MemInputs  bool // transaction is spending inputs from other unconfirmed tx(s)
	SigopsCost uint64
	Final      bool // if true RFB will not work on it
	VerifyTime time.Duration
}

type Wait4Input struct {
	missingTx *btc.Uint256
	*TxRcvd
}

type OneTxRejected struct {
	Id *btc.Uint256
	time.Time
	Size   uint32
	Reason byte
	*Wait4Input
}

type OneWaitingList struct {
	TxID *btc.Uint256
	Ids  map[BIDX]time.Time // List of pending tx ids
}

func NeedThisTx(id *btc.Uint256, cb func()) (res bool) {
	return NeedThisTxExt(id, cb)==0
}

// Return false if we do not want to receive a data for this tx
func NeedThisTxExt(id *btc.Uint256, cb func()) (why_not int) {
	TxMutex.Lock()
	if _, present := TransactionsToSend[id.BIdx()]; present {
		why_not = 1
	} else if _, present := TransactionsRejected[id.BIdx()]; present {
		why_not = 2
	} else if _, present := TransactionsPending[id.BIdx()]; present {
		why_not = 3
	} else if txo, _ := common.BlockChain.Unspent.UnspentGet(&btc.TxPrevOut{Hash: id.Hash}); txo != nil {
		why_not = 4
		// This assumes that tx's out #0 has not been spent yet, which may not always be the case, but well...
		common.CountSafe("TxMinedRejected")
	} else {
		// why_not = 0
		if cb != nil {
			cb()
		}
	}
	TxMutex.Unlock()
	return
}

// Handle tx-inv notifications
func (c *OneConnection) TxInvNotify(hash []byte) {
	if NeedThisTx(btc.NewUint256(hash), nil) {
		var b [1 + 4 + 32]byte
		b[0] = 1 // One inv
		if (c.Node.Services & SERVICE_SEGWIT) != 0 {
			binary.LittleEndian.PutUint32(b[1:5], MSG_WITNESS_TX) // SegWit Tx
			//println(c.ConnID, "getdata", btc.NewUint256(hash).String())
		} else {
			b[1] = MSG_TX // Tx
		}
		copy(b[5:37], hash)
		c.SendRawMsg("getdata", b[:])
	}
}

// Adds a transaction to the rejected list or not, it it has been mined already
// Make sure to call it with locked TxMutex.
// Returns the OneTxRejected or nil if it has not been added.
func RejectTx(id *btc.Uint256, size int, why byte) *OneTxRejected {
	rec := new(OneTxRejected)
	rec.Id = id
	rec.Time = time.Now()
	rec.Size = uint32(size)
	rec.Reason = why
	TransactionsRejected[id.BIdx()] = rec
	TransactionsRejectedSize += uint64(rec.Size)
	return rec
}

// Handle incoming "tx" msg
func (c *OneConnection) ParseTxNet(pl []byte) {
	if uint32(len(pl)) > common.GetUint32(&common.CFG.TXPool.MaxTxSize) {
		common.CountSafe("TxRejectedBig")
		return
	}
	tx, le := btc.NewTx(pl)
	if tx == nil {
		c.DoS("TxRejectedBroken")
		return
	}
	if le != len(pl) {
		c.DoS("TxRejectedLenMismatch")
		return
	}
	if len(tx.TxIn) < 1 {
		c.Misbehave("TxRejectedNoInputs", 100)
		return
	}

	tx.SetHash(pl)
	NeedThisTx(tx.Hash, func() {
		// This body is called with a locked TxMutex
		tx.Raw = pl
		select {
		case NetTxs <- &TxRcvd{conn: c, tx: tx, raw: pl}:
			TransactionsPending[tx.Hash.BIdx()] = true
		default:
			common.CountSafe("TxRejectedFullQ")
			//println("NetTxsFULL")
		}
	})
}

func bidx2str(idx BIDX) string {
	return hex.EncodeToString(idx[:])
}

// Must be called from the chain's thread
func HandleNetTx(ntx *TxRcvd, retry bool) (accepted bool) {
	common.CountSafe("HandleNetTx")

	tx := ntx.tx
	start_time := time.Now()
	var final bool // set to true if any of the inpits has a final sequence

	var totinp, totout uint64
	var frommem bool

	TxMutex.Lock()

	if !retry {
		if _, present := TransactionsPending[tx.Hash.BIdx()]; !present {
			// It had to be mined in the meantime, so just drop it now
			TxMutex.Unlock()
			common.CountSafe("TxNotPending")
			return
		}
		delete(TransactionsPending, ntx.tx.Hash.BIdx())
	} else {
		// In case case of retry, it is on the rejected list,
		// ... so remove it now to free any tied WaitingForInputs
		deleteRejected(tx.Hash.BIdx())
	}

	pos := make([]*btc.TxOut, len(tx.TxIn))
	spent := make([]uint64, len(tx.TxIn))

	rbf_tx_list := make(map[BIDX]bool)

	// Check if all the inputs exist in the chain
	for i := range tx.TxIn {
		if !final && tx.TxIn[i].Sequence >= 0xfffffffe {
			final = true
		}

		spent[i] = tx.TxIn[i].Input.UIdx()

		if so, ok := SpentOutputs[spent[i]]; ok {
			rbf_tx_list[so] = true
		}

		inptx := btc.NewUint256(tx.TxIn[i].Input.Hash[:])

		if txinmem, ok := TransactionsToSend[inptx.BIdx()]; ok {
			if int(tx.TxIn[i].Input.Vout) >= len(txinmem.TxOut) {
				RejectTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_BAD_INPUT)
				TxMutex.Unlock()
				common.CountSafe("TxRejectedBadInput")
				return
			}

			if !ntx.trusted && !common.CFG.TXPool.AllowMemInputs {
				RejectTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_NOT_MINED)
				TxMutex.Unlock()
				common.CountSafe("TxRejectedMemInput1")
				return
			}

			pos[i] = txinmem.TxOut[tx.TxIn[i].Input.Vout]
			common.CountSafe("TxInputInMemory")
			frommem = true
		} else {
			pos[i], _ = common.BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
			if pos[i] == nil {
				var newone bool

				if !common.CFG.TXPool.AllowMemInputs {
					RejectTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_NOT_MINED)
					TxMutex.Unlock()
					common.CountSafe("TxRejectedMemInput2")
					return
				}
				// In this case, let's "save" it for later...
				missingid := btc.NewUint256(tx.TxIn[i].Input.Hash[:])
				nrtx := RejectTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_NO_TXOU)

				if nrtx != nil {
					nrtx.Wait4Input = &Wait4Input{missingTx: missingid, TxRcvd: ntx}

					// Add to waiting list:
					var rec *OneWaitingList
					if rec, _ = WaitingForInputs[nrtx.Wait4Input.missingTx.BIdx()]; rec == nil {
						rec = new(OneWaitingList)
						rec.TxID = nrtx.Wait4Input.missingTx
						rec.Ids = make(map[BIDX]time.Time)
						newone = true
					}
					rec.Ids[tx.Hash.BIdx()] = time.Now()
					WaitingForInputs[nrtx.Wait4Input.missingTx.BIdx()] = rec
				}

				TxMutex.Unlock()
				if newone {
					common.CountSafe("TxRejectedNoInpNew")
				} else {
					common.CountSafe("TxRejectedNoInpOld")
				}
				return
			} else {
				if pos[i].WasCoinbase {
					if common.Last.BlockHeight()+1-pos[i].BlockHeight < chain.COINBASE_MATURITY {
						RejectTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_CB_INMATURE)
						TxMutex.Unlock()
						common.CountSafe("TxRejectedCBInmature")
						fmt.Println(tx.Hash.String(), "trying to spend inmature coinbase block", pos[i].BlockHeight, "at", common.Last.BlockHeight())
						return
					}
				}
			}
		}
		totinp += pos[i].Value
	}

	// Check if total output value does not exceed total input
	for i := range tx.TxOut {
		totout += tx.TxOut[i].Value
	}

	if totout > totinp {
		RejectTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_OVERSPEND)
		TxMutex.Unlock()
		ntx.conn.DoS("TxOverspend")
		return
	}

	// Check for a proper fee
	fee := totinp - totout
	if !ntx.trusted && fee < (uint64(tx.VSize()) * common.MinFeePerKB() / 1000) {
		RejectTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_LOW_FEE)
		TxMutex.Unlock()
		common.CountSafe("TxRejectedLowFee")
		return
	}

	//var new_spb, old_spb float64
	var totlen int
	var totfees, new_min_fee uint64

	if len(rbf_tx_list) > 0 {
		already_done := make(map[BIDX]bool)
		for len(already_done) < len(rbf_tx_list) {
			for k, _ := range rbf_tx_list {
				if _, yes := already_done[k]; !yes {
					already_done[k] = true
					if new_recs := findPendingTxs(TransactionsToSend[k].Tx); len(new_recs) > 0 {
						if len(rbf_tx_list)+len(new_recs) > 100 {
							RejectTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_RBF_100)
							TxMutex.Unlock()
							common.CountSafe("TxRejectedRBF100+")
							return
						}
						for _, id := range new_recs {
							rbf_tx_list[id] = true
						}
					}
				}
			}
		}

		for k, _ := range rbf_tx_list {
			ctx := TransactionsToSend[k]

			if !ntx.trusted && ctx.Final {
				RejectTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_RBF_FINAL)
				TxMutex.Unlock()
				common.CountSafe("TxRejectedRBFFinal")
				return
			}

			totlen += len(ctx.Data)
			totfees += ctx.Fee
		}
		new_min_fee = totfees + (uint64(len(ntx.raw)) * common.MinFeePerKB() / 1000)

		if !ntx.trusted && fee < new_min_fee {
			RejectTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_RBF_LOWFEE)
			TxMutex.Unlock()
			common.CountSafe("TxRejectedRBFLowFee")
			return
		}
	}

	sigops := btc.WITNESS_SCALE_FACTOR * tx.GetLegacySigOpCount()

	if !ntx.trusted { // Verify scripts
		var wg sync.WaitGroup
		var ver_err_cnt uint32

		prev_dbg_err := script.DBG_ERR
		script.DBG_ERR = false // keep quiet for incorrect txs
		for i := range tx.TxIn {
			wg.Add(1)
			go func(prv []byte, amount uint64, i int, tx *btc.Tx) {
				if !script.VerifyTxScript(prv, amount, i, tx, script.STANDARD_VERIFY_FLAGS) {
					atomic.AddUint32(&ver_err_cnt, 1)
				}
				wg.Done()
			}(pos[i].Pk_script, pos[i].Value, i, tx)
		}

		wg.Wait()
		script.DBG_ERR = prev_dbg_err

		if ver_err_cnt > 0 {
			RejectTx(ntx.tx.Hash, len(ntx.raw), TX_REJECTED_SCRIPT_FAIL)
			TxMutex.Unlock()
			ntx.conn.DoS("TxScriptFail")
			if len(rbf_tx_list) > 0 {
				fmt.Println("RBF try", ver_err_cnt, "script(s) failed!")
				fmt.Print("> ")
			}
			return
		}
	}

	for i := range tx.TxIn {
		if btc.IsP2SH(pos[i].Pk_script) {
			sigops += btc.WITNESS_SCALE_FACTOR * btc.GetP2SHSigOpCount(tx.TxIn[i].ScriptSig)
		}
		sigops += uint(tx.CountWitnessSigOps(i, pos[i].Pk_script))
	}

	if len(rbf_tx_list) > 0 {
		for k, _ := range rbf_tx_list {
			ctx := TransactionsToSend[k]
			DeleteToSend(ctx)
			common.CountSafe("TxRemovedByRBF")
		}
	}

	rec := &OneTxToSend{Data: ntx.raw, Spent: spent, Volume: totinp,
		Fee: fee, Firstseen: time.Now(), Tx: tx, MemInputs: frommem,
		SigopsCost: uint64(sigops), Final: final, VerifyTime: time.Now().Sub(start_time)}
	TransactionsToSend[tx.Hash.BIdx()] = rec
	TransactionsToSendSize += uint64(len(rec.Data))
	for i := range spent {
		SpentOutputs[spent[i]] = tx.Hash.BIdx()
	}

	wtg := WaitingForInputs[tx.Hash.BIdx()]
	if wtg != nil {
		defer RetryWaitingForInput(wtg) // Redo waiting txs when leaving this function
	}

	TxMutex.Unlock()
	common.CountSafe("TxAccepted")

	if frommem {
		// Gocoin does not route txs that need unconfirmed inputs
		rec.Blocked = TX_REJECTED_NOT_MINED
		common.CountSafe("TxRouteNotMined")
	} else if !ntx.trusted && isRoutable(rec) {
		// do not automatically route loacally loaded txs
		rec.Invsentcnt += NetRouteInvExt(1, tx.Hash, ntx.conn, 1000*fee/uint64(len(ntx.raw)))
		common.CountSafe("TxRouteOK")
	}

	if ntx.conn != nil {
		ntx.conn.Mutex.Lock()
		ntx.conn.txsCur++
		ntx.conn.X.TxsReceived++
		ntx.conn.Mutex.Unlock()
	}

	accepted = true
	return
}

// Return txs in mempool that are spending any outputs form the given tx
func findPendingTxs(tx *btc.Tx) (res []BIDX) {
	var in btc.TxPrevOut
	copy(in.Hash[:], tx.Hash.Hash[:])
	for in.Vout = 0; in.Vout < uint32(len(tx.TxOut)); in.Vout++ {
		if r, ok := SpentOutputs[in.UIdx()]; ok {
			res = append(res, r)
		}
	}
	return res
}

func isRoutable(rec *OneTxToSend) bool {
	if !common.CFG.TXRoute.Enabled {
		common.CountSafe("TxRouteDisabled")
		rec.Blocked = TX_REJECTED_DISABLED
		return false
	}
	if uint32(len(rec.Data)) > common.GetUint32(&common.CFG.TXRoute.MaxTxSize) {
		common.CountSafe("TxRouteTooBig")
		rec.Blocked = TX_REJECTED_TOO_BIG
		return false
	}
	if rec.Fee < (uint64(rec.VSize()) * common.RouteMinFeePerKB() / 1000) {
		common.CountSafe("TxRouteLowFee")
		rec.Blocked = TX_REJECTED_LOW_FEE
		return false
	}
	return true
}

func RetryWaitingForInput(wtg *OneWaitingList) {
	for k, _ := range wtg.Ids {
		pendtxrcv := TransactionsRejected[k].Wait4Input.TxRcvd
		if HandleNetTx(pendtxrcv, true) {
			common.CountSafe("TxRetryAccepted")
		} else {
			common.CountSafe("TxRetryRejected")
		}
	}
}

// Make sure to call it with locked TxMutex
func DeleteToSend(rec *OneTxToSend) {
	for i := range rec.Spent {
		delete(SpentOutputs, rec.Spent[i])
	}
	TransactionsToSendSize -= uint64(len(rec.Data))
	delete(TransactionsToSend, rec.Tx.Hash.BIdx())
}

func txChecker(tx *btc.Tx) bool {
	TxMutex.Lock()
	rec, ok := TransactionsToSend[tx.Hash.BIdx()]
	TxMutex.Unlock()
	if ok && rec.Own != 0 {
		common.CountSafe("TxScrOwn")
		return false // Assume own txs as non-trusted
	}
	if ok {
		ok = tx.WTxID().Equal(rec.WTxID())
		if !ok {
			println("wTXID mismatch at", tx.Hash.String(), tx.WTxID().String(), rec.WTxID().String())
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

func init() {
	chain.TrustedTxChecker = txChecker
}

// Make sure to call it with locked TxMutex
func deleteRejected(bidx BIDX) {
	if tr, ok := TransactionsRejected[bidx]; ok {
		if tr.Wait4Input != nil {
			w4i, _ := WaitingForInputs[tr.Wait4Input.missingTx.BIdx()]
			delete(w4i.Ids, bidx)
			if len(w4i.Ids) == 0 {
				delete(WaitingForInputs, tr.Wait4Input.missingTx.BIdx())
			}
		}
		TransactionsRejectedSize -= uint64(TransactionsRejected[bidx].Size)
		delete(TransactionsRejected, bidx)
	}
}

func RemoveFromRejected(hash *btc.Uint256) {
	TxMutex.Lock()
	deleteRejected(hash.BIdx())
	TxMutex.Unlock()
}

var (
	poolenabled bool
	expireperbyte float64
	maxexpiretime time.Duration
	timenow time.Time
)

func expireTime(size int) (t time.Time) {
	exp := time.Duration(float64(size) * expireperbyte)
	if exp > maxexpiretime {
		exp = maxexpiretime
	}
	return timenow.Add(-exp)
}

func ExpireTxs() {
	var cnt1a, cnt1b, cnt2 uint64

	common.LockCfg()
	poolenabled = common.CFG.TXPool.Enabled
	expireperbyte = common.ExpirePerByte
	maxexpiretime = common.MaxExpireTime
	common.UnlockCfg()
	timenow = time.Now()

	TxMutex.Lock()
	for _, v := range TransactionsToSend {
		if v.Own == 0 && (!poolenabled || v.Firstseen.Before(expireTime(len(v.Data)))) { // Do not expire own txs
			DeleteToSend(v)
			if v.Blocked == 0 {
				cnt1a++
			} else {
				cnt1b++
			}
		}
	}
	for k, v := range TransactionsRejected {
		if !poolenabled || v.Time.Before(expireTime(int(v.Size))) {
			deleteRejected(k)
			cnt2++
		}
	}
	TxMutex.Unlock()

	common.CounterMutex.Lock()
	common.Counter["TxPurgedTicks"]++
	if cnt1a > 0 {
		common.Counter["TxPurgedOK"] += cnt1a
	}
	if cnt1b > 0 {
		common.Counter["TxPurgedBlocked"] += cnt1b
	}
	if cnt2 > 0 {
		common.Counter["TxPurgedRejected"] += cnt2
	}
	common.CounterMutex.Unlock()
}

/* Old method
func MempoolSave1() {
	if !common.CFG.TXPool.SaveOnDisk {
		os.Remove(common.GocoinHomeDir + MEMPOOL_FILE_NAME)
		return
	}

	f, er := os.Create(common.GocoinHomeDir + MEMPOOL_FILE_NAME)
	if er != nil {
		println(er.Error())
		return
	}

	fmt.Println("Saving", MEMPOOL_FILE_NAME)
	wr := bufio.NewWriter(f)

	wr.Write(common.Last.Block.BlockHash.Hash[:])
	btc.WriteVlen(wr, uint64(len(TransactionsToSend)))

	for _, t2s := range TransactionsToSend {
		btc.WriteVlen(wr, uint64(len(t2s.Raw)))
		wr.Write(t2s.Raw)
	}

	wr.Write(END_MARKER[:])
	wr.Flush()
	f.Close()
}
*/

func bool2byte(v bool) byte {
	if v {
		return 1
	} else {
		return 0
	}
}

func (t2s *OneTxToSend)WriteBytes(wr io.Writer) {
	btc.WriteVlen(wr, uint64(len(t2s.Data)))
	wr.Write(t2s.Data)

	btc.WriteVlen(wr, uint64(len(t2s.Spent)))
	binary.Write(wr, binary.LittleEndian, t2s.Spent[:])

	binary.Write(wr, binary.LittleEndian, t2s.Invsentcnt)
	binary.Write(wr, binary.LittleEndian, t2s.SentCnt)
	binary.Write(wr, binary.LittleEndian, uint32(t2s.Firstseen.Unix()))
	binary.Write(wr, binary.LittleEndian, uint32(t2s.Lastsent.Unix()))
	binary.Write(wr, binary.LittleEndian, t2s.Volume)
	binary.Write(wr, binary.LittleEndian, t2s.Fee)
	binary.Write(wr, binary.LittleEndian, t2s.SigopsCost)
	binary.Write(wr, binary.LittleEndian, t2s.VerifyTime)
	wr.Write([]byte{t2s.Own, t2s.Blocked, bool2byte(t2s.MemInputs), bool2byte(t2s.Final)})
}

func MempoolSave2() {
	if !common.CFG.TXPool.SaveOnDisk {
		os.Remove(common.GocoinHomeDir + MEMPOOL_FILE_NAME2)
		return
	}

	f, er := os.Create(common.GocoinHomeDir + MEMPOOL_FILE_NAME2)
	if er != nil {
		println(er.Error())
		return
	}

	fmt.Println("Saving", MEMPOOL_FILE_NAME2)
	wr := bufio.NewWriter(f)

	wr.Write(common.Last.Block.BlockHash.Hash[:])

	btc.WriteVlen(wr, uint64(len(TransactionsToSend)))
	for _, t2s := range TransactionsToSend {
		t2s.WriteBytes(wr)
	}

	btc.WriteVlen(wr, uint64(len(SpentOutputs)))
	for k, v := range SpentOutputs {
		binary.Write(wr, binary.LittleEndian, k)
		binary.Write(wr, binary.LittleEndian, v)
	}

	wr.Write(END_MARKER[:])
	wr.Flush()
	f.Close()
}

func SubmitTrustedTx(tx *btc.Tx, rawtx []byte) bool {
	return HandleNetTx(&TxRcvd{tx: tx, raw: rawtx, trusted: true}, true)
}

/* Old slow method
func MempoolLoad1() bool {
	var ha [32]byte
	var totcnt, txlen uint64
	var rawtx []byte
	var tx *btc.Tx
	f, er := os.Open(common.GocoinHomeDir + MEMPOOL_FILE_NAME)
	if er != nil {
		println(er.Error())
		return false
	}
	defer f.Close()

	fmt.Println("Loading mempool from", MEMPOOL_FILE_NAME)

	rd := bufio.NewReader(f)
	er = btc.ReadAll(rd, ha[:])
	if er != nil {
		println(er.Error())
		return false
	}
	if !bytes.Equal(ha[:], common.Last.Block.BlockHash.Hash[:]) {
		println(MEMPOOL_FILE_NAME, "is for different last block hash")
		return false
	}

	totcnt, er = btc.ReadVLen(rd)
	if er != nil {
		println(er.Error())
		return false
	}



	for ; totcnt > 0; totcnt-- {
		txlen, er = btc.ReadVLen(rd)
		if er != nil {
			println(er.Error())
			return false
		}
		rawtx = make([]byte, int(txlen))
		er = btc.ReadAll(rd, rawtx)
		if er != nil {
			println(er.Error())
			return false
		}
		tx, _ = btc.NewTx(rawtx)
		if tx == nil {
			println("Error parsing tx from", MEMPOOL_FILE_NAME)
			return false
		}
		tx.SetHash(rawtx)
		SubmitTrustedTx(tx, rawtx)
	}

	er = btc.ReadAll(rd, ha[:len(END_MARKER)])
	if er != nil {
		println(er.Error())
		return false
	}
	if !bytes.Equal(ha[:len(END_MARKER)], END_MARKER) {
		println(MEMPOOL_FILE_NAME, "has marker missing")
		return false
	}

	fmt.Println(len(TransactionsToSend), "transactions loaded from", MEMPOOL_FILE_NAME)

	return true
}
*/

func MempoolLoad2() bool {
	var t2s *OneTxToSend
	var totcnt, le uint64
	var tmp [32]byte
	var bi BIDX
	var tina uint32
	var i int

	f, er := os.Open(common.GocoinHomeDir + MEMPOOL_FILE_NAME2)
	if er != nil {
		fmt.Println("MempoolLoad:", er.Error())
		return false
	}
	defer f.Close()

	fmt.Println("Loading mempool from", MEMPOOL_FILE_NAME2)

	rd := bufio.NewReader(f)
	if er = btc.ReadAll(rd, tmp[:32]); er != nil {
		goto fatal_error
	}
	if !bytes.Equal(tmp[:32], common.Last.Block.BlockHash.Hash[:]) {
		er = errors.New(MEMPOOL_FILE_NAME2 + " is for different last block hash")
		goto fatal_error
	}

	if totcnt, er = btc.ReadVLen(rd); er != nil {
		goto fatal_error
	}

	TransactionsToSend = make(map[BIDX]*OneTxToSend, int(totcnt))
	for ; totcnt > 0; totcnt-- {
		le, er = btc.ReadVLen(rd)
		if er != nil {
			goto fatal_error
		}

		t2s = new(OneTxToSend)
		t2s.Data = make([]byte, int(le))

		er = btc.ReadAll(rd, t2s.Data)
		if er != nil {
			goto fatal_error
		}

		t2s.Tx, i = btc.NewTx(t2s.Data)
		if t2s.Tx == nil || i != len(t2s.Data) {
			er = errors.New(fmt.Sprint("Error parsing tx from ", MEMPOOL_FILE_NAME2, " at idx", len(TransactionsToSend)))
			goto fatal_error
		}
		t2s.Tx.SetHash(t2s.Data)

		le, er = btc.ReadVLen(rd)
		if er != nil {
			goto fatal_error
		}
		t2s.Spent = make([]uint64, int(le))
		if er = binary.Read(rd, binary.LittleEndian, t2s.Spent[:]); er != nil {
			goto fatal_error
		}

		if er = binary.Read(rd, binary.LittleEndian, &t2s.Invsentcnt); er != nil {
			goto fatal_error
		}

		if er = binary.Read(rd, binary.LittleEndian, &t2s.SentCnt); er != nil {
			goto fatal_error
		}

		if er = binary.Read(rd, binary.LittleEndian, &tina); er != nil {
			goto fatal_error
		}
		t2s.Firstseen = time.Unix(int64(tina), 0)

		if er = binary.Read(rd, binary.LittleEndian, &tina); er != nil {
			goto fatal_error
		}
		t2s.Lastsent = time.Unix(int64(tina), 0)

		if er = binary.Read(rd, binary.LittleEndian, &t2s.Volume); er != nil {
			goto fatal_error
		}

		if er = binary.Read(rd, binary.LittleEndian, &t2s.Fee); er != nil {
			goto fatal_error
		}

		if er = binary.Read(rd, binary.LittleEndian, &t2s.SigopsCost); er != nil {
			goto fatal_error
		}

		if er = binary.Read(rd, binary.LittleEndian, &t2s.VerifyTime); er != nil {
			goto fatal_error
		}

		if er = btc.ReadAll(rd, tmp[:4]); er != nil {
			goto fatal_error
		}
		t2s.Own = tmp[0]
		t2s.Blocked = tmp[1]
		t2s.MemInputs = tmp[2] != 0
		t2s.Final = tmp[3] != 0

		t2s.Tx.Fee = t2s.Fee

		TransactionsToSend[t2s.Hash.BIdx()] = t2s
		TransactionsToSendSize += uint64(len(t2s.Data))
	}

	if totcnt, er = btc.ReadVLen(rd); er != nil {
		goto fatal_error
	}

	SpentOutputs = make(map[uint64]BIDX, int(totcnt))
	for ; totcnt > 0; totcnt-- {
		if er = binary.Read(rd, binary.LittleEndian, &le); er != nil {
			goto fatal_error
		}

		if er = binary.Read(rd, binary.LittleEndian, &bi); er != nil {
			goto fatal_error
		}

		SpentOutputs[le] = bi
	}

	if er = btc.ReadAll(rd, tmp[:len(END_MARKER)]); er != nil {
		goto fatal_error
	}
	if !bytes.Equal(tmp[:len(END_MARKER)], END_MARKER) {
		er = errors.New(MEMPOOL_FILE_NAME2 + " has marker missing")
		goto fatal_error
	}

	fmt.Println(len(TransactionsToSend), "transactions loaded from", MEMPOOL_FILE_NAME2)

	return true

fatal_error:
	fmt.Println("Error loading", MEMPOOL_FILE_NAME2, ":", er.Error())
	TransactionsToSend = make(map[BIDX]*OneTxToSend)
	TransactionsToSendSize = 0
	SpentOutputs = make(map[uint64]BIDX)
	return false
}




// This function is called for each tx mined in a new block
func tx_mined(tx *btc.Tx) (wtg *OneWaitingList) {
	h := tx.Hash
	if rec, ok := TransactionsToSend[h.BIdx()]; ok {
		common.CountSafe("TxMinedToSend")
		DeleteToSend(rec)
	}
	if _, ok := TransactionsRejected[h.BIdx()]; ok {
		common.CountSafe("TxMinedRejected")
		deleteRejected(h.BIdx())
	}
	if _, ok := TransactionsPending[h.BIdx()]; ok {
		common.CountSafe("TxMinedPending")
		delete(TransactionsPending, h.BIdx())
	}

	// Go through all the inputs and make sure we are not leaving them in SpentOutputs
	for i := range tx.TxIn {
		idx := tx.TxIn[i].Input.UIdx()
		if val, ok := SpentOutputs[idx]; ok {
			if rec, _ := TransactionsToSend[val]; rec != nil {
				if rec.Own != 0 {
					common.CountSafe("TxMinedMalleabled")
					fmt.Println("Input from own ", rec.Tx.Hash.String(), " mined in ", tx.Hash.String())
				} else {
					common.CountSafe("TxMinedOtherSpend")
				}
				DeleteToSend(rec)
			} else {
				common.CountSafe("TxMinedSpentERROR")
				fmt.Println("WTF? Input from ", rec.Tx.Hash.String(), " in mem-spent, but tx not in the mem-pool")
			}
			delete(SpentOutputs, idx)
		}
	}

	wtg = WaitingForInputs[h.BIdx()]
	return
}



// Removes all the block's tx from the mempool
func BlockMined(bl *btc.Block) {
	wtgs := make([]*OneWaitingList, len(bl.Txs)-1)
	var wtg_cnt int
	TxMutex.Lock()
	for i := 1; i < len(bl.Txs); i++ {
		wtg := tx_mined(bl.Txs[i])
		if wtg != nil {
			wtgs[wtg_cnt] = wtg
			wtg_cnt++
		}
	}
	TxMutex.Unlock()

	// Try to redo waiting txs
	if wtg_cnt > 0 {
		common.CountSafeAdd("TxMinedGotInput", uint64(wtg_cnt))
		for _, wtg := range wtgs[:wtg_cnt] {
			RetryWaitingForInput(wtg)
		}
	}
}
