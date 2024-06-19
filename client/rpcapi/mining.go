package rpcapi

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/lib/btc"
)

const MAX_TXS_LEN = 999e3 // 999KB, with 1KB margin to not exceed 1MB with conibase

type OneTransaction struct {
	Data    string `json:"data"`
	Hash    string `json:"hash"`
	Depends []uint `json:"depends"`
	Fee     uint64 `json:"fee"`
	Sigops  uint64 `json:"sigops"`
}

type GetBlockTemplateResp struct {
	Capabilities      []string         `json:"capabilities"`
	Version           uint32           `json:"version"`
	PreviousBlockHash string           `json:"previousblockhash"`
	Transactions      []OneTransaction `json:"transactions"`
	Coinbaseaux       struct {
		Flags string `json:"flags"`
	} `json:"coinbaseaux"`
	Coinbasevalue uint64   `json:"coinbasevalue"`
	Longpollid    string   `json:"longpollid"`
	Target        string   `json:"target"`
	Mintime       uint     `json:"mintime"`
	Mutable       []string `json:"mutable"`
	Noncerange    string   `json:"noncerange"`
	Sigoplimit    uint     `json:"sigoplimit"`
	Sizelimit     uint     `json:"sizelimit"`
	Curtime       uint     `json:"curtime"`
	Bits          string   `json:"bits"`
	Height        uint     `json:"height"`
}

type GetWorkTemplateResp struct {
	Data   string `json:"data"`
	Target string `json:"target"`
}

type GetMiningInfoResp struct {
	Blocks     uint   `json:"blocks"`
	Difficulty uint   `json:"difficulty"`
	Chain      string `json:"target"`
}

type RpcGetBlockTemplateResp struct {
	Id     interface{}          `json:"id"`
	Result GetBlockTemplateResp `json:"result"`
	Error  interface{}          `json:"error"`
}

type RpcGetWorkResp struct {
	Id     interface{}         `json:"id"`
	Result GetWorkTemplateResp `json:"result"`
	Error  interface{}         `json:"error"`
}

type RpcGetMiningInfoResp struct {
	Id     interface{}       `json:"id"`
	Result GetMiningInfoResp `json:"result"`
	Error  interface{}       `json:"error"`
}

var curr_block *btc.Block

var the_pk_script []byte

func init() {
	the_addr, _ := btc.NewAddrFromString("n2ASs8pUXUMxnkNjek6H7PTHfjnvre7QfQ")
	the_pk_script = the_addr.OutScript()
}

func make_coinbase_tx(height uint32) (tx *btc.Tx) {
	tx = new(btc.Tx)
	tx.TxIn = make([]*btc.TxIn, 1)
	tx.TxIn[0] = new(btc.TxIn)
	/*
		var null_32 [32]byte
		tx.SegWit = make([][][]byte, 1)
		tx.SegWit[0] = make([][]byte, 1)
		tx.SegWit[0][0] = null_32[:]
	*/

	var exp [6]byte
	var exp_len int
	if height <= 16 {
		exp[0] = btc.OP_1 - 1 + byte(height)
		exp_len = 1
	} else {
		binary.LittleEndian.PutUint32(exp[1:5], height)
		for exp_len = 5; exp_len > 1; exp_len-- {
			if exp[exp_len] != 0 || exp[exp_len-1] >= 0x80 {
				break
			}
		}
		exp[0] = byte(exp_len)
		exp_len++
	}
	tx.TxIn[0].ScriptSig = exp[:exp_len]
	tx.TxIn[0].Sequence = 0xffffffff

	tx.TxOut = make([]*btc.TxOut, 1)
	tx.TxOut[0] = new(btc.TxOut)
	tx.TxOut[0].Value = 50e8
	tx.TxOut[0].Pk_script = the_pk_script

	/*
		tx.TxOut[1] = new(btc.TxOut)
		merkle, _ := btc.GetWitnessMerkle([]*btc.Tx{tx})
		with_nonce := btc.Sha2Sum(append(merkle, tx.SegWit[0][0]...))
		tx.TxOut[1].Pk_script = append([]byte{0x6a, 0x24, 0xaa, 0x21, 0xa9, 0xed}, with_nonce[:]...)
	*/
	return
}

func GetWork(r *RpcGetWorkResp) {
	curr_block = new(btc.Block)
	common.Last.Mutex.Lock()
	curr_block.Txs = make([]*btc.Tx, 1)
	curr_block.Txs[0] = make_coinbase_tx(common.Last.Block.Height + 1)
	raw_tx := curr_block.Txs[0].SerializeNew()
	curr_block.Txs[0].SetHash(raw_tx)
	merkle, _ := curr_block.GetMerkle()

	var zer [32]byte
	var data [80]byte
	now := uint32(time.Now().Unix()) + 10*60
	bits := common.BlockChain.GetNextWorkRequired(common.Last.Block, now)
	//fmt.Printf("BitsA:0x%08x  %d >? %d\n", bits, now, common.Last.Block.Timestamp()+chain.TargetSpacing*2)
	common.Last.Mutex.Unlock()

	target := btc.SetCompact(bits).Bytes()
	r.Result.Target = hex.EncodeToString(append(zer[:32-len(target)], target...))
	binary.LittleEndian.PutUint32(data[0:4], 0x20000000)
	copy(data[4:36], common.Last.Block.BlockHash.Hash[:])
	copy(data[36:36+32], merkle)
	binary.LittleEndian.PutUint32(data[68:72], now)
	binary.LittleEndian.PutUint32(data[72:76], bits)
	// data[76:80]  - nonce
	r.Result.Data = hex.EncodeToString(data[:])
}

func GetNextBlockTemplate(r *GetBlockTemplateResp) {
	var zer [32]byte

	common.Last.Mutex.Lock()

	//r.Curtime = uint(common.Last.Block.Timestamp()) + 10*60
	r.Curtime = uint(time.Now().Unix()) + 10*60
	if now := uint(common.Last.Block.Timestamp()); now > r.Curtime {
		r.Curtime = now
	}
	r.Mintime = uint(common.Last.Block.GetMedianTimePast()) + 1
	if r.Curtime < r.Mintime {
		r.Curtime = r.Mintime
	}
	height := common.Last.Block.Height + 1
	bits := common.BlockChain.GetNextWorkRequired(common.Last.Block, uint32(r.Curtime))
	//fmt.Printf("BitsB:0x%08x\n", bits)
	target := btc.SetCompact(bits).Bytes()

	r.Capabilities = []string{"proposal"}
	r.Version = 4
	r.PreviousBlockHash = common.Last.Block.BlockHash.String()
	r.Transactions, r.Coinbasevalue = GetTransactions(height, uint32(r.Mintime))
	r.Coinbasevalue += btc.GetBlockReward(height)
	r.Coinbaseaux.Flags = ""
	r.Longpollid = r.PreviousBlockHash
	r.Target = hex.EncodeToString(append(zer[:32-len(target)], target...))
	r.Mutable = []string{"time", "transactions", "prevblock"}
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

func (tl sortedTxList) Len() int           { return len(tl) }
func (tl sortedTxList) Swap(i, j int)      { tl[i], tl[j] = tl[j], tl[i] }
func (tl sortedTxList) Less(i, j int) bool { return tl[j].Fee < tl[i].Fee }

var txs_so_far map[[32]byte]uint
var totlen int
var sigops uint64

func get_next_tranche_of_txs(height, timestamp uint32) (res sortedTxList) {
	var unsp *btc.TxOut
	var all_inputs_found bool
	for _, v := range network.TransactionsToSend {
		tx := v.Tx

		if tx.SegWit != nil { // testing on bfgminer 5.5.0 that cannot deal with segwit blocks
			continue
		}

		if _, ok := txs_so_far[tx.Hash.Hash]; ok {
			continue
		}

		if !tx.IsFinal(height, timestamp) {
			continue
		}

		if totlen+len(v.Raw) > 1e6 {
			//println("Too many txs - limit to 999000 bytes")
			return
		}
		totlen += len(v.Raw)

		if sigops+v.SigopsCost > btc.MAX_BLOCK_SIGOPS_COST {
			//println("Too many sigops - limit to 999000 bytes")
			return
		}
		sigops += v.SigopsCost

		all_inputs_found = true
		var depends []uint
		for i := range tx.TxIn {
			unsp = common.BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
			if unsp == nil {
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
			res = append(res, &one_mining_tx{OneTxToSend: v, depends: depends, startat: 1 + len(txs_so_far)})
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
		if new_piece.Len() == 0 {
			break
		}
		//println("adding another", len(new_piece))
		sort.Sort(new_piece)

		for i := 0; i < len(new_piece); i++ {
			txs_so_far[new_piece[i].Tx.Hash.Hash] = uint(1 + len(sorted) + i)
		}

		sorted = append(sorted, new_piece...)
	}
	/*if len(txs_so_far)!=len(network.TransactionsToSend) {
		println("ERROR: txs_so_far len", len(txs_so_far), " - please report!")
	}*/
	txs_so_far = nil // leave it for the garbage collector

	res = make([]OneTransaction, len(sorted))
	for cnt = 0; cnt < len(sorted); cnt++ {
		v := sorted[cnt]
		res[cnt].Data = hex.EncodeToString(v.Raw)
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
