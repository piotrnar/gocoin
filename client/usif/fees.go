package usif

import (
	"bufio"
	"encoding/gob"
	"os"
	"sync"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
)

const (
	BLKFES_FILE_NAME = "blkfees.gob"
)

var (
	BlockFeesMutex sync.Mutex
	BlockFees      map[uint32][][3]uint64 = make(map[uint32][][3]uint64) // [0]=Weight  [1]-Fee  [2]-Group
	BlockFeesDirty bool                                                  // it true, clean up old data
)

func ProcessBlockFees(height uint32, bl *btc.Block) {
	if len(bl.Txs) < 2 {
		return
	}

	txs := make(map[[32]byte]int, len(bl.Txs)) // group_id -> transaciton_idx
	txs[bl.Txs[0].Hash.Hash] = 0

	fees := make([][3]uint64, len(bl.Txs)-1)

	for i := 1; i < len(bl.Txs); i++ {
		txs[bl.Txs[i].Hash.Hash] = i
		fees[i-1][0] = uint64(3*bl.Txs[i].NoWitSize + bl.Txs[i].Size)
		fees[i-1][1] = uint64(bl.Txs[i].Fee)
		fees[i-1][2] = uint64(i)
	}

	for i := 1; i < len(bl.Txs); i++ {
		for _, inp := range bl.Txs[i].TxIn {
			if paretidx, yes := txs[inp.Input.Hash]; yes {
				if fees[paretidx-1][2] < fees[i-1][2] { // only update it for a lower index
					fees[i-1][2] = fees[paretidx-1][2]
				}
			}
		}
	}

	BlockFeesMutex.Lock()
	BlockFees[height] = fees
	BlockFeesDirty = true
	BlockFeesMutex.Unlock()
}

func ExpireBlockFees() {
	var height uint32
	common.Last.Lock()
	height = common.Last.Block.Height
	common.Last.Unlock()

	if height <= 144 {
		return
	}
	height -= 144

	BlockFeesMutex.Lock()
	if BlockFeesDirty {
		for k, _ := range BlockFees {
			if k < height {
				delete(BlockFees, k)
			}
		}
		BlockFeesDirty = false
	}
	BlockFeesMutex.Unlock()
}

func SaveBlockFees() {
	f, er := os.Create(common.GocoinHomeDir + BLKFES_FILE_NAME)
	if er != nil {
		println("SaveBlockFees:", er.Error())
		return
	}

	ExpireBlockFees()
	buf := bufio.NewWriter(f)
	er = gob.NewEncoder(buf).Encode(BlockFees)

	if er != nil {
		println("SaveBlockFees:", er.Error())
	}

	buf.Flush()
	f.Close()

}

func LoadBlockFees() {
	f, er := os.Open(common.GocoinHomeDir + BLKFES_FILE_NAME)
	if er != nil {
		println("LoadBlockFees:", er.Error())
		return
	}

	buf := bufio.NewReader(f)
	er = gob.NewDecoder(buf).Decode(&BlockFees)
	if er != nil {
		println("LoadBlockFees:", er.Error())
	}

	f.Close()
}

var (
	AverageFeeMutex     sync.Mutex
	AverageFeeBytes     uint64
	AverageFeeTotal     uint64
	AverageFee_SPB      float64
	averageFeeLastBlock *chain.BlockTreeNode
	averageFeeLastCount uint = 0xffffffff
)

func GetAverageFee() float64 {
	common.Last.Mutex.Lock()
	end := common.Last.Block
	common.Last.Mutex.Unlock()

	common.LockCfg()
	blocks := common.CFG.Stat.FeesBlks
	common.UnlockCfg()
	if blocks <= 0 {
		blocks = 1 // at leats one block
	}

	AverageFeeMutex.Lock()
	defer AverageFeeMutex.Unlock()

	if end == averageFeeLastBlock && averageFeeLastCount == blocks {
		return AverageFee_SPB // we've already calculated for this block
	}

	averageFeeLastBlock = end
	averageFeeLastCount = blocks

	AverageFeeBytes = 0
	AverageFeeTotal = 0

	for blocks > 0 {
		bl, _, e := common.BlockChain.Blocks.BlockGet(end.BlockHash)
		if e != nil {
			return 0
		}
		block, e := btc.NewBlockX(bl, end.BlockHash)
		if e != nil {
			return 0
		}

		rb, cbasetx := GetReceivedBlockX(block)
		var fees_from_this_block int64
		for o := range cbasetx.TxOut {
			fees_from_this_block += int64(cbasetx.TxOut[o].Value)
		}
		fees_from_this_block -= int64(btc.GetBlockReward(end.Height))

		if fees_from_this_block > 0 {
			AverageFeeTotal += uint64(fees_from_this_block)
		}

		AverageFeeBytes += uint64(rb.ThePaidVSize)

		blocks--
		end = end.Parent
	}
	if AverageFeeBytes == 0 {
		if AverageFeeTotal != 0 {
			panic("Impossible that miner gest a fee with no transactions in the block")
		}
		AverageFee_SPB = 0
	} else {
		AverageFee_SPB = float64(AverageFeeTotal) / float64(AverageFeeBytes)
	}
	return AverageFee_SPB
}
