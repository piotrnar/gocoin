package chain

import (
	"errors"
	"fmt"

	"github.com/piotrnar/gocoin/lib/btc"
)

func nextBlock(ch *Chain, hash, header []byte, height, blen, txs uint32) {
	bh := btc.NewUint256(hash[:])
	if _, ok := ch.BlockIndex[bh.BIdx()]; ok {
		fmt.Println("nextBlock:", bh.String(), "- already in")
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

// loadBlockIndex loads the block index from the disk.
func (ch *Chain) loadBlockIndex() {
	ch.BlockIndex = make(map[[btc.Uint256IdxLen]byte]*BlockTreeNode, BlockMapInitLen)
	ch.BlockTreeRoot = new(BlockTreeNode)
	ch.BlockTreeRoot.BlockHash = ch.Genesis
	ch.RebuildGenesisHeader()
	ch.BlockIndex[ch.Genesis.BIdx()] = ch.BlockTreeRoot

	ch.Blocks.LoadBlockIndex(ch, nextBlock)
	tlb := ch.Unspent.LastBlockHash
	//fmt.Println("Building tree from", len(ch.BlockIndex), "nodes")
	for _, v := range ch.BlockIndex {
		if AbortNow {
			return
		}
		if v == ch.BlockTreeRoot {
			// skip root block (should be only one)
			continue
		}

		par, ok := ch.BlockIndex[btc.NewUint256(v.BlockHeader[4:36]).BIdx()]
		if !ok {
			fmt.Println("ERROR: Block", v.Height, v.BlockHash.String(), "has no Parent")
			fmt.Println("...", btc.NewUint256(v.BlockHeader[4:36]).String(), "- removing it from blocksDB")
			continue // it will be removed (along with its descendants) by removeDetachedNodes()
		}
		v.Parent = par
		v.Parent.addChild(v)
	}
	ch.removeDetachedNodes()
	if tlb == nil {
		//fmt.Println("No last block - full rescan will be needed")
		ch.SetLast(ch.BlockTreeRoot)
		return
	} else {
		//fmt.Println("Last Block Hash:", btc.NewUint256(tlb).String())
		last, ok := ch.BlockIndex[btc.NewUint256(tlb).BIdx()]
		if !ok {
			panic("Last Block Hash not found")
		}
		ch.SetLast(last)
	}
}

// removeDetachedNodes removes from the block index all the nodes that are not
// connected to the root of the tree (blocks with no parent and all their descendants).
// The order in which the tree was being built does not matter here.
func (ch *Chain) removeDetachedNodes() {
	// first just count the nodes of the tree, as usually there is nothing to remove
	var cnt int
	stack := []*BlockTreeNode{ch.BlockTreeRoot}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = append(stack[:len(stack)-1], n.Childs...)
		cnt++
	}
	if cnt == len(ch.BlockIndex) {
		return
	}

	attached := make(map[*BlockTreeNode]struct{}, cnt)
	stack = append(stack, ch.BlockTreeRoot)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = append(stack[:len(stack)-1], n.Childs...)
		attached[n] = struct{}{}
	}
	for k, v := range ch.BlockIndex {
		if _, ok := attached[v]; !ok {
			if v.Parent != nil {
				fmt.Println("ERROR: Block", v.Height, v.BlockHash.String(), "is not connected to the tree - removing it")
			}
			delete(ch.BlockIndex, k)
		}
	}
}

func (ch *Chain) GetRawTx(BlockHeight uint32, txid *btc.Uint256) (data []byte, er error) {
	// Find the block with the indicated Height in the main tree
	ch.BlockIndexAccess.Lock()
	n := ch.LastBlock()
	if n.Height < BlockHeight {
		fmt.Println(n.Height, BlockHeight)
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
			data = bl.Txs[i].Raw
			return
		}
	}
	er = errors.New("GetRawTx: BuildTxList failed")
	return
}
