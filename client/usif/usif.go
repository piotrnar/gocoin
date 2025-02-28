package usif

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"math/rand"
	"sync"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/peersdb"
	"github.com/piotrnar/gocoin/client/txpool"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/qdb"
	"github.com/piotrnar/gocoin/lib/others/rawtxlib"
	"github.com/piotrnar/gocoin/lib/others/sys"
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

	FetchingBalances sys.SyncBool
	Exit_now         sys.SyncBool
	Restart          sys.SyncBool
)

func getpo(prevout *btc.TxPrevOut) (po *btc.TxOut) {
	inpid := btc.NewUint256(prevout.Hash[:])
	bidx := inpid.BIdx()
	var tx *btc.Tx
	txpool.TxMutex.Lock()
	if t2s, ok := txpool.TransactionsToSend[bidx]; ok {
		tx = t2s.Tx
	} else if txr, ok := txpool.TransactionsRejected[bidx]; ok {
		tx = txr.Tx
	}
	txpool.TxMutex.Unlock()
	if tx != nil {
		if int(prevout.Vout) >= len(tx.TxOut) {
			println("ERROR: Vout TOO BIG (%d/%d)!", int(prevout.Vout), len(tx.TxOut))
		} else {
			po = tx.TxOut[prevout.Vout]
		}
	} else {
		po = common.BlockChain.Unspent.UnspentGet(prevout)
	}
	return
}

func DecodeTx(o io.Writer, tx *btc.Tx) {
	totinp, totout, missinginp := rawtxlib.Decode(o, tx, getpo, common.Testnet, false)
	if missinginp == 0 {
		fee_spb := float64(totinp-totout) / float64(tx.VSize())
		avg_fee := GetAverageFee()
		if fee_spb > 2*avg_fee {
			fmt.Fprintf(o, "WARNING: High fee SPB of %.02f (vs %.02f average).\n", fee_spb, avg_fee)
		}
	}
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

	wb := new(bytes.Buffer)
	DecodeTx(wb, tx)
	s = wb.String()

	txpool.TxMutex.Lock()
	txpool.DeleteRejectedByIdx(tx.Hash.BIdx(), false) // in case we rejected it eariler, to try it again as trusted
	txpool.TxMutex.Unlock()

	if why := txpool.NeedThisTxExt(&tx.Hash, nil); why != 0 {
		s += fmt.Sprintln("Transaction not needed or not wanted", why)
		txpool.TxMutex.Lock()
		if t2s := txpool.TransactionsToSend[tx.Hash.BIdx()]; t2s != nil {
			t2s.Local = true // make as own (if not needed)
		}
		txpool.TxMutex.Unlock()
		return
	}

	if !txpool.SubmitLocalTx(tx, txd) {
		txpool.TxMutex.Lock()
		rr := txpool.TransactionsRejected[tx.Hash.BIdx()]
		txpool.TxMutex.Unlock()
		if rr != nil {
			s += fmt.Sprintln("Transaction rejected", rr.Reason)
		} else {
			s += fmt.Sprintln("Transaction rejected in a weird way")
		}
		return
	}

	txpool.TxMutex.Lock()
	_, ok := txpool.TransactionsToSend[tx.Hash.BIdx()]
	txpool.TxMutex.Unlock()
	if ok {
		s += fmt.Sprintln("Transaction added to the memory pool. You can broadcast it now.")
	} else {
		s += fmt.Sprintln("Transaction not rejected, but also not accepted - very strange!")
	}

	return
}

func SendInvToRandomPeer(typ uint32, h *btc.Uint256) {
	common.CountSafePar("NetSendOneInv-", typ)

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
	if FetchingBalances.Get() {
		fmt.Println("Client is currently busy fetching wallet balance.\nYour command has been queued and will execute soon.")
	}

	//fmt.Println("main.go last seen in line", common.BusyIn())
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

func MemoryPoolFees() (res string) {
	res = fmt.Sprintln("Content of mempool sorted by fee's SPB:")
	txpool.TxMutex.Lock()
	defer txpool.TxMutex.Unlock()

	sorted := txpool.GetMempoolFees(txpool.TransactionsToSendWeight)

	var totlen uint64
	var txcnt int
	for cnt, v := range sorted {
		newlen := totlen + uint64(v.Weight)
		txcnt += len(v.Txs)
		if cnt == 0 || cnt+1 == len(sorted) || (newlen/400e3) != (totlen/400e3) {
			res += fmt.Sprintf(" up to %9d weight: %6d txs @ %9d SPKB\n", newlen, txcnt, 4000*v.Fee/v.Weight)
		}
		if (newlen / 4e6) != (totlen / 4e6) {
			res += "======================================================\n"
		}
		totlen = newlen
	}
	return
}

// UnbanPeer unbans peer of a given IP or "all" banned peers
func UnbanPeer(par string) (s string) {
	var ad *peersdb.PeerAddr

	if par != "all" {
		var er error
		ad, er = peersdb.NewAddrFromString(par, false)
		if er != nil {
			s = fmt.Sprintln(par, er.Error())
			return
		}
		s += fmt.Sprintln("Unban", ad.Ip(), "...")
		network.HammeringMutex.Lock()
		delete(network.RecentlyDisconencted, ad.Ip4)
		network.HammeringMutex.Unlock()
	} else {
		s += fmt.Sprintln("Unban all peers ...")
		network.HammeringMutex.Lock()
		network.RecentlyDisconencted = make(map[[4]byte]*network.RecentlyDisconenctedType)
		network.HammeringMutex.Unlock()
	}

	var keys []qdb.KeyType
	var vals [][]byte
	peersdb.PeerDB.Browse(func(k qdb.KeyType, v []byte) uint32 {
		peer := peersdb.NewPeer(v)
		if peer.Banned != 0 {
			if ad == nil || peer.Ip() == ad.Ip() {
				s += fmt.Sprintln(" -", peer.NetAddr.String())
				peer.Banned = 0
				keys = append(keys, k)
				vals = append(vals, peer.Bytes())
			}
		}
		return 0
	})
	for i := range keys {
		peersdb.PeerDB.Put(keys[i], vals[i])
	}

	s += fmt.Sprintln(len(keys), "peer(s) un-baned")
	return
}

func GetReceivedBlockX(block *btc.Block) (rb *network.OneReceivedBlock, cbasetx *btc.Tx) {
	network.MutexRcv.Lock()
	rb = network.ReceivedBlocks[block.Hash.BIdx()]
	if len(block.Txs) == 0 {
		block.BuildTxListExt(false) // we will not need txId's here
	}
	if rb.BlockUserInfo == nil {
		rb.BlockUserInfo = block.GetUserInfo()
	}
	cbasetx = block.Txs[0]
	network.MutexRcv.Unlock()
	return
}
