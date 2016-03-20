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
	Depends []int `json:"depends"`
	Fee uint64 `json:"fee"`
	Sigops uint `json:"sigops"`
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
	height := common.Last.Block.Height+1
	bits := common.BlockChain.GetNextWorkRequired(common.Last.Block, uint32(r.Curtime))
	target := btc.SetCompact(bits).Bytes()

	r.Capabilities = []string{"proposal"}
	r.Version = 4
	r.PreviousBlockHash = common.Last.Block.BlockHash.String()
	r.Transactions, r.Coinbasevalue = GetTransactions()
	r.Coinbasevalue += btc.GetBlockReward(height)
	r.Coinbaseaux.Flags = ""
	r.Longpollid = r.PreviousBlockHash
	r.Target = hex.EncodeToString(append(zer[:32-len(target)], target...))
	r.Mutable = []string{"time","transactions","prevblock"}
	r.Noncerange = "00000000ffffffff"
	r.Sigoplimit = 20000
	r.Sizelimit = btc.MAX_BLOCK_SIZE
	r.Bits = fmt.Sprintf("%08x", bits)
	r.Height = uint(height)

	last_given_time = uint32(r.Curtime)
	last_given_mintime = uint32(r.Mintime)

	common.Last.Mutex.Unlock()
}



/* memory pool transaction sorting stuff */
type sortedTxList []*network.OneTxToSend
func (tl sortedTxList) Len() int {return len(tl)}
func (tl sortedTxList) Swap(i, j int)      { tl[i], tl[j] = tl[j], tl[i] }
func (tl sortedTxList) Less(i, j int) bool { return tl[j].Fee < tl[i].Fee }

func GetTransactions() (res []OneTransaction, totfees uint64) {
	network.TxMutex.Lock()
	defer network.TxMutex.Unlock()

	sorted := make(sortedTxList, len(network.TransactionsToSend))
	var cnt int
	for _, v := range network.TransactionsToSend {
		if v.MemInputs {
			continue // skip meminput txs for now
		}
		sorted[cnt] = v
		cnt++
	}
	sorted = sorted[:cnt]
	sort.Sort(sorted)

	res = make([]OneTransaction, len(sorted))
	totlen := 1000
	for cnt=0; cnt<len(sorted); cnt++ {
		v := sorted[cnt]

		if totlen+len(v.Data) > btc.MAX_BLOCK_SIZE {
			println("Too many txs - limit to 999000 bytes")
			res = res[:cnt]
			return
		}
		totlen += len(v.Data)

		res[cnt].Data = hex.EncodeToString(v.Data)
		res[cnt].Hash = v.Tx.Hash.String()
		//res[cnt].Depends
		res[cnt].Fee = v.Fee
		res[cnt].Sigops = v.Tx.Sigops
		totfees += v.Fee
	}
	return
}
