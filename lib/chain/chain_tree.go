package chain

import (
	"fmt"
	"time"
	"sort"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
)


type BlockTreeNode struct {
	BlockHash *btc.Uint256
	Height uint32
	Parent *BlockTreeNode
	Childs []*BlockTreeNode

	BlockSize uint32 // if this is zero, only header is known so far
	TxCount uint32
	SigopsCost uint32

	BlockHeader [80]byte
}

func (ch *Chain) ParseTillBlock(end *BlockTreeNode) {
	var b []byte
	var er error
	var trusted bool
	var tot_bytes uint64

	ch.Blocks.DoNotCache = true

	sta := time.Now()
	prv := sta

	for !AbortNow && ch.BlockTreeEnd != end {
		cur := time.Now()
		if cur.Sub(prv) >= 10 * time.Second {
			mbps := float64(tot_bytes) / float64(cur.Sub(sta)/1e3)
			fmt.Printf("ParseTillBlock %d / %d ... %.2f MB/s\n", ch.BlockTreeEnd.Height, end.Height, mbps)
			prv = cur
		}

		nxt := ch.BlockTreeEnd.FindPathTo(end)
		if nxt == nil {
			break
		}

		if nxt.BlockSize==0 {
			println("ParseTillBlock: ", nxt.Height, nxt.BlockHash.String(), "- not yet commited")
			break
		}

		b, trusted, er = ch.Blocks.BlockGet(nxt.BlockHash)
		if er != nil {
			panic("Db.BlockGet(): "+er.Error())
		}
		tot_bytes += uint64(len(b))

		bl, er := btc.NewBlock(b)
		if er != nil {
			ch.DeleteBranch(nxt, nil)
			break
		}

		er = bl.BuildTxList()
		if er != nil {
			ch.DeleteBranch(nxt, nil)
			break
		}

		bl.Trusted = trusted

		changes, er := ch.ProcessBlockTransactions(bl, nxt.Height, end.Height)
		if er != nil {
			println("ProcessBlockTransactionsB", nxt.BlockHash.String(), nxt.Height, er.Error())
			ch.DeleteBranch(nxt, nil)
			break
		}
		nxt.SigopsCost = bl.SigopsCost
		if !trusted {
			ch.Blocks.BlockTrusted(bl.Hash.Hash[:])
		}

		ch.Unspent.CommitBlockTxs(changes, bl.Hash.Hash[:])

		ch.BlockTreeEnd = nxt
	}

	ch.Blocks.DoNotCache = false

	if !AbortNow && ch.BlockTreeEnd != end {
		end, _ = ch.BlockTreeRoot.FindFarthestNode()
		fmt.Println("ParseTillBlock failed - now go to", end.Height)
		ch.MoveToBlock(end)
	}
}

func (n *BlockTreeNode) BlockVersion() (uint32) {
	return binary.LittleEndian.Uint32(n.BlockHeader[0:4])
}

func (n *BlockTreeNode) Timestamp() (uint32) {
	return binary.LittleEndian.Uint32(n.BlockHeader[68:72])
}

func (n *BlockTreeNode) Bits() (uint32) {
	return binary.LittleEndian.Uint32(n.BlockHeader[72:76])
}

// Returns median time of the last 11 blocks
func (pindex *BlockTreeNode) GetMedianTimePast() (uint32) {
	var pmedian [MedianTimeSpan]int
	pbegin := MedianTimeSpan
	pend := MedianTimeSpan
	for i:=0; i<MedianTimeSpan && pindex!=nil; i++ {
		pbegin--
		pmedian[pbegin] = int(pindex.Timestamp())
		pindex = pindex.Parent
	}
	sort.Ints(pmedian[pbegin:pend])
	return uint32(pmedian[pbegin+((pend - pbegin)/2)])
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
func (n *BlockTreeNode) FindPathTo(end *BlockTreeNode) (*BlockTreeNode) {
	if n==end {
		return nil
	}

	if end.Height <= n.Height {
		panic("FindPathTo: End block is not higher then current")
	}

	if len(n.Childs)==0 {
		panic("FindPathTo: Unknown path to block " + end.BlockHash.String())
	}

	if len(n.Childs)==1 {
		return n.Childs[0]  // if there is only one child, do it fast
	}

	for {
		// more then one children: go from the end until you reach the current node
		if end.Parent==n {
			return end
		}
		end = end.Parent
	}

	return nil
}


// Check whether the given node has all its parent blocks already comitted
func (ch *Chain) HasAllParents(dst *BlockTreeNode) bool {
	for {
		dst = dst.Parent
		if ch.OnActiveBranch(dst) {
			return true
		}
		if dst==nil || dst.TxCount==0 {
			return false
		}
	}
}


// returns true if the given node is on the active branch
func (ch *Chain) OnActiveBranch(dst *BlockTreeNode) bool {
	top := ch.BlockTreeEnd
	for {
		if dst==top {
			return true
		}
		if dst.Height>=top.Height {
			return false
		}
		top = top.Parent
	}
}


func (ch *Chain) MoveToBlock(dst *BlockTreeNode) {
	cur := dst
	for cur.Height > ch.BlockTreeEnd.Height {
		cur = cur.Parent

		// if cur.TxCount is zero, it means we dont yet have this block's data
		if cur.TxCount==0 {
			fmt.Println("MoveToBlock cannot continue A")
			fmt.Println("Trying to go:", dst.BlockHash.String())
			fmt.Println("Cannot go at:", cur.BlockHash.String())
			return
		}
	}

	// At this point both "ch.BlockTreeEnd" and "cur" should be at the same height
	for tmp := ch.BlockTreeEnd; tmp != cur; tmp = tmp.Parent {
		if cur.Parent.TxCount==0 {
			fmt.Println("MoveToBlock cannot continue B")
			fmt.Println("Trying to go:", dst.BlockHash.String())
			fmt.Println("Cannot go at:", cur.Parent.BlockHash.String())
			return
		}
		cur = cur.Parent
	}

	// At this point "cur" is at the highest common block
	for ch.BlockTreeEnd != cur {
		if AbortNow {
			return
		}
		ch.UndoLastBlock()
	}
	ch.ParseTillBlock(dst)
}


func (ch *Chain) UndoLastBlock() {
	fmt.Println("Undo block", ch.BlockTreeEnd.Height, ch.BlockTreeEnd.BlockHash.String(),
		ch.BlockTreeEnd.BlockSize>>10, "KB")

	raw, _, er := ch.Blocks.BlockGet(ch.BlockTreeEnd.BlockHash)
	if er != nil {
		panic(er.Error())
	}

	bl, _ := btc.NewBlock(raw)
	bl.BuildTxList()

	ch.Unspent.UndoBlockTxs(bl, ch.BlockTreeEnd.Parent.BlockHash.Hash[:])
	ch.BlockTreeEnd = ch.BlockTreeEnd.Parent
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


// make sure ch.BlockIndexAccess is locked before calling it
func (cur *BlockTreeNode) delAllChildren(ch *Chain, deleteCallback func(*btc.Uint256)) {
	for i := range cur.Childs {
		if deleteCallback!=nil {
			deleteCallback(cur.Childs[i].BlockHash)
		}
		cur.Childs[i].delAllChildren(ch, deleteCallback)
		delete(ch.BlockIndex, cur.Childs[i].BlockHash.BIdx())
		ch.Blocks.BlockInvalid(cur.BlockHash.Hash[:])
	}
	cur.Childs = nil
}


func (ch *Chain) DeleteBranch(cur *BlockTreeNode, deleteCallback func(*btc.Uint256)) {
	// first disconnect it from the Parent
	ch.Blocks.BlockInvalid(cur.BlockHash.Hash[:])
	ch.BlockIndexAccess.Lock()
	delete(ch.BlockIndex, cur.BlockHash.BIdx())
	cur.Parent.delChild(cur)
	cur.delAllChildren(ch, deleteCallback)
	ch.BlockIndexAccess.Unlock()
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
