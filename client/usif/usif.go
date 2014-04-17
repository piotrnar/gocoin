package usif

import (
	"fmt"
	"time"
	"sync"
	"errors"
	"math/rand"
	"encoding/hex"
	"encoding/binary"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
)


type OneUiReq struct {
	Param string
	Handler func(pars string)
	Done sync.WaitGroup
}

var (
	UiChannel chan *OneUiReq = make(chan *OneUiReq, 1)

	Exit_now bool
	DefragBlocksDB bool
)


func DecodeTx(tx *btc.Tx) (s string, missinginp bool, totinp, totout uint64, e error) {
	s += fmt.Sprintln("Transaction details (for your information):")
	s += fmt.Sprintln(len(tx.TxIn), "Input(s):")
	for i := range tx.TxIn {
		s += fmt.Sprintf(" %3d %s", i, tx.TxIn[i].Input.String())
		var po *btc.TxOut

		if txinmem, ok := network.TransactionsToSend[tx.TxIn[i].Input.Hash]; ok {
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
			ok := btc.VerifyTxScript(tx.TxIn[i].ScriptSig, po.Pk_script, i, tx, true)
			if !ok {
				s += fmt.Sprintln("\nERROR: The transacion does not have a valid signature.")
				e = errors.New("Invalid signature")
				return
			}
			totinp += po.Value
			s += fmt.Sprintf(" %15.8f BTC @ %s\n", float64(po.Value)/1e8,
				btc.NewAddrFromPkScript(po.Pk_script, common.Testnet).String())
		} else {
			s += fmt.Sprintln(" - UNKNOWN INPUT")
			missinginp = true
		}
	}
	s += fmt.Sprintln(len(tx.TxOut), "Output(s):")
	for i := range tx.TxOut {
		totout += tx.TxOut[i].Value
		s += fmt.Sprintf(" %15.8f BTC to %s\n", float64(tx.TxOut[i].Value)/1e8,
			btc.NewAddrFromPkScript(tx.TxOut[i].Pk_script, common.Testnet).String())
	}
	if missinginp {
		s += fmt.Sprintln("WARNING: There are missing inputs and we cannot calc input BTC amount.")
		s += fmt.Sprintln("If there is somethign wrong with this transaction, you can loose money...")
	} else {
		s += fmt.Sprintf("All OK: %.8f BTC in -> %.8f BTC out, with %.8f BTC fee\n", float64(totinp)/1e8,
			float64(totout)/1e8, float64(totinp-totout)/1e8)
	}
	return
}

func LoadRawTx(buf []byte) (s string) {
	txd, er := hex.DecodeString(string(buf))
	if er != nil {
		txd = buf
	}

	// At this place we should have raw transaction in txd
	tx, le := btc.NewTx(txd)
	if tx==nil || le != len(txd) {
		s += fmt.Sprintln("Could not decode transaction file or it has some extra data")
		return
	}
	tx.Hash = btc.NewSha2Hash(txd)

	var missinginp bool
	var totinp, totout uint64
	s, missinginp, totinp, totout, er = DecodeTx(tx)
	if er != nil {
		return
	}

	network.TxMutex.Lock()
	if missinginp {
		network.TransactionsToSend[tx.Hash.Hash] = &network.OneTxToSend{Tx:tx, Data:txd, Own:2, Firstseen:time.Now(),
			Volume:totout}
	} else {
		network.TransactionsToSend[tx.Hash.Hash] = &network.OneTxToSend{Tx:tx, Data:txd, Own:1, Firstseen:time.Now(),
			Volume:totinp, Fee:totinp-totout}
	}
	network.TxMutex.Unlock()
	s += fmt.Sprintln("Transaction added to the memory pool. Please double check its details above.")
	s += fmt.Sprintln("If it does what you intended, you can send it the network.\nUse TxID:", tx.Hash.String())
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
		if idx==cnt {
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


func GetNetworkHashRate() string {
	hours := common.CFG.HashrateHours
	common.Last.Mutex.Lock()
	end := common.Last.Block
	common.Last.Mutex.Unlock()
	now := time.Now().Unix()
	cnt := 0
	var diff float64
	for ; end!=nil; cnt++ {
		if now-int64(end.Timestamp()) > int64(hours)*3600 {
			break
		}
		diff += btc.GetDifficulty(end.Bits())
		end = end.Parent
	}
	if cnt==0 {
		return "0"
	}
	diff /= float64(cnt)
	bph := float64(cnt)/float64(hours)
	return common.HashrateToString(bph/6 * diff * 7158278.826667)
}


func init() {
	rand.Seed(int64(time.Now().Nanosecond()))
}
