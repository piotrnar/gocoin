package chain

import (
	"errors"
	"github.com/piotrnar/gocoin/lib/btc"
)


func nextBlock(ch *Chain, hash, header []byte, height, blen, txs uint32) {
	bh := btc.NewUint256(hash[:])
	if _, ok := ch.BlockIndex[bh.BIdx()]; ok {
		println("nextBlock:", bh.String(), "- already in")
		return
	}
	v := new(BlockTreeNode)
	v.BlockHash = bh
	v.Height = height
	v.BlockSize = blen
	v.TxCount = txs
	copy(v.BlockHeader[:], header)
	ch.BlockIndex[v.BlockHash.BIdx()] = v
}


// Loads block index from the disk
func (ch *Chain)loadBlockIndex() {
	ch.BlockIndex = make(map[[btc.Uint256IdxLen]byte]*BlockTreeNode, BlockMapInitLen)
	ch.BlockTreeRoot = new(BlockTreeNode)
	ch.BlockTreeRoot.BlockHash = ch.Genesis
	ch.RebuildGenesisHeader()
	ch.BlockIndex[ch.Genesis.BIdx()] = ch.BlockTreeRoot


	ch.Blocks.LoadBlockIndex(ch, nextBlock)
	tlb := ch.Unspent.LastBlockHash
	//println("Building tree from", len(ch.BlockIndex), "nodes")
	for k, v := range ch.BlockIndex {
		if AbortNow {
			return
		}
		if v==ch.BlockTreeRoot {
			// skip root block (should be only one)
			continue
		}

		par, ok := ch.BlockIndex[btc.NewUint256(v.BlockHeader[4:36]).BIdx()]
		if !ok {
			println("ERROR: Block", v.Height, v.BlockHash.String(), "has no Parent")
			println("...", btc.NewUint256(v.BlockHeader[4:36]).String(), "- removing it from blocksDB")
			delete(ch.BlockIndex, k)
			continue
		}
		v.Parent = par
		v.Parent.addChild(v)
	}
	if tlb == nil {
		//println("No last block - full rescan will be needed")
		ch.BlockTreeEnd = ch.BlockTreeRoot
		return
	} else {
		//println("Last Block Hash:", btc.NewUint256(tlb).String())
		var ok bool
		ch.BlockTreeEnd, ok = ch.BlockIndex[btc.NewUint256(tlb).BIdx()]
		if !ok {
			panic("Last Block Hash not found")
		}
	}
}

func (ch *Chain) GetRawTx(BlockHeight uint32, txid *btc.Uint256) (data []byte, er error) {
	// Find the block with the indicated Height in the main tree
	ch.BlockIndexAccess.Lock()
	n := ch.BlockTreeEnd
	if n.Height < BlockHeight {
		println(n.Height, BlockHeight)
		ch.BlockIndexAccess.Unlock()
		er = errors.New("GetRawTx: block height too big")
		return
	}
	for n.Height > BlockHeight {
		n = n.Parent
	}
	ch.BlockIndexAccess.Unlock()

	bd, _, e := ch.Blocks.BlockGet(n.BlockHash)
	if e != nil {
		er = errors.New("GetRawTx: block not in the database")
		return
	}

	bl, e := btc.NewBlock(bd)
	if e != nil {
		er = errors.New("GetRawTx: NewBlock failed")
		return
	}

	e = bl.BuildTxList()
	if e != nil {
		er = errors.New("GetRawTx: BuildTxList failed")
		return
	}

	// Find the transaction we need and store it in the file
	for i := range bl.Txs {
		if bl.Txs[i].Hash.Equal(txid) {
			data = bl.Txs[i].Serialize()
			return
		}
	}
	er = errors.New("GetRawTx: BuildTxList failed")
	return
}
