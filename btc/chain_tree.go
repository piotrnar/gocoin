package btc

import (
	"fmt"
	"time"
)


type BlockTreeNode struct {
	BlockHash *Uint256
	Height uint32
	Bits uint32
	Timestamp uint32
	parenHash *Uint256
	parent *BlockTreeNode
	childs []*BlockTreeNode
}


func (ch *Chain) ParseTillBlock(end *BlockTreeNode) {
	var b []byte
	var er error
	var trusted bool
	
	prv_sync := ch.DoNotSync
	ch.DoNotSync = true

	sta := time.Now().Unix()
	ChSta("ParseTillBlock")
	for ch.BlockTreeEnd != end {
		cur := time.Now().Unix()
		if cur-sta >= 10 {
			fmt.Println("ParseTillBlock ...", ch.BlockTreeEnd.Height, "/", end.Height)
			sta = cur
		}

		nxt := ch.BlockTreeEnd.FindPathTo(end)
		if nxt == nil {
			break
		}
		
		b, trusted, er = ch.Blocks.BlockGet(nxt.BlockHash)
		if er != nil {
			panic("Db.BlockGet(): "+er.Error())
		}

		bl, er := NewBlock(b)  
		if er != nil {
			ch.DeleteBranch(nxt)
			break
		}

		bl.Trusted = trusted

		bl.BuildTxList()

		changes, er := ch.ProcessBlockTransactions(bl, nxt.Height)
		if er != nil {
			ch.DeleteBranch(nxt)
			break
		}
		ch.Blocks.BlockTrusted(bl.Hash.Hash[:])
		if !ch.DoNotSync {
			ch.Blocks.Sync()
		}
		ch.Unspent.CommitBlockTxs(changes, bl.Hash.Hash[:])

		ch.BlockTreeEnd = nxt
	}
	ChSto("ParseTillBlock")
	
	if ch.BlockTreeEnd != end {
		end, _ = ch.BlockTreeRoot.FindFarthestNode()
		fmt.Println("ParseTillBlock failed - now go to", end.Height)
		ch.MoveToBlock(end)
	}
	ch.DoNotSync = prv_sync
}


// Looks for the fartherst node
func (n *BlockTreeNode) FindFarthestNode() (*BlockTreeNode, int) {
	//fmt.Println("FFN:", n.Height, "kids:", len(n.childs))
	if len(n.childs)==0 {
		return n, 0
	}
	res, depth := n.childs[0].FindFarthestNode()
	if len(n.childs) > 1 {
		for i := 1; i<len(n.childs); i++ {
			_re, _dept := n.childs[i].FindFarthestNode()
			if _dept > depth {
				res = _re
				depth = _dept
			}
		}
	}
	return res, depth+1
}


// Returns the next node that leads to the given destiantion
func (n *BlockTreeNode)FindPathTo(end *BlockTreeNode) (*BlockTreeNode) {
	if n==end {
		return nil
	}
	
	if end.Height <= n.Height {
		panic("End block is not higher then current")
	}

	if len(n.childs)==0 {
		panic("Unknown path to block " + end.BlockHash.String() )
	}
	
	if len(n.childs)==1 {
		return n.childs[0]  // if there is only one child, do it fast
	}

	for {
		// more then one children: go fomr the end until you reach the current node
		if end.parent==n {
			return end
		}
		end = end.parent
	}

	return nil
}


func (ch *Chain)MoveToBlock(dst *BlockTreeNode) {
	fmt.Printf("MoveToBlock: %d -> %d\n", ch.BlockTreeEnd.Height, dst.Height)

	cur := dst
	for cur.Height > ch.BlockTreeEnd.Height {
		cur = cur.parent
	}
	// At this point both "ch.BlockTreeEnd" and "cur" should be at the same height
	for ch.BlockTreeEnd != cur {
		if don(DBG_ORPHAS) {
			fmt.Printf("->orph block %s @ %d\n", ch.BlockTreeEnd.BlockHash.String(), 
				ch.BlockTreeEnd.Height)
		}
		ch.Unspent.UndoBlockTransactions(ch.BlockTreeEnd.Height, ch.BlockTreeEnd.parent.BlockHash.Hash[:])
		ch.BlockTreeEnd = ch.BlockTreeEnd.parent
		cur = cur.parent
	}
	fmt.Printf("Reached common node @ %d\n", ch.BlockTreeEnd.Height)
	ch.ParseTillBlock(dst)
}


func (cur *BlockTreeNode) delAllChildren() {
	for i := range cur.childs {
		cur.childs[i].delAllChildren()
	}
}


func (ch *Chain) DeleteBranch(cur *BlockTreeNode) {
	// first disconnect it from the parent
	delete(ch.BlockIndex, cur.BlockHash.BIdx())
	cur.parent.delChild(cur)
	cur.delAllChildren()
	ch.Blocks.BlockInvalid(cur.BlockHash.Hash[:])
	if !ch.DoNotSync {
		ch.Blocks.Sync()
	}
}


func (n *BlockTreeNode)addChild(c *BlockTreeNode) {
	n.childs = append(n.childs, c)
}


func (n *BlockTreeNode)delChild(c *BlockTreeNode) {
	newChds := make([]*BlockTreeNode, len(n.childs)-1)
	xxx := 0
	for i := range n.childs {
		if n.childs[i]!=c {
			newChds[xxx] = n.childs[i]
			xxx++
		}
	}
	if xxx!=len(n.childs)-1 {
		panic("Child not found")
	}
	n.childs = newChds
}



