package btc

import (
	"errors"
	"os"
	"fmt"
	"time"
	"math/rand"
)

const blockMapLen = 8  // The bigger it is, the more memory is needed, but lower chance of a collision

var TestRollback bool


type BlockTreeNode struct {
	BlockHash *Uint256
	Height uint32
	parenHash *Uint256
	parent *BlockTreeNode
	childs []*BlockTreeNode
}

type Chain struct {
	Db BtcDB
	
	BlockTreeRoot *BlockTreeNode
	BlockTreeEnd *BlockTreeNode
	Genesis *Uint256

	BlockIndex map[[blockMapLen]byte] *BlockTreeNode
}


func NewChain(genesis *Uint256, rescan bool) (ch *Chain) {
	ch = new(Chain)
	ch.Genesis = genesis
	ch.Db = NewDb()
	if ch.loadBlockIndex() || rescan {
		ch.rescan()
	}
	return 
}


func NewBlockIndex(h []byte) (o [blockMapLen]byte) {
	copy(o[:], h[:blockMapLen])
	return 
}


func (ch *Chain)ProcessBlockTransactions(bl *Block, height uint32) (changes *BlockChanges, e error) {
	changes = new(BlockChanges)
	changes.Height = height
	e = ch.commitTxs(bl, changes)
	return
}


func (ch *Chain)PickUnspent(txin *TxPrevOut) (*TxOut) {
	o, e := ch.Db.UnspentGet(txin)
	if e == nil {
		return o
	}
	return nil
}


func (ch *Chain)commitTxs(bl *Block, changes *BlockChanges) (error) {
	sumblockin := GetBlockReward(changes.Height)
	sumblockout := uint64(0)
	
	if don(DBG_TX) {
		fmt.Printf("Commiting %d transactions\n", len(bl.Txs))
	}
	
	// Add each tx outs from the current block to the temporary pool
	blUnsp := make(map[[32]byte] []*TxOut, len(bl.Txs))
	for i := range bl.Txs {
		outs := make([]*TxOut, len(bl.Txs[i].TxOut))
		for j := range bl.Txs[i].TxOut {
			outs[j] = bl.Txs[i].TxOut[j]
		}
		blUnsp[bl.Txs[i].Hash.Hash] = outs
	}

	for i := range bl.Txs {
		var txoutsum, txinsum uint64
		if don(DBG_TX) {
			fmt.Printf("tx %d/%d:\n", i+1, len(bl.Txs))
		}

		// Check each tx for a valid input
		for j := range bl.Txs[i].TxIn {
			if i>0 {
				inp := &bl.Txs[i].TxIn[j].Input
				tout := ch.PickUnspent(inp)
				if tout==nil {
					t, ok := blUnsp[inp.Hash]
					if !ok {
						println("Unknown txin", inp.String())
						return errors.New("Unknown input")
						println(ch.Db.GetStats())
					}

					if inp.Vout>=uint32(len(t)) {
						println("Vout too big", len(t), inp.String())
						return errors.New("Vout too big")
					}

					if t[inp.Vout] == nil {
						println("Vout already spent", inp.String())
						return errors.New("Vout already spent")
					}

					tout = t[inp.Vout]
					t[inp.Vout] = nil // and now mark it as spent:
					//println("TxInput from the current block", inp.String())
				}
				// Verify Transaction script:
				
				if !VerifyTxScript(bl.Txs[i].TxIn[j].ScriptSig, tout) {
					fmt.Printf("Transaction signature error in block %d!\n", changes.Height)
					os.Exit(1)
				}

				txinsum += tout.Value
				changes.DeledTxs = append(changes.DeledTxs,
					&OneAddedTx{Tx_Adr:&bl.Txs[i].TxIn[j].Input, Val_Pk:tout})

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

		for j := range bl.Txs[i].TxOut {
			if don(DBG_TX) {
				fmt.Printf("  out %d: %12.8f\n", j+1, float64(bl.Txs[i].TxOut[j].Value)/1e8)
			}
			txoutsum += bl.Txs[i].TxOut[j].Value
			txa := new(TxPrevOut)
			copy(txa.Hash[:], bl.Txs[i].Hash.Hash[:])
			txa.Vout = uint32(j)
			changes.AddedTxs = append(changes.AddedTxs, &OneAddedTx{Tx_Adr:txa, Val_Pk:bl.Txs[i].TxOut[j]})
			
		}
		sumblockout += txoutsum
		
		if don(DBG_TX) {
			fmt.Sprintf("  %12.8f -> %12.8f  (%.8f)\n", 
				float64(txinsum)/1e8, float64(txoutsum)/1e8,
				float64(txinsum-txoutsum)/1e8)
		}
	}

	if sumblockin < sumblockout {
		return errors.New(fmt.Sprintf("Out:%d > In:%d", sumblockout, sumblockin))
	} else if don(DBG_WASTED) && sumblockin != sumblockout {
		fmt.Printf("%.8f BTC wasted in block %d\n", float64(sumblockin-sumblockout)/1e8, changes.Height)
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
	panic("MoveToBranch not implemented")
	fmt.Printf("Moving branches %d -> %d\n", ch.BlockTreeEnd.Height, cur.Height)

	old := ch.BlockTreeEnd
	
	for cur.Height > old.Height {
		cur = cur.parent
	}
	
	for old!=cur {
		ch.Db.UndoBlockTransactions(old.Height)
		
		fmt.Printf("->orph block %s @ %d\n", old.BlockHash.String(), old.Height)
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
		
		b, er := ch.Db.BlockGet(cur.BlockHash)
		if er != nil {
			return er
		}

		bl, er := NewBlock(b)
		if er != nil {
			return er
		}

		fmt.Println("  ... Got block ", bl.Hash.String())
		bl.BuildTxList()

		changes, er := ch.ProcessBlockTransactions(bl, cur.Height)
		if er != nil {
			return er
		}
		ch.Db.CommitBlockTxs(changes)
		fmt.Printf("  ... %d new txs commited\n", len(bl.Txs))

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

	prevblk, ok := ch.BlockIndex[bl.GetParent().BIdx()]
	if !ok {
		return errors.New("AcceptBlock() : parent not found : "+bl.GetParent().String())
	}

	// create new BlockTreeNode
	cur := new(BlockTreeNode)
	cur.BlockHash = bl.Hash
	cur.parent = prevblk
	cur.Height = prevblk.Height + 1
	
	prevblk.addChild(cur)
	
	// Add this block to the block index
	ch.BlockIndex[cur.BlockHash.BIdx()] = cur

	// Store the block in the persistent storage
	ch.Db.BlockAdd(cur.Height, bl)
	
	// Update the end of the tree
	if ch.BlockTreeEnd==prevblk {
		ch.BlockTreeEnd = cur
		if don(DBG_BLOCKS) {
			fmt.Printf("Adding block %s @ %d\n", cur.BlockHash.String(), cur.Height)
		}
		changes, e := ch.ProcessBlockTransactions(bl, cur.Height)
		if e != nil {
			fmt.Println("rejecting block", cur.Height, cur.BlockHash.String())
			fmt.Println("parent:", bl.GetParent().String())
			ch.BlockTreeEnd = ch.BlockTreeEnd.parent
		} else {
			ch.Db.CommitBlockTxs(changes)
		}
	} else {
		if don(DBG_BLOCKS|DBG_ORPHAS) {
			fmt.Printf("Orphan block %s @ %d\n", cur.BlockHash.String(), cur.Height)
		}
		if cur.Height > ch.BlockTreeEnd.Height {
			e = ch.MoveToBranch(cur)
			/*return errors.New(fmt.Sprintf("The different branch is longer now %d/%d!\n",
				cur.Height, ch.BlockTreeEnd.Height))*/
		}
	}

	if e != nil {
		delete(ch.BlockIndex, cur.BlockHash.BIdx())
	}
	// TODO: Check proof of work
	// TODO: Check timestamp against prev
	
	// Assume that all transactions are finalized

	return 
}


func (ch *Chain)Stats() (s string) {
	s = fmt.Sprintf("CHAIN: tot_blocks:%d  max_height:%d\n", len(ch.BlockIndex), ch.BlockTreeEnd.Height)
	s += ch.Db.GetStats()
	return
}

func (ch *Chain)GetHeight() uint32 {
	return ch.BlockTreeEnd.Height
}


func (ch *Chain)Close() {
	ch.Db.Close()
}


func (ch *Chain)rescan() {
	var bl *Block
	println("Rescanning blocks...")
	ch.Db.UnspentPurge()

	if TestRollback {
		rand.Seed(time.Now().UnixNano())
	}

	cur := ch.BlockTreeRoot
	sta := time.Now()
	for cur!=nil {
		if TestRollback && rand.Intn(1000)==0 {
			n := 1+rand.Intn(144)
			println(cur.Height, "Rollback", n, "blocks")
			for n>0 && cur.parent!=nil {
				cur = cur.parent
				ch.Db.UndoBlockTransactions(cur.Height)
				n--
			}
		}
		
		// Read block
		b, e := ch.Db.BlockGet(cur.BlockHash)
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
		
		/*if cur.Height==70e3 {
			println("aborting sooner")
			break
		}*/

		if (cur.Height%10000)==0 {
			println(cur.Height)  // progress indicator
		}
		
		bl, e = NewBlock(b[:])
		if e != nil {
			panic("Rescan: NewBlock error")
			return
		}
		//fmt.Println("\nBlock", cur.Height, cur.BlockHash.String(), nxt)

		e = bl.CheckBlock()
		if e != nil {
			panic(e.Error())
		}

		//ch.Db.StartTransaction()
		changes, e := ch.ProcessBlockTransactions(bl, cur.Height)
		if e != nil {
			panic(e.Error())
		}
		ch.Db.CommitBlockTxs(changes)

		cur = nxt
	}
	sto := time.Now()

	println("block Index rescan done", ch.BlockTreeEnd.Height)
	println("operation took", sto.Unix()-sta.Unix(), "seconds")
}


func nextBlock(ch *Chain, hash, prev []byte, height uint32) {
	bh := NewUint256(hash[:])
	_, ok := ch.BlockIndex[bh.BIdx()]
	if ok {
		println("nextBlock:", bh.String(), "- already in")
		return
	}

	v := new(BlockTreeNode)
	v.BlockHash = bh
	v.parenHash = NewUint256(prev[:])
	v.Height = height
	ch.BlockIndex[v.BlockHash.BIdx()] = v
}


func (ch *Chain)loadBlockIndex() bool {
	ch.BlockIndex = make(map[[blockMapLen]byte]*BlockTreeNode, BlockMapInitLen)
	ch.BlockTreeRoot = new(BlockTreeNode)
	ch.BlockTreeRoot.BlockHash = ch.Genesis
	ch.BlockIndex[NewBlockIndex(ch.Genesis.Hash[:])] = ch.BlockTreeRoot
	ch.BlockTreeEnd = nil
	println("Loading Index...", len(ch.BlockIndex))
	ch.Db.LoadBlockIndex(ch, nextBlock)
	println("Building block tree", len(ch.BlockIndex))
	var mh, mhcnt uint32
	for _, v := range ch.BlockIndex {
		if v==ch.BlockTreeRoot {
			println(" - skip root block (should be only one)")
			continue
		}
		par, ok := ch.BlockIndex[v.parenHash.BIdx()]
		if !ok {
			panic(v.BlockHash.String()+" has no parent "+v.parenHash.String())
		}
		v.parent = par
		v.parent.addChild(v)
		v.parenHash = nil
		if v.Height>mh {
			mh = v.Height
			mhcnt = 0
			ch.BlockTreeEnd = v
		} else if v.Height==mh {
			mhcnt++
		}
	}
	println("Done", len(ch.BlockIndex), mh, mhcnt)
	return mhcnt>0
}


/*
func (ch *Chain) LookUnspent(tid [32]byte, vout uint32) *TxOut {
	txin := TxPrevOut{Hash:tid, Vout:vout}
	return ch.unspent.LookUnspent(&txin)
}
*/

func (ch *Chain) GetUnspentFromPkScr(scr []byte) []OneUnspentTx {
	return ch.Db.GetUnspentFromPkScr(scr)
}

