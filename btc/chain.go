package btc

import (
	"errors"
	"bytes"
	"os"
	"fmt"
)

type BlockTreeNode struct {
	hash *Uint256
	height uint32
	parent *BlockTreeNode
	childs []*BlockTreeNode
}

func (v *BlockTreeNode)Save(f *os.File) {
	f.Write(v.hash.Hash[:])
	if v.parent != nil {
		f.Write(v.parent.hash.Hash[:])
	} else {
		f.Write(bytes.Repeat([]byte{0}, 32))
	}
}

type Chain struct {
	
	blockTreeRoot *BlockTreeNode
	blockTreeEnd *BlockTreeNode

	blockIndex map[[32]byte] *BlockTreeNode
	orphaned map[[32]byte] *BlockTreeNode
	
	blockdb *BlockDB;
	unspent *UnspentDb
}

func NewChain(blockdb *BlockDB, genesis *Uint256) (ch *Chain) {
	ch = new(Chain)
	ch.blockdb = blockdb
	ch.blockIndex = make(map[[32]byte] *BlockTreeNode, BlockMapInitLen)
	
	ch.blockTreeRoot = new(BlockTreeNode)
	ch.blockTreeRoot.hash = genesis
	ch.blockIndex[genesis.Hash] = ch.blockTreeRoot
	ch.blockTreeEnd = ch.blockTreeRoot
	ch.orphaned = make(map[[32]byte] *BlockTreeNode, UnwindBufferMaxHistory)
	ch.unspent = NewUnspentDb()
	return 
}


func (ch *Chain)CommitTransactions(bl *Block, height uint32) (e error) {
	ch.unspent.NewHeight(height)
	e = ch.commitTxs(bl, height)
	return
}

func (ch *Chain)commitTxs(bl *Block, height uint32) (error) {
	sumblockin := GetBlockReward(height)
	sumblockout := uint64(0)
	
	if don(DBG_TX) {
		fmt.Printf("Commiting %d transactions\n", len(bl.Txs))
	}
	for i := range bl.Txs {
		var txoutsum, txinsum uint64
		if don(DBG_TX) {
			fmt.Printf("tx %d/%d:\n", i+1, len(bl.Txs))
		}
		for j := range bl.Txs[i].TxIn {
			if i>0 {
				tout, present := ch.unspent.PickUnspent(&bl.Txs[i].TxIn[j].Input)
				if !present {
					return errors.New("CommitTransactions() : unknown input " + bl.Txs[i].TxIn[j].Input.String())
				}
				// Verify Transaction script:
				
				if !VerifyTxScript(bl.Txs[i].TxIn[j].ScriptSig, tout) {
					fmt.Printf("Transaction signature error in block %d!\n", height)
					os.Exit(1)
				}

				txinsum += tout.Value

				ch.unspent.RemoveLastPick(height)

				if don(DBG_TX) {
					fmt.Printf("  in %d: %s  %.8f\n", j+1, bl.Txs[i].TxIn[j].Input.String(),
						float64(tout.Value)/1e8)
				}
			} else {
				if don(DBG_TX) {
					fmt.Printf("  freshly generated %.8f\n", float64(sumblockin)/1e8)
				}
			}
		}
		sumblockin += txinsum

		var iii TxPrevOut
		copy(iii.Hash[:], bl.Txs[i].Hash.Hash[:])
		for j := range bl.Txs[i].TxOut {
			if don(DBG_TX) {
				fmt.Printf("  out %d: %12.8f\n", j+1, float64(bl.Txs[i].TxOut[j].Value)/1e8)
			}
			txoutsum += bl.Txs[i].TxOut[j].Value
			iii.Index = uint32(j)
			ch.unspent.Append(height, iii, &bl.Txs[i].TxOut[j])
			
		}
		sumblockout += txoutsum
		
		if don(DBG_TX) {
			fmt.Sprintf("  %12.8f -> %12.8f  (%.8f)\n", 
				float64(txinsum)/1e8, float64(txoutsum)/1e8,
				float64(txinsum-txoutsum)/1e8)
		}
	}

	if sumblockin < sumblockout {
		return errors.New(fmt.Sprintf("CommitTransactions: Out:%d > In:%d", sumblockout, sumblockin))
	} else if don(DBG_WASTED) && sumblockin != sumblockout {
		fmt.Printf("%.8f BTC wasted in block %d\n", float64(sumblockin-sumblockout)/1e8, height)
	}
	return nil
}


func (n *BlockTreeNode)GetmaxDepth() (res uint32) {
	for i :=  range n.childs {
		dep := 1+n.childs[i].GetmaxDepth()
		if dep > res {
			res = dep
		}
	}
	return
}


func (n *BlockTreeNode)FindLongestChild() (res *BlockTreeNode) {
	if len(n.childs)==0 {
		return
	}
	
	res = n.childs[0]
	cdepth := n.childs[0].GetmaxDepth()

	for i := 1; i<len(n.childs); i++ {
		ncd := n.childs[i].GetmaxDepth()
		if ncd > cdepth {
			ncd = cdepth
			res = n.childs[i]
		}
	}
	return
}

func (ch *Chain)MoveToBranch(cur *BlockTreeNode) (error) {
	fmt.Printf("Moving branches %d -> %d\n", ch.blockTreeEnd.height, cur.height)
	old := ch.blockTreeEnd
	
	for cur.height > old.height {
		cur = cur.parent
	}
	
	for old!=cur {
		ch.unspent.UnwindBlock(old.height)
		
		fmt.Printf("->orph block %s @ %d\n", old.hash.String(), old.height)
		ch.orphaned[old.hash.Hash] = old
		delete(ch.blockIndex, old.hash.Hash)
		
		old = old.parent
		cur = cur.parent
	}
	
	fmt.Printf("Found common node @ %d\n", cur.height)
	for  {
		cur = cur.FindLongestChild()
		if cur == nil {
			break
		}
		fmt.Printf(" + new %d ... \n", cur.height)
		
		b, er := ch.blockdb.GetBlock(cur.hash)
		errorFatal(er, "MoveToBranch/GetBlock")

		bl, er := NewBlock(b)
		errorFatal(er, "MoveToBranch/NewBlock")

		fmt.Println("  ... Got block ", bl.Hash.String())
		bl.BuildTxList()

		er = ch.CommitTransactions(bl, cur.height)
		errorFatal(er, "MoveToBranch/CommitTransactions")
		fmt.Printf("  ... %d new txs commited\n", len(bl.Txs))

		delete(ch.orphaned, cur.hash.Hash)
		ch.blockTreeEnd = cur
	}
	return nil
}


func (ch *Chain)AcceptBlock(bl *Block) (e error) {
	_, pres := ch.blockIndex[bl.Hash.Hash]
	if pres {
		return errors.New("AcceptBlock() : "+bl.Hash.String()+" already in mapblockIndex")
	}

	var p [32]byte
	copy(p[:], bl.GetParent()[:])
	prevblk, ok := ch.blockIndex[p]
	if !ok {
		return errors.New("AcceptBlock() : prev block not found")
	}

	// create new BlockTreeNode
	cur := new(BlockTreeNode)
	cur.hash = bl.Hash
	cur.parent = prevblk
	cur.height = prevblk.height + 1
	
	prevblk.childs = append(prevblk.childs, cur)
	
	// Add this block to the block index
	ch.blockIndex[cur.hash.Hash] = cur

	// Update the end of the tree
	if ch.blockTreeEnd==prevblk {
		ch.blockTreeEnd = cur
		if don(DBG_BLOCKS) {
			fmt.Printf("Adding block %s @ %d\n", cur.hash.String(), cur.height)
		}
	} else {
		if don(DBG_BLOCKS|DBG_ORPHAS) {
			fmt.Printf("Orphan block %s @ %d\n", cur.hash.String(), cur.height)
		}
		ch.orphaned[bl.Hash.Hash] = cur
		if cur.height > ch.blockTreeEnd.height {
			ch.MoveToBranch(cur)
			/*return errors.New(fmt.Sprintf("The different branch is longer now %d/%d!\n",
				cur.height, ch.blockTreeEnd.height))*/
		}
		return nil
	}

	// TODO: Check proof of work
	// TODO: Check timestamp against prev
	
	// Assume that all transactions are finalized

	e = ch.CommitTransactions(bl, cur.height)
	if e != nil {
		println("rejecting block", cur.height, cur.hash.String())
		ch.unspent.UnwindBlock(cur.height)
		delete(ch.blockIndex, cur.hash.Hash)
		ch.blockTreeEnd = ch.blockTreeEnd.parent
	}
	return 
}


func (ch *Chain)Stats() (s string) {
	s = fmt.Sprintf("BCHAIN  : height=%d orphaned=%d/%d\n", 
		ch.blockTreeEnd.height, len(ch.orphaned), len(ch.blockIndex))
	s += ch.unspent.Stats()
	return
}

func (ch *Chain)GetHeight() uint32 {
	return ch.blockTreeEnd.height
}


func (ch *Chain)Save() {
	ch.blockdb.Save()
	ch.unspent.Save()
	
	// 96 bytes per record
	f, e := os.Create("blocktr_idx.bin")
	if e == nil {
		for k, v := range ch.blockIndex {
			f.Write(k[:])
			v.Save(f)
		}
		println(len(ch.blockIndex), "saved in blocktr_idx.bin")
		f.Close()
	}
	
	// 96 bytes per record
	f, e = os.Create("orphaned.bin")
	if e == nil {
		for k, v := range ch.orphaned {
			f.Write(k[:])
			v.Save(f)
		}
		println(len(ch.orphaned), "saved in orphaned.bin")
		f.Close()
	}
}

func (ch *Chain)LookUnspent(tid [32]byte, vout uint32) *TxOut {
	txin := TxPrevOut{Hash:tid, Index:vout}
	return ch.unspent.LookUnspent(&txin)
}
