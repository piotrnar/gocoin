package rpcapi

import (
	"bytes"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"sort"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/txpool"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/script"
)

var (
	DO_SEGWIT        = false   // set to false for old miners that use "getblocktemplate" but dont support segwit (e.gg. bfgminer 5.5.0)
	DO_NOT_SUBMIT    = false   // set it to true if you dont want to commit newly mined blocks to the blockchian/network
	WAIT_FOR_SECONDS = 19 * 60 // it will allow diff 1 one minute before the others
	COINBASE_ADDRESS = "tb1petwqkk8wnk4lgweyy4xvgxk0c7f572mrval39mwaxl34scex8zsqlpfgdd"
	COINBASE_STRING  = "/cipa/"

	currently_worked_block *btc.Block
	mining_info            GetMiningInfoResp
)

type OneTransaction struct {
	Data    string `json:"data"`
	Hash    string `json:"hash"`
	Depends []uint `json:"depends"`
	Fee     uint64 `json:"fee"`
	Sigops  uint64 `json:"sigops"`
}

type GetBlockTemplateResp struct {
	Bits              string `json:"bits"`
	PreviousBlockHash string `json:"previousblockhash"`
	Coinbaseaux       struct {
		Flags string `json:"flags"`
	} `json:"coinbaseaux"`
	Noncerange    string           `json:"noncerange"`
	Longpollid    string           `json:"longpollid"`
	Target        string           `json:"target"`
	Mutable       []string         `json:"mutable"`
	Transactions  []OneTransaction `json:"transactions"`
	Capabilities  []string         `json:"capabilities"`
	Mintime       uint             `json:"mintime"`
	Coinbasevalue uint64           `json:"coinbasevalue"`
	Sigoplimit    uint             `json:"sigoplimit"`
	Sizelimit     uint             `json:"sizelimit"`
	Curtime       uint             `json:"curtime"`
	Height        uint             `json:"height"`
	Version       uint32           `json:"version"`
}

type GetWorkTemplateResp struct {
	Data   string `json:"data"`
	Target string `json:"target"`
}

type GetMiningInfoResp struct {
	Blocks     uint    `json:"blocks"`
	Difficulty float64 `json:"difficulty"`
}

type RpcGetBlockTemplateResp struct {
	Id     interface{}          `json:"id"`
	Error  interface{}          `json:"error"`
	Result GetBlockTemplateResp `json:"result"`
}

type RpcGetWorkResp struct {
	Id     interface{}         `json:"id"`
	Error  interface{}         `json:"error"`
	Result GetWorkTemplateResp `json:"result"`
}

type RpcGetMiningInfoResp struct {
	Id     interface{}       `json:"id"`
	Error  interface{}       `json:"error"`
	Result GetMiningInfoResp `json:"result"`
}

func swap256(d []byte) {
	if len(d) != 32 {
		panic("swap256 call with wrong length")
	}
	for i := 0; i < 16; i++ {
		d[i], d[31-i] = d[31-i], d[i]
	}
}

func swap32(d []byte) {
	for i := 0; i < len(d); i += 4 {
		binary.BigEndian.PutUint32(d[i:i+4], binary.LittleEndian.Uint32(d[i:i+4]))
	}
}

func make_coinbase_tx(height uint32) (tx *btc.Tx) {
	tx = new(btc.Tx)
	tx.TxIn = make([]*btc.TxIn, 1)
	tx.TxIn[0] = new(btc.TxIn)
	tx.TxIn[0].Input.Vout = 0xffffffff

	// make the coinbase tx segwit type
	tx.SegWit = make([][][]byte, 1)
	tx.SegWit[0] = make([][]byte, 1)
	tx.SegWit[0][0] = make([]byte, 32)
	rand.Read(tx.SegWit[0][0])

	wr := bytes.NewBuffer(script.UintToScript(height))
	wr.Write([]byte{byte(len(COINBASE_STRING))})
	wr.Write([]byte(COINBASE_STRING))

	tx.TxIn[0].ScriptSig = wr.Bytes()
	tx.TxIn[0].Sequence = 0xffffffff

	the_addr, _ := btc.NewAddrFromString(COINBASE_ADDRESS)

	// add first output (for the reward)
	tx.TxOut = make([]*btc.TxOut, 2)
	tx.TxOut[0] = new(btc.TxOut)
	tx.TxOut[0].Value = 50e8
	tx.TxOut[0].Pk_script = the_addr.OutScript()

	// add second output - witness merkle, null for now (to be updated after adding txs to the block)
	tx.TxOut[1] = new(btc.TxOut)
	tx.TxOut[1].Pk_script = make([]byte, 6+32)
	copy(tx.TxOut[1].Pk_script[0:6], []byte{0x6a, 0x24, 0xaa, 0x21, 0xa9, 0xed})
	return
}

func update_witness_merkle(bl *btc.Block) {
	tx := bl.Txs[0]
	merkle, _ := btc.GetWitnessMerkle(bl.Txs)
	with_nonce := btc.Sha2Sum(append(merkle, tx.SegWit[0][0]...))
	copy(tx.TxOut[1].Pk_script[6:], with_nonce[:])
}

func get_testnet_timestamp() (curtime uint32) {
	now := time.Now().Unix()
	prv := int64(common.Last.Block.Timestamp())
	if now-prv > int64(WAIT_FOR_SECONDS) {
		curtime = uint32(prv + 20*60 + 1)
		if uint32(now) > curtime {
			curtime = uint32(now)
		}
	} else {
		curtime = uint32(now)
	}
	return
}

func GetWork(r *RpcGetWorkResp) {
	bl := new(btc.Block)

	common.Last.Mutex.Lock()
	height := common.Last.Block.Height + 1
	curtime := get_testnet_timestamp()
	bits := common.BlockChain.GetNextWorkRequired(common.Last.Block, uint32(curtime))
	common.Last.Mutex.Unlock()

	bl.Txs = make([]*btc.Tx, 1)
	bl.Txs[0] = make_coinbase_tx(height)

	cpfp := txpool.GetSortedMempoolRBF()
	//println(len(cpfp), "transactions")
	bl.Txs[0].SetHash(bl.Txs[0].SerializeNew()) // this will not be the final hash, but to get a propoer weight in the next line
	cur_tx_weight := bl.Txs[0].Weight()

	for _, v := range cpfp {
		tx := v.Tx
		if !tx.IsFinal(height, uint32(curtime)) {
			continue
		}

		w := tx.Weight()
		if cur_tx_weight+w > 4e6 {
			//println("Too many txs - max weight reached")
			break
		}

		if sigops+v.SigopsCost > btc.MAX_BLOCK_SIGOPS_COST {
			//println("Too many sigops - limit to 999000 bytes")
			return
		}

		bl.Txs = append(bl.Txs, tx)
		bl.Txs[0].TxOut[0].Value += v.Fee
		cur_tx_weight += w
		sigops += v.SigopsCost
	}
	update_witness_merkle(bl)

	bl.Txs[0].SetHash(bl.Txs[0].SerializeNew())
	merkle, _ := bl.GetMerkle()

	var zer [32]byte
	var data [128]byte

	mining_info.Blocks = uint(height)
	mining_info.Difficulty = btc.GetDifficulty(bits)

	target_ := btc.SetCompact(bits).Bytes()
	target := append(zer[:32-len(target_)], target_...)
	swap256(target) // getwork is expected to return the target as little endian
	r.Result.Target = hex.EncodeToString(target)
	binary.LittleEndian.PutUint32(data[0:4], 0x20000000)
	copy(data[4:36], common.Last.Block.BlockHash.Hash[:])
	copy(data[36:36+32], merkle)
	binary.LittleEndian.PutUint32(data[68:72], uint32(curtime))
	binary.LittleEndian.PutUint32(data[72:76], bits)
	// data[76:80]  - nonce

	bl.Raw = make([]byte, 80)
	copy(bl.Raw, data[:80])

	swap32(data[:80]) // getwork is expected to return the block header in a fucked up way
	r.Result.Data = hex.EncodeToString(data[:])

	currently_worked_block = bl
	fmt.Printf("getwork  time_off:%d  =>  #%d / dif:%.0f / txs:%d / val:%d / ts:%s\n",
		WAIT_FOR_SECONDS, height, btc.GetDifficulty(bits), len(bl.Txs), bl.Txs[0].TxOut[0].Value,
		time.Unix(int64(curtime), 0).Format("15:04:05"))
}

func GetNextBlockTemplate(r *GetBlockTemplateResp) {
	var zer [32]byte

	common.Last.Mutex.Lock()
	r.Curtime = uint(get_testnet_timestamp())
	r.Mintime = uint(common.Last.Block.GetMedianTimePast()) + 1
	if r.Curtime < r.Mintime {
		r.Curtime = r.Mintime
	}
	println("getblocktemplate timestamp:", time.Unix(int64(r.Curtime), 0).Format("15:04:05"))
	height := common.Last.Block.Height + 1
	bits := common.BlockChain.GetNextWorkRequired(common.Last.Block, uint32(r.Curtime))
	r.PreviousBlockHash = common.Last.Block.BlockHash.String()
	common.Last.Mutex.Unlock()

	target := btc.SetCompact(bits).Bytes()

	r.Capabilities = []string{"proposal"}
	r.Version = 0x20000000

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

}

/* memory pool transaction sorting stuff */
type one_mining_tx struct {
	*txpool.OneTxToSend
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
	for _, v := range txpool.TransactionsToSend {
		tx := v.Tx

		if !DO_SEGWIT && tx.SegWit != nil {
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

	txpool.TxMutex.Lock()
	defer txpool.TxMutex.Unlock()

	var cnt int
	var sorted sortedTxList
	txs_so_far = make(map[[32]byte]uint)
	totlen = 0
	sigops = 0
	//println("\ngetting txs from the pool of", len(txpool.TransactionsToSend), "...")
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
	/*if len(txs_so_far)!=len(txpool.TransactionsToSend) {
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
