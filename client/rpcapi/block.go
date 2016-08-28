package rpcapi

import (
	"time"
	"sync"
	"strings"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/client/network"
	"encoding/json"
	"io/ioutil"
	"fmt"
	"github.com/piotrnar/gocoin/client/common"
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
			if len(uu)<1 {
				resp.Error = RpcError{Code: -1, Message: "empty params array"}
				return
			}
			bd, er = hex.DecodeString(uu[0].(string))
			if er != nil {
				resp.Error = RpcError{Code: -3, Message: er.Error()}
				return
			}

		default:
			resp.Error = RpcError{Code: -2, Message: "incorrect params type"}
			return
	}

	bs := new(BlockSubmited)

	bs.Block, er = btc.NewBlock(bd)
	if er != nil {
		resp.Error = RpcError{Code: -4, Message: er.Error()}
		return
	}

	network.MutexRcv.Lock()
	network.ReceivedBlocks[bs.Block.Hash.BIdx()] = &network.OneReceivedBlock{TmStart: time.Now()}
	network.MutexRcv.Unlock()

	println("new block", bs.Block.Hash.String(), "len", len(bd), "- submitting...")
	bs.Done.Add(1)
	RpcBlocks <- bs
	bs.Done.Wait()
	if bs.Error != "" {
		//resp.Error = RpcError{Code: -10, Message: bs.Error}
		idx := strings.Index(bs.Error, "- RPC_Result:")
		if idx == -1 {
			resp.Result = "inconclusive"
		} else {
			resp.Result = bs.Error[idx+13:]
		}
		println("submiting block error:", bs.Error)
		println("submiting block result:", resp.Result.(string))

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

	// cress check with bitcoind...
	if false {
		bitcoind_result := process_rpc(b)
		json.Unmarshal(bitcoind_result, &resp)
		switch cmd.Params.(type) {
			case string:
				println("\007Block rejected by bitcoind:", resp.Result.(string))
				ioutil.WriteFile(fmt.Sprint(bs.Block.Height, "-", bs.Block.Hash.String()), bd, 0777)
			default:
				println("submiting block verified OK", bs.Error)
		}
	}
}

var last_given_time, last_given_mintime uint32
