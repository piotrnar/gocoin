package usif

import (
	"bufio"
	"encoding/gob"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"os"
	"sync"
)

const (
	BLKFES_FILE_NAME = "blkfees.gob"
)

var (
	BlockFeesMutex sync.Mutex
	BlockFees map[uint32][][3]uint64 = make(map[uint32][][3]uint64) // [0]=Size  [1]-Fee  [2]-Group
	BlockFeesDirty bool // it true, clean up old data
)

func ProcessBlockFees(newbl *network.BlockRcvd) {
	bl := newbl.Block

	if len(bl.Txs) < 2 {
		return
	}

	txs := make(map[[32]byte]int, len(bl.Txs)) // group_id -> transaciton_idx
	txs[bl.Txs[0].Hash.Hash] = 0

	fees := make([][3]uint64, len(bl.Txs)-1)

	for i := 1; i < len(bl.Txs); i++ {
		txs[bl.Txs[i].Hash.Hash] = i

		kspb := 1000 * bl.Txs[i].Fee / uint64(bl.Txs[i].Size)
		if i == 1 || newbl.MinFeeKSPB > kspb {
			newbl.MinFeeKSPB = kspb
		}
		fees[i-1][0] = uint64(bl.Txs[i].Size)
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
	BlockFees[newbl.BlockTreeNode.Height] = fees
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
