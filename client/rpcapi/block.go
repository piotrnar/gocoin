package rpcapi

import (
	"bytes"
	"encoding/hex"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/lib/btc"
)

type BlockSubmited struct {
	*btc.Block
	Error string
	Done  sync.WaitGroup
}

var RpcBlocks chan *BlockSubmited = make(chan *BlockSubmited, 1)

func SubmitBlock(cmd *RpcCommand, resp *RpcResponse, b []byte) {
	var bd []byte
	var er error

	switch uu := cmd.Params.(type) {
	case []interface{}:
		if len(uu) < 1 {
			resp.Error = RpcError{Code: -1, Message: "empty params array"}
			return
		}
		str := uu[0].(string)
		if str[0] == '@' {
			/*
				gocoin special case: if the string starts with @, it's a name of the file with block's binary data
					curl --user gocoinrpc:gocoinpwd --data-binary \
						'{"jsonrpc": "1.0", "id":"curltest", "method": "submitblock", "params": \
							["@450529_000000000000000000cf208f521de0424677f7a87f2f278a1042f38d159565f5.bin"] }' \
						-H 'content-type: text/plain;' http://127.0.0.1:8332/
			*/
			//println("jade z koksem", str[1:])
			bd, er = os.ReadFile(str[1:])
		} else {
			bd, er = hex.DecodeString(str)
		}
		if er != nil {
			resp.Error = RpcError{Code: -3, Message: er.Error()}
			return
		}

	default:
		resp.Error = RpcError{Code: -2, Message: "incorrect params type"}
		return
	}

	bl, er := btc.NewBlock(bd)
	if er != nil {
		resp.Error = RpcError{Code: -4, Message: er.Error()}
		return
	}

	resp.Result = submitBlockInt(bl)
}

func SubmitWork(bl *btc.Block) {
	if bl == nil {
		println("ERROR: No work pending")
		return
	}

	bl.Hash = btc.NewSha2Hash(bl.Raw[:80])
	println("NewBlock by SubmitWork:", bl.Hash.String())
	wr := bytes.NewBuffer(bl.Raw[:80])
	btc.WriteVlen(wr, uint64(len(bl.Txs)))
	for _, tx := range bl.Txs {
		tx.WriteSerializedNew(wr)
	}
	bl.UpdateContent(wr.Bytes())
	submitBlockInt(bl)
}

func submitBlockInt(bl *btc.Block) (result string) {
	if DO_NOT_SUBMIT {
		println("*** Do not submit blocks for now - just simulation ***")
		return
	}

	bs := new(BlockSubmited)

	bs.Block = bl

	network.MutexRcv.Lock()
	network.ReceivedBlocks[bs.Block.Hash.BIdx()] = &network.OneReceivedBlock{TmStart: time.Now()}
	network.MutexRcv.Unlock()

	println("###### new block", bs.Block.Hash.String(), "len", len(bl.Raw), "######")
	bs.Done.Add(1)
	RpcBlocks <- bs
	bs.Done.Wait()
	if bs.Error != "" {
		//resp.Error = RpcError{Code: -10, Message: bs.Error}
		idx := strings.Index(bs.Error, "- RPC_Result:")
		if idx == -1 {
			result = "inconclusive"
		} else {
			result = bs.Error[idx+13:]
		}
		println("submiting block error:", bs.Error)
		println("submiting block result:", result)

		print("time_now:", time.Now().Unix())
		print("  cur_block_ts:", bs.Block.BlockTime())
		print("  last_given_now:", last_given_time)
		print("  last_given_min:", last_given_mintime)
		common.Last.Mutex.Lock()
		print("  prev_block_ts:", common.Last.Block.Timestamp())
		common.Last.Mutex.Unlock()
		println()

		return
	}
	return
}

var last_given_time, last_given_mintime uint32
