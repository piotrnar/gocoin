package rpcapi

import (
	"sort"
	"time"
	"encoding/hex"
	"fmt"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
)

const MAX_TXS_LEN = 999e3 // 999KB, with 1KB margin to not exceed 1MB with conibase

type OneTransaction struct {
	Data string `json:"data"`
	Hash string `json:"hash"`
	Depends []uint `json:"depends"`
	Fee uint64 `json:"fee"`
	Sigops uint64 `json:"sigops"`
}

type GetBlockTemplateResp struct {
	Capabilities []string `json:"capabilities"`
	Version uint32 `json:"version"`
	PreviousBlockHash string `json:"previousblockhash"`
	Transactions []OneTransaction `json:"transactions"`
	Coinbaseaux struct {
		Flags string `json:"flags"`
	} `json:"coinbaseaux"`
	Coinbasevalue uint64 `json:"coinbasevalue"`
	Longpollid string `json:"longpollid"`
	Target string `json:"target"`
	Mintime uint `json:"mintime"`
	Mutable []string `json:"mutable"`
	Noncerange string `json:"noncerange"`
	Sigoplimit uint `json:"sigoplimit"`
	Sizelimit uint `json:"sizelimit"`
	Curtime uint `json:"curtime"`
	Bits string `json:"bits"`
	Height uint `json:"height"`
}

type RpcGetBlockTemplateResp struct {
	Id interface{} `json:"id"`
	Result GetBlockTemplateResp `json:"result"`
	Error interface{} `json:"error"`
}

func GetNextBlockTemplate(r *GetBlockTemplateResp) {
	var zer [32]byte

	common.Last.Mutex.Lock()

	r.Curtime = uint(time.Now().Unix())
	r.Mintime = uint(common.Last.Block.GetMedianTimePast()) + 1
	if r.Curtime < r.Mintime {
		r.Curtime = r.Mintime
	}
	height := common.Last.Block.Height+1
	bits := common.BlockChain.GetNextWorkRequired(common.Last.Block, uint32(r.Curtime))
	target := btc.SetCompact(bits).Bytes()

	r.Capabilities = []string{"proposal"}
	r.Version = 4
	r.PreviousBlockHash = common.Last.Block.BlockHash.String()
	r.Transactions, r.Coinbasevalue = GetTransactions(height, uint32(r.Mintime))
	r.Coinbasevalue += btc.GetBlockReward(height)
	r.Coinbaseaux.Flags = ""
	r.Longpollid = r.PreviousBlockHash
	r.Target = hex.EncodeToString(append(zer[:32-len(target)], target...))
	r.Mutable = []string{"time","transactions","prevblock"}
	r.Noncerange = "00000000ffffffff"
	r.Sigoplimit = btc.MAX_BLOCK_SIGOPS_COST / btc.WITNESS_SCALE_FACTOR
	r.Sizelimit = 1e6
	r.Bits = fmt.Sprintf("%08x", bits)
	r.Height = uint(height)

	last_given_time = uint32(r.Curtime)
	last_given_mintime = uint32(r.Mintime)

	common.Last.Mutex.Unlock()
}



/* memory pool transaction sorting stuff */
type one_mining_tx struct {
	*network.OneTxToSend
	depends []uint
	startat int
}

type sortedTxList []*one_mining_tx
func (tl sortedTxList) Len() int {return len(tl)}
func (tl sortedTxList) Swap(i, j int)      { tl[i], tl[j] = tl[j], tl[i] }
func (tl sortedTxList) Less(i, j int) bool { return tl[j].Fee < tl[i].Fee }


var txs_so_far map[[32]byte] uint
var totlen int
var sigops uint64

func get_next_tranche_of_txs(height, timestamp uint32) (res sortedTxList) {
	var unsp *btc.TxOut
	var all_inputs_found bool
	for _, v := range network.TransactionsToSend {
		tx := v.Tx

		if _, ok := txs_so_far[tx.Hash.Hash]; ok {
			continue
		}

		if !tx.IsFinal(height, timestamp) {
			continue
		}

		if totlen+len(v.Data) > 1e6 {
			//println("Too many txs - limit to 999000 bytes")
			return
		}
		totlen += len(v.Data)

		if sigops + v.SigopsCost > btc.MAX_BLOCK_SIGOPS_COST {
			//println("Too many sigops - limit to 999000 bytes")
			return
		}
		sigops += v.SigopsCost

		all_inputs_found = true
		var depends []uint
		for i := range tx.TxIn {
			unsp, _ = common.BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
			if unsp==nil {
				// not found in the confirmed blocks
				// check if txid is in txs_so_far
				if idx, ok := txs_so_far[tx.TxIn[i].Input.Hash]; !ok {
					// also not in txs_so_far
					all_inputs_found = false
					break
				} else {
					depends = append(depends, idx)
				}
			}
		}

		if all_inputs_found {
			res = append(res, &one_mining_tx{OneTxToSend:v, depends:depends, startat:1+len(txs_so_far)})
		}
	}
	return
}

func GetTransactions(height, timestamp uint32) (res []OneTransaction, totfees uint64) {

	network.TxMutex.Lock()
	defer network.TxMutex.Unlock()

	var cnt int
	var sorted sortedTxList
	txs_so_far = make(map[[32]byte]uint)
	totlen = 0
	sigops = 0
	//println("\ngetting txs from the pool of", len(network.TransactionsToSend), "...")
	for {
		new_piece := get_next_tranche_of_txs(height, timestamp)
		if new_piece.Len()==0 {
			break
		}
		//println("adding another", len(new_piece))
		sort.Sort(new_piece)

		for i:=0; i<len(new_piece); i++ {
			txs_so_far[new_piece[i].Tx.Hash.Hash] = uint(1+len(sorted)+i)
		}

		sorted = append(sorted, new_piece...)
	}
	/*if len(txs_so_far)!=len(network.TransactionsToSend) {
		println("ERROR: txs_so_far len", len(txs_so_far), " - please report!")
	}*/
	txs_so_far = nil // leave it for the garbage collector

	res = make([]OneTransaction, len(sorted))
	for cnt=0; cnt<len(sorted); cnt++ {
		v := sorted[cnt]
		res[cnt].Data = hex.EncodeToString(v.Data)
		res[cnt].Hash = v.Tx.Hash.String()
		res[cnt].Fee = v.Fee
		res[cnt].Sigops = v.SigopsCost
		res[cnt].Depends = v.depends
		totfees += v.Fee
		//println("", cnt+1, v.Tx.Hash.String(), "  turn:", v.startat, "  spb:", int(v.Fee)/len(v.Data), "  depend:", fmt.Sprint(v.depends))
	}

	//println("returning transacitons:", totlen, len(res))
	return
}
