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
	Parent *BlockTreeNode
	Childs []*BlockTreeNode
	BlockSize uint32
}


func (ch *Chain) ParseTillBlock(end *BlockTreeNode) {
	var b []byte
	var er error
	var trusted bool

	prv := time.Now().UnixNano()
	for !AbortNow && ch.BlockTreeEnd != end {
		cur := time.Now().UnixNano()
		if cur-prv >= 10e9 {
			fmt.Println("ParseTillBlock ...", ch.BlockTreeEnd.Height, "/", end.Height)
			prv = cur
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

		er = bl.BuildTxList()
		if er != nil {
			ch.DeleteBranch(nxt)
			break
		}

		bl.Trusted = trusted

		changes, er := ch.ProcessBlockTransactions(bl, nxt.Height)
		if er != nil {
			println("ProcessBlockTransactions", nxt.Height, er.Error())
			ch.DeleteBranch(nxt)
			break
		}
		if !trusted {
			ch.Blocks.BlockTrusted(bl.Hash.Hash[:])
		}
		ch.Unspent.CommitBlockTxs(changes, bl.Hash.Hash[:])

		ch.BlockTreeEnd = nxt
	}

	if !AbortNow && ch.BlockTreeEnd != end {
		end, _ = ch.BlockTreeRoot.FindFarthestNode()
		fmt.Println("ParseTillBlock failed - now go to", end.Height)
		ch.MoveToBlock(end)
	}
	ch.Save()
}


// Looks for the fartherst node
func (n *BlockTreeNode) FindFarthestNode() (*BlockTreeNode, int) {
	//fmt.Println("FFN:", n.Height, "kids:", len(n.Childs))
	if len(n.Childs)==0 {
		return n, 0
	}
	res, depth := n.Childs[0].FindFarthestNode()
	if len(n.Childs) > 1 {
		for i := 1; i<len(n.Childs); i++ {
			_re, _dept := n.Childs[i].FindFarthestNode()
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

	if len(n.Childs)==0 {
		panic("Unknown path to block " + end.BlockHash.String() )
	}

	if len(n.Childs)==1 {
		return n.Childs[0]  // if there is only one child, do it fast
	}

	for {
		// more then one children: go fomr the end until you reach the current node
		if end.Parent==n {
			return end
		}
		end = end.Parent
	}

	return nil
}


func (ch *Chain)MoveToBlock(dst *BlockTreeNode) {
	if don(DBG_ORPHAS) {
		fmt.Printf("MoveToBlock: %d -> %d\n", ch.BlockTreeEnd.Height, dst.Height)
	}

	cur := dst
	for cur.Height > ch.BlockTreeEnd.Height {
		cur = cur.Parent
	}
	// At this point both "ch.BlockTreeEnd" and "cur" should be at the same height
	for ch.BlockTreeEnd != cur {
		if AbortNow {
			return
		}
		if don(DBG_ORPHAS) {
			fmt.Printf("->orph block %s @ %d\n", ch.BlockTreeEnd.BlockHash.String(),
				ch.BlockTreeEnd.Height)
		}
		ch.Unspent.UndoBlockTransactions(ch.BlockTreeEnd.Height)
		ch.BlockTreeEnd = ch.BlockTreeEnd.Parent
		cur = cur.Parent
	}
	if don(DBG_ORPHAS) {
		fmt.Printf("Reached common node @ %d\n", ch.BlockTreeEnd.Height)
	}
	ch.ParseTillBlock(dst)
}


// Returns a common parent with the highest height
func (cur *BlockTreeNode)FirstCommonParent(dst *BlockTreeNode) *BlockTreeNode {
	if cur.Height > dst.Height {
		for cur.Height > dst.Height {
			cur = cur.Parent
		}
	} else {
		for cur.Height < dst.Height {
			dst = dst.Parent
		}
	}
	// From this point on, both cur and dst should be at the same height
	for cur != dst {
		cur = cur.Parent
		dst = dst.Parent
	}
	return cur
}


func (cur *BlockTreeNode) delAllChildren() {
	for i := range cur.Childs {
		cur.Childs[i].delAllChildren()
	}
}


func (ch *Chain) DeleteBranch(cur *BlockTreeNode) {
	// first disconnect it from the Parent
	ch.BlockIndexAccess.Lock()
	delete(ch.BlockIndex, cur.BlockHash.BIdx())
	cur.Parent.delChild(cur)
	cur.delAllChildren()
	ch.BlockIndexAccess.Unlock()
	ch.Blocks.BlockInvalid(cur.BlockHash.Hash[:])
	if !ch.DoNotSync {
		ch.Blocks.Sync()
	}
}


func (n *BlockTreeNode)addChild(c *BlockTreeNode) {
	n.Childs = append(n.Childs, c)
}


func (n *BlockTreeNode)delChild(c *BlockTreeNode) {
	newChds := make([]*BlockTreeNode, len(n.Childs)-1)
	xxx := 0
	for i := range n.Childs {
		if n.Childs[i]!=c {
			newChds[xxx] = n.Childs[i]
			xxx++
		}
	}
	if xxx!=len(n.Childs)-1 {
		panic("Child not found")
	}
	n.Childs = newChds
}
