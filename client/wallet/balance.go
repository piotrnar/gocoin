package wallet

import (
	"io"
	"os"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/client/common"
)

func GetRawTransaction(BlockHeight uint32, txid *btc.Uint256, txf io.Writer) bool {
	// Find the block with the indicated Height in the main tree
	common.BlockChain.BlockIndexAccess.Lock()
	n := common.Last.Block
	if n.Height < BlockHeight {
		println(n.Height, BlockHeight)
		common.BlockChain.BlockIndexAccess.Unlock()
		panic("This should not happen")
	}
	for n.Height > BlockHeight {
		n = n.Parent
	}
	common.BlockChain.BlockIndexAccess.Unlock()

	bd, _, e := common.BlockChain.Blocks.BlockGet(n.BlockHash)
	if e != nil {
		println("BlockGet", n.BlockHash.String(), BlockHeight, e.Error())
		println("This should not happen - please, report a bug.")
		println("You can probably fix it by launching the client with -rescan")
		os.Exit(1)
	}

	bl, e := btc.NewBlock(bd)
	if e != nil {
		println("NewBlock: ", e.Error())
		os.Exit(1)
	}

	e = bl.BuildTxList()
	if e != nil {
		println("BuildTxList:", e.Error())
		os.Exit(1)
	}

	// Find the transaction we need and store it in the file
	for i := range bl.Txs {
		if bl.Txs[i].Hash.Equal(txid) {
			txf.Write(bl.Txs[i].Serialize())
			return true
		}
	}
	return false
}
