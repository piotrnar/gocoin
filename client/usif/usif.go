package usif

import (
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/lib/script"
	"math/rand"
	"sort"
	"sync"
	"time"
)

type OneUiReq struct {
	Param   string
	Handler func(pars string)
	Done    sync.WaitGroup
}

// A thread that wants to lock the main thread calls:
// In.Add(1); Out.Add(1); [put msg into LocksChan]; In.Wait(); [do synchronized code]; Out.Done()
// The main thread, upon receiving the message, does:
// In.Done(); Out.Wait();
type OneLock struct {
	In  sync.WaitGroup // main thread calls Done() on this one and then Stop.Wait()
	Out sync.WaitGroup // the synchronized thread calls Done
}

var (
	UiChannel chan *OneUiReq = make(chan *OneUiReq, 1)
	LocksChan chan *OneLock  = make(chan *OneLock, 1)

	Exit_now sys.SyncBool
)

func DecodeTxSops(tx *btc.Tx) (s string, missinginp bool, totinp, totout uint64, sigops uint, e error) {
	s += fmt.Sprintln("Transaction details (for your information):")
	s += fmt.Sprintln(len(tx.TxIn), "Input(s):")
	sigops = btc.WITNESS_SCALE_FACTOR * tx.GetLegacySigOpCount()
	for i := range tx.TxIn {
		s += fmt.Sprintf(" %3d %s", i, tx.TxIn[i].Input.String())
		var po *btc.TxOut

		inpid := btc.NewUint256(tx.TxIn[i].Input.Hash[:])
		if txinmem, ok := network.TransactionsToSend[inpid.BIdx()]; ok {
			s += fmt.Sprint(" mempool")
			if int(tx.TxIn[i].Input.Vout) >= len(txinmem.TxOut) {
				s += fmt.Sprintf(" - Vout TOO BIG (%d/%d)!", int(tx.TxIn[i].Input.Vout), len(txinmem.TxOut))
			} else {
				po = txinmem.TxOut[tx.TxIn[i].Input.Vout]
			}
		} else {
			po, _ = common.BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
			if po != nil {
				s += fmt.Sprintf("%8d", po.BlockHeight)
			}
		}
		if po != nil {
			ok := script.VerifyTxScript(po.Pk_script, po.Value, i, tx, script.VER_P2SH|script.VER_DERSIG|script.VER_CLTV)
			if !ok {
				s += fmt.Sprintln("\nERROR: The transacion does not have a valid signature.")
				e = errors.New("Invalid signature")
			}
			totinp += po.Value

			ads := "???"
			if ad := btc.NewAddrFromPkScript(po.Pk_script, common.Testnet); ad != nil {
				ads = ad.String()
			}
			s += fmt.Sprintf(" %15.8f BTC @ %s", float64(po.Value)/1e8, ads)

			if btc.IsP2SH(po.Pk_script) {
				so := btc.WITNESS_SCALE_FACTOR * btc.GetP2SHSigOpCount(tx.TxIn[i].ScriptSig)
				s += fmt.Sprintf("  + %d sigops", so)
				sigops += so
			}

			swo := tx.CountWitnessSigOps(i, po.Pk_script)
			if swo > 0 {
				s += fmt.Sprintf("  + %d segops", swo)
				sigops += swo
			}

			s += "\n"
		} else {
			s += fmt.Sprintln(" - UNKNOWN INPUT")
			missinginp = true
		}
	}
	s += fmt.Sprintln(len(tx.TxOut), "Output(s):")
	for i := range tx.TxOut {
		totout += tx.TxOut[i].Value
		adr := btc.NewAddrFromPkScript(tx.TxOut[i].Pk_script, common.Testnet)
		if adr != nil {
			s += fmt.Sprintf(" %15.8f BTC to adr %s\n", float64(tx.TxOut[i].Value)/1e8, adr.String())
		} else {
			s += fmt.Sprintf(" %15.8f BTC to scr %s\n", float64(tx.TxOut[i].Value)/1e8, hex.EncodeToString(tx.TxOut[i].Pk_script))
		}
	}
	if missinginp {
		s += fmt.Sprintln("WARNING: There are missing inputs and we cannot calc input BTC amount.")
		s += fmt.Sprintln("If there is somethign wrong with this transaction, you can loose money...")
	} else {
		s += fmt.Sprintf("All OK: %.8f BTC in -> %.8f BTC out, with %.8f BTC fee\n", float64(totinp)/1e8,
			float64(totout)/1e8, float64(totinp-totout)/1e8)
	}

	s += fmt.Sprintln("ECDSA sig operations : ", sigops)

	return
}

func DecodeTx(tx *btc.Tx) (s string, missinginp bool, totinp, totout uint64, e error) {
	s, missinginp, totinp, totout, _, e = DecodeTxSops(tx)
	return
}

func LoadRawTx(buf []byte) (s string) {
	txd, er := hex.DecodeString(string(buf))
	if er != nil {
		txd = buf
	}

	// At this place we should have raw transaction in txd
	tx, le := btc.NewTx(txd)
	if tx == nil || le != len(txd) {
		s += fmt.Sprintln("Could not decode transaction file or it has some extra data")
		return
	}
	tx.SetHash(txd)

	s, _, _, _, _ = DecodeTx(tx)

	network.RemoveFromRejected(tx.Hash) // in case we rejected it eariler, to try it again as trusted

	if why := network.NeedThisTxExt(tx.Hash, nil); why != 0 {
		s += fmt.Sprintln("Transaction not needed or not wanted", why)
		network.TxMutex.Lock()
		if t2s := network.TransactionsToSend[tx.Hash.BIdx()]; t2s != nil {
			t2s.Own = 1 // make as own (if not needed)
		}
		network.TxMutex.Unlock()
		return
	}

	if !network.SubmitTrustedTx(tx, txd) {
		network.TxMutex.Lock()
		rr := network.TransactionsRejected[tx.Hash.BIdx()]
		network.TxMutex.Unlock()
		if rr != nil {
			s += fmt.Sprintln("Transaction rejected", rr.Reason)
		} else {
			s += fmt.Sprintln("Transaction rejected in a weird way")
		}
		return
	}

	network.TxMutex.Lock()
	t2s := network.TransactionsToSend[tx.Hash.BIdx()]
	network.TxMutex.Unlock()
	if t2s != nil {
		t2s.Own = 1
		s += fmt.Sprintln("Transaction added to the memory pool. You can broadcast it now.")
	} else {
		s += fmt.Sprintln("Transaction not rejected in a weird way.")
	}

	return
}

func SendInvToRandomPeer(typ uint32, h *btc.Uint256) {
	common.CountSafe(fmt.Sprint("NetSendOneInv", typ))

	// Prepare the inv
	inv := new([36]byte)
	binary.LittleEndian.PutUint32(inv[0:4], typ)
	copy(inv[4:36], h.Bytes())

	// Append it to PendingInvs in a random connection
	network.Mutex_net.Lock()
	idx := rand.Intn(len(network.OpenCons))
	var cnt int
	for _, v := range network.OpenCons {
		if idx == cnt {
			v.Mutex.Lock()
			v.PendingInvs = append(v.PendingInvs, inv)
			v.Mutex.Unlock()
			break
		}
		cnt++
	}
	network.Mutex_net.Unlock()
	return
}

func GetNetworkHashRateNum() float64 {
	hours := common.CFG.Stat.HashrateHrs
	common.Last.Mutex.Lock()
	end := common.Last.Block
	common.Last.Mutex.Unlock()
	now := time.Now().Unix()
	cnt := 0
	var diff float64
	for ; end != nil; cnt++ {
		if now-int64(end.Timestamp()) > int64(hours)*3600 {
			break
		}
		diff += btc.GetDifficulty(end.Bits())
		end = end.Parent
	}
	if cnt == 0 {
		return 0
	}
	diff /= float64(cnt)
	bph := float64(cnt) / float64(hours)
	return bph / 6 * diff * 7158278.826667
}

func ExecUiReq(req *OneUiReq) {
	common.Busy_mutex.Lock()
	if common.BusyWith != "" {
		fmt.Print("now common.BusyWith with ", common.BusyWith)
	}
	common.Busy_mutex.Unlock()
	fmt.Println("...")
	sta := time.Now().UnixNano()
	req.Done.Add(1)
	UiChannel <- req
	go func() {
		req.Done.Wait()
		sto := time.Now().UnixNano()
		fmt.Printf("Ready in %.3fs\n", float64(sto-sta)/1e9)
		fmt.Print("> ")
	}()
}

type SortedTxToSend []*network.OneTxToSend

func (tl SortedTxToSend) Len() int      { return len(tl) }
func (tl SortedTxToSend) Swap(i, j int) { tl[i], tl[j] = tl[j], tl[i] }
func (tl SortedTxToSend) Less(i, j int) bool {
	spb_i := float64(tl[i].Fee) / float64(tl[i].VSize())
	spb_j := float64(tl[j].Fee) / float64(tl[j].VSize())
	return spb_j < spb_i
}

func MemoryPoolFees() (res string) {
	res = fmt.Sprintln("Content of mempool sorted by fee's SPB:")
	cnt := 0
	network.TxMutex.Lock()
	defer network.TxMutex.Unlock()

	sorted := make(SortedTxToSend, len(network.TransactionsToSend))
	for _, v := range network.TransactionsToSend {
		sorted[cnt] = v
		cnt++
	}
	sort.Sort(sorted)

	var totlen uint64
	for cnt = 0; cnt < len(sorted); cnt++ {
		v := sorted[cnt]
		newlen := totlen + uint64(len(v.Data))

		if cnt == 0 || cnt+1 == len(sorted) || (newlen/100e3) != (totlen/100e3) {
			spb := float64(v.Fee) / float64(len(v.Data))
			toprint := newlen
			if cnt != 0 && cnt+1 != len(sorted) {
				toprint = newlen / 100e3 * 100e3
			}
			res += fmt.Sprintf(" %9d bytes, %6d txs @ fee %8.1f Satoshis / byte\n", toprint, cnt+1, spb)
		}
		if (newlen / 1e6) != (totlen / 1e6) {
			res += "===========================================================\n"
		}

		totlen = newlen
	}
	return
}

func init() {
	rand.Seed(int64(time.Now().Nanosecond()))
}
