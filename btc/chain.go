package btc

import (
	"errors"
	"os"
	"fmt"
)

const blockMapLen = 8  // The bigger it is, the more memory is needed, but lower chance of a collision


type BlockTreeNode struct {
	BlockHash Uint256
	Height uint32
	parent *BlockTreeNode
	childs []*BlockTreeNode
	Orphan bool
}

type Chain struct {
	Db BtcDB
	
	BlockTreeRoot *BlockTreeNode
	BlockTreeEnd *BlockTreeNode

	BlockIndex map[[blockMapLen]byte] *BlockTreeNode
	
	unspent *UnspentDb
}


func NewChain(genesis *Uint256) (ch *Chain) {
	ch = new(Chain)
	ch.Db = NewDb()
	ch.BlockIndex = make(map[[blockMapLen]byte] *BlockTreeNode, BlockMapInitLen)
	
	ch.BlockTreeRoot = new(BlockTreeNode)
	ch.BlockTreeRoot.BlockHash = *genesis
	ch.BlockIndex[NewBlockIndex(genesis.Hash[:])] = ch.BlockTreeRoot
	ch.BlockTreeEnd = ch.BlockTreeRoot
	ch.unspent = NewUnspentDb(ch.Db)

	ch.loadIndex()
	return 
}


func NewBlockIndex(h []byte) (o [blockMapLen]byte) {
	copy(o[:], h[:blockMapLen])
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
			iii.Vout = uint32(j)
			ch.unspent.Append(height, iii, bl.Txs[i].TxOut[j])
			
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
	for i := range n.childs {
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
	if len(n.childs)==1 {
		return
	}

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
	fmt.Printf("Moving branches %d -> %d\n", ch.BlockTreeEnd.Height, cur.Height)

	old := ch.BlockTreeEnd
	
	for cur.Height > old.Height {
		cur = cur.parent
	}
	
	for old!=cur {
		ch.unspent.UnwindBlock(old.Height)
		
		fmt.Printf("->orph block %s @ %d\n", old.BlockHash.String(), old.Height)
		old.Orphan = true
		ch.Db.BlockOrphan(&old.BlockHash, 1)
		
		old = old.parent
		cur = cur.parent
	}
	
	fmt.Printf("Found common node @ %d\n", cur.Height)
	for  {
		cur = cur.FindLongestChild()
		if cur == nil {
			break
		}
		fmt.Printf(" + new %d ... \n", cur.Height)
		
		b, er := ch.Db.BlockGet(&cur.BlockHash)
		if er != nil {
			return er
		}

		bl, er := NewBlock(b)
		if er != nil {
			return er
		}

		fmt.Println("  ... Got block ", bl.Hash.String())
		bl.BuildTxList()

		er = ch.CommitTransactions(bl, cur.Height)
		if er != nil {
			return er
		}
		fmt.Printf("  ... %d new txs commited\n", len(bl.Txs))

		cur.Orphan = false
		ch.Db.BlockOrphan(&cur.BlockHash, 0)
		ch.BlockTreeEnd = cur
	}
	return nil
}


func (n *BlockTreeNode)addChild(c *BlockTreeNode) {
	n.childs = append(n.childs, c)
}


func (ch *Chain)AcceptBlock(bl *Block) (e error) {
	_, pres := ch.BlockIndex[bl.Hash.BIdx()]
	if pres {
		return errors.New("AcceptBlock() : "+bl.Hash.String()+" already in mapBlockIndex")
	}

	var p [32]byte
	copy(p[:], bl.GetParent()[:])
	prevblk, ok := ch.BlockIndex[NewBlockIndex(p[:])]
	if !ok {
		return errors.New("AcceptBlock() : prv not found:"+NewUint256(p[:]).String())
	}

	// create new BlockTreeNode
	cur := new(BlockTreeNode)
	cur.BlockHash = *bl.Hash
	cur.parent = prevblk
	cur.Height = prevblk.Height + 1
	
	prevblk.addChild(cur)
	
	ch.Db.StartTransaction()
	
	// Add this block to the block index
	ch.BlockIndex[cur.BlockHash.BIdx()] = cur

	// Update the end of the tree
	if ch.BlockTreeEnd==prevblk {
		ch.BlockTreeEnd = cur
		if don(DBG_BLOCKS) {
			fmt.Printf("Adding block %s @ %d\n", cur.BlockHash.String(), cur.Height)
		}
		e = ch.CommitTransactions(bl, cur.Height)
		if e != nil {
			fmt.Println("rejecting block", cur.Height, cur.BlockHash.String())
			fmt.Println("parent:", NewUint256(p[:]).String())
			ch.unspent.UnwindBlock(cur.Height)
			ch.BlockTreeEnd = ch.BlockTreeEnd.parent
		}
	} else {
		if don(DBG_BLOCKS|DBG_ORPHAS) {
			fmt.Printf("Orphan block %s @ %d\n", cur.BlockHash.String(), cur.Height)
		}
		cur.Orphan = true
		ch.Db.BlockOrphan(bl.Hash, 1)
		if cur.Height > ch.BlockTreeEnd.Height {
			e = ch.MoveToBranch(cur)
			/*return errors.New(fmt.Sprintf("The different branch is longer now %d/%d!\n",
				cur.Height, ch.BlockTreeEnd.Height))*/
		}
	}

	if e == nil {
		ch.Db.BlockAdd(cur.Height, bl)
		ch.Db.CommitTransaction()
	} else {
		delete(ch.BlockIndex, cur.BlockHash.BIdx())
		ch.Db.RollbackTransaction()
	}
	// TODO: Check proof of work
	// TODO: Check timestamp against prev
	
	// Assume that all transactions are finalized

	return 
}


func (ch *Chain)Stats() (s string) {
	return ch.Db.GetStats()
}

func (ch *Chain)GetHeight() uint32 {
	return ch.BlockTreeEnd.Height
}


func (ch *Chain) orphanTree(cur *BlockTreeNode) {
	cur.Orphan = true
	ch.Db.BlockOrphan(&cur.BlockHash, 1)
	for i := range cur.childs {
		ch.orphanTree(cur.childs[i])
	}
}


func (ch *Chain)Rescan() {
	var bl *Block
	println("Rescanning blocks...")
	ch.Db.UnspentPurge()

	cur := ch.BlockTreeRoot
	for cur!=nil {
		cur.Orphan = false
		
		// Read block
		b, e := ch.Db.BlockGet(&cur.BlockHash)
		if b==nil || e!=nil {
			panic("BlockGet failed")
		}
		
		nxt := cur.FindLongestChild()
		if nxt == nil {
			ch.BlockTreeEnd = cur
			//break // Last block
		} else if cur.Height+1 != nxt.Height {
			println("height error", cur.Height+1, nxt.Height, len(cur.childs))
			os.Exit(1)
		}
		
		if (cur.Height%10000)==0 {
			println(cur.Height)  // progress indicator
		}
		
		// mark all the orphans
		for i := range cur.childs {
			if cur.childs[i]!=nxt {
				//println("orphan tree @", cur.childs[i].Height)
				ch.orphanTree(cur.childs[i])
			}
		}

		bl, e = NewBlock(b[:])
		if e != nil {
			panic("Rescan: NewBlock error")
			return
		}
		//fmt.Println("\nBlock", cur.Height, cur.BlockHash.String(), nxt)

		e = bl.CheckBlock()
		if e != nil {
			panic("Rescan: CheckBlock error:"+e.Error())
		}

		//ch.Db.StartTransaction()
		e = ch.CommitTransactions(bl, cur.Height)
		if e != nil {
			panic("Rescan: CommitTransactions error:"+e.Error())
		}
		//ch.Db.CommitTransaction()

		cur = nxt
	}

	println("block Index rescan done", ch.BlockTreeEnd.Height)
}


func nextBlock(ch *Chain, hash, prev []byte, orph int) {
	bh := NewUint256(hash[:])
	ph := NewUint256(prev[:])
	_, ok := ch.BlockIndex[bh.BIdx()]
	if ok {
		println("nextBlock:", bh.String(), "- already in")
		return
	}
	
	v := new(BlockTreeNode)
	v.BlockHash = *bh
	v.parent, ok = ch.BlockIndex[ph.BIdx()]
	if !ok {
		println("Mid: no such parent", ph.String(), " - hook to ", bh.String())
		os.Exit(1)
	}
	v.Height = v.parent.Height + 1
	v.parent.addChild(v)
	v.Orphan = (orph!=0)
	ch.BlockIndex[v.BlockHash.BIdx()] = v
	if !v.Orphan {
		ch.BlockTreeEnd = v // they shoudl come in block height order
	}
}


func (ch *Chain)loadIndex() {
	println("Loading Index...")
	ch.Db.LoadBlockIndex(ch, nextBlock)
	println("block Index loaded")
}

func (ch *Chain) LookUnspent(tid [32]byte, vout uint32) *TxOut {
	txin := TxPrevOut{Hash:tid, Vout:vout}
	return ch.unspent.LookUnspent(&txin)
}

func (ch *Chain) GetUnspentFromPkScr(scr []byte) []OneUnspentTx {
	return ch.unspent.GetUnspentFromPkScr(scr)
}


