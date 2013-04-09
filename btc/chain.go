package btc

import (
	"errors"
	"bytes"
	"os"
	"fmt"
)

const blockMapLen = 8  // The bigger it is, the more memory is needed, but lower chance of a collision


type BlockTreeNode struct {
	BlockHash Uint256
	Height uint32
	parent *BlockTreeNode
	childs []*BlockTreeNode
}

type Chain struct {
	
	BlockTreeRoot *BlockTreeNode
	BlockTreeEnd *BlockTreeNode

	BlockIndex map[[blockMapLen]byte] *BlockTreeNode
	OrphanedBlocks map[[blockMapLen]byte] *BlockTreeNode
	
	blockdb *BlockDB;
	unspent *UnspentDb
}


func NewChain(blockdb *BlockDB, genesis *Uint256) (ch *Chain) {
	ch = new(Chain)
	ch.blockdb = blockdb
	ch.BlockIndex = make(map[[blockMapLen]byte] *BlockTreeNode, BlockMapInitLen)
	
	ch.BlockTreeRoot = new(BlockTreeNode)
	ch.BlockTreeRoot.BlockHash = *genesis
	ch.BlockIndex[NewBlockIndex(genesis.Hash[:])] = ch.BlockTreeRoot
	ch.BlockTreeEnd = ch.BlockTreeRoot
	ch.OrphanedBlocks = make(map[[blockMapLen]byte] *BlockTreeNode)
	ch.unspent = NewUnspentDb()
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
	fmt.Printf("Moving branches %d -> %d\n", ch.BlockTreeEnd.Height, cur.Height)

	old := ch.BlockTreeEnd
	
	for cur.Height > old.Height {
		cur = cur.parent
	}
	
	for old!=cur {
		ch.unspent.UnwindBlock(old.Height)
		
		fmt.Printf("->orph block %s @ %d\n", old.BlockHash.String(), old.Height)
		ch.OrphanedBlocks[old.BlockHash.BIdx()] = old
		
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
		
		b, er := ch.blockdb.GetBlock(&cur.BlockHash)
		errorFatal(er, "MoveToBranch/GetBlock")

		bl, er := NewBlock(b)
		errorFatal(er, "MoveToBranch/NewBlock")

		fmt.Println("  ... Got block ", bl.Hash.String())
		bl.BuildTxList()

		er = ch.CommitTransactions(bl, cur.Height)
		errorFatal(er, "MoveToBranch/CommitTransactions")
		fmt.Printf("  ... %d new txs commited\n", len(bl.Txs))

		delete(ch.OrphanedBlocks, cur.BlockHash.BIdx())
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
		return errors.New("AcceptBlock() : prev block not found"+NewUint256(p[:]).String())
	}

	// create new BlockTreeNode
	cur := new(BlockTreeNode)
	cur.BlockHash = *bl.Hash
	cur.parent = prevblk
	cur.Height = prevblk.Height + 1
	
	prevblk.addChild(cur)
	
	// Add this block to the block index
	ch.BlockIndex[cur.BlockHash.BIdx()] = cur

	// Update the end of the tree
	if ch.BlockTreeEnd==prevblk {
		ch.BlockTreeEnd = cur
		if don(DBG_BLOCKS) {
			fmt.Printf("Adding block %s @ %d\n", cur.BlockHash.String(), cur.Height)
		}
	} else {
		if don(DBG_BLOCKS|DBG_ORPHAS) {
			fmt.Printf("Orphan block %s @ %d\n", cur.BlockHash.String(), cur.Height)
		}
		ch.OrphanedBlocks[bl.Hash.BIdx()] = cur
		if cur.Height > ch.BlockTreeEnd.Height {
			ch.MoveToBranch(cur)
			/*return errors.New(fmt.Sprintf("The different branch is longer now %d/%d!\n",
				cur.Height, ch.BlockTreeEnd.Height))*/
		}
		return nil
	}

	// TODO: Check proof of work
	// TODO: Check timestamp against prev
	
	// Assume that all transactions are finalized

	e = ch.CommitTransactions(bl, cur.Height)
	if e != nil {
		println("rejecting block", cur.Height, cur.BlockHash.String(), 
			"\nparent:", NewUint256(p[:]).String(),
			"\n", e.Error())
		ch.unspent.UnwindBlock(cur.Height)
		delete(ch.BlockIndex, cur.BlockHash.BIdx())
		ch.BlockTreeEnd = ch.BlockTreeEnd.parent
	}
	return 
}


func (ch *Chain)Stats() (s string) {
	siz := uint64(len(ch.OrphanedBlocks)+len(ch.BlockIndex)) * (32 + 4 + 8 + 16)
	s = fmt.Sprintf("BCHAIN  : height=%d OrphanedBlocks=%d/%d  siz:~%dMB\n", 
		ch.BlockTreeEnd.Height, len(ch.OrphanedBlocks), len(ch.BlockIndex),
			siz >> 20 )
	s += ch.unspent.Stats()
	return
}

func (ch *Chain)GetHeight() uint32 {
	return ch.BlockTreeEnd.Height
}


func (v *BlockTreeNode)Load(f *os.File) (h [32]byte, e error) {
	_, e = f.Read(v.BlockHash.Hash[:])
	if e == nil {
		_, e = f.Read(h[:])
	}
	return 
}


func (v *BlockTreeNode)Save(f *os.File) {
	f.Write(v.BlockHash.Hash[:])
	if v.parent != nil {
		f.Write(v.parent.BlockHash.Hash[:])
	} else {
		f.Write(bytes.Repeat([]byte{0}, 32))
	}
}

func (ch *Chain)loadIndex(f *os.File) {
	var k, h [32]byte
	var orph [1]byte
	var ok bool
	var e error
	for {
		v := new(BlockTreeNode)
		h, e = v.Load(f)
		if e != nil {
			break
		}

		if allzeros(h[:]) {
			ch.BlockTreeRoot = v
		} else {
			v.parent, ok = ch.BlockIndex[NewBlockIndex(h[:])]
			if !ok {
				println("Mid: no such parent", NewUint256(k[:]).String(), " - hook to ", NewUint256(h[:]).String())
				os.Exit(1)
			}
			v.Height = v.parent.Height + 1
			v.parent.addChild(v)
		}
		ch.BlockIndex[v.BlockHash.BIdx()] = v

		f.Read(orph[:])
		if (orph[0]!=0) {
			ch.OrphanedBlocks[v.BlockHash.BIdx()] = v
		} else {
			ch.BlockTreeEnd = v // they shoudl be saved in order
		}
	}
	
	println(len(ch.BlockIndex), "loaded into blidxDB. OrphanedBlocks:", len(ch.OrphanedBlocks))
}


func (ch *Chain)Load() {
	f, e := os.Open("blockdb.bin")
	errorFatal(e, "cannot open blockdb.bin")
	ch.blockdb.Load(f)
	f.Close()

	f, e = os.Open("unspent.bin")
	errorFatal(e, "cannot open unspent.bin")
	ch.unspent.Load(f)
	f.Close()

	f, e = os.Open("unwinds.bin")
	errorFatal(e, "cannot open unwinds.bin")
	ch.unspent.unwd.Load(f)
	f.Close()
	
	ch.BlockIndex = make(map[[blockMapLen]byte] *BlockTreeNode, BlockMapInitLen)
	ch.OrphanedBlocks = make(map[[blockMapLen]byte] *BlockTreeNode)
	f, e = os.Open("blookup.bin")
	errorFatal(e, "cannot open blookup.bin")
	ch.loadIndex(f)
	f.Close()
}


func (ch *Chain)SaveBlockIndexTree(f *os.File, v *BlockTreeNode) (cnt uint32) {
	v.Save(f)
	_, orphnd := ch.OrphanedBlocks[v.BlockHash.BIdx()]
	if orphnd {
		f.Write([]byte{1})
	} else {
		f.Write([]byte{0})
	}
	cnt = 1
	for i := range v.childs {
		cnt += ch.SaveBlockIndexTree(f, v.childs[i])
	}
	return 
}


func (ch *Chain)Save() {
	f, e := os.Create("blockdb.bin")
	errorFatal(e, "cannot create blockdb.bin")
	ch.blockdb.Save(f)
	f.Close()

	f, e = os.Create("unspent.bin")
	errorFatal(e, "cannot create unspent.bin")
	ch.unspent.Save(f)
	f.Close()
	
	f, e = os.Create("unwinds.bin")
	errorFatal(e, "cannot create unwinds.bin")
	ch.unspent.unwd.Save(f)
	f.Close()
	
	f, e = os.Create("blookup.bin")
	errorFatal(e, "cannot create blookup.bin")
	cnt := ch.SaveBlockIndexTree(f, ch.BlockTreeRoot)
	println(cnt, len(ch.BlockIndex), "saved in blidxDB - last ", ch.BlockTreeEnd.BlockHash.String())
	f.Close()
}

func (ch *Chain) LookUnspent(tid [32]byte, vout uint32) *TxOut {
	txin := TxPrevOut{Hash:tid, Vout:vout}
	return ch.unspent.LookUnspent(&txin)
}

func (ch *Chain) GetUnspentFromPkScr(scr []byte) []OneUnspentTx {
	return ch.unspent.GetUnspentFromPkScr(scr)
}


