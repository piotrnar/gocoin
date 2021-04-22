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

	Trusted bool
}

func (ch *Chain) ParseTillBlock(end *BlockTreeNode) {
	var crec *BlckCachRec
	var er error
	var trusted bool
	var tot_bytes uint64

	last := ch.LastBlock()
	var total_size_to_process uint64
	fmt.Print("Calculating size of blockchain overhead...")
	for n := end; n != nil && n != last; n = n.Parent {
		l, _ := ch.Blocks.BlockLength(n.BlockHash, false)
		total_size_to_process += uint64(l)
	}
	fmt.Println("\rApplying", total_size_to_process>>20, "MB of transactions data from", end.Height-last.Height, "blocks to UTXO.db")
	sta := time.Now()
	prv := sta
	for !AbortNow && last != end {
		cur := time.Now()
		if cur.Sub(prv) >= 10 * time.Second {
			mbps := float64(tot_bytes) / float64(cur.Sub(sta)/1e3)
			sec_left := int64(float64(total_size_to_process)/1e6 / mbps)
			fmt.Printf("ParseTillBlock %d / %d ... %.2f MB/s - %d:%02d:%02d left (%d)\n", last.Height,
				end.Height, mbps, sec_left/3600, (sec_left/60)%60, sec_left%60, cur.Unix()-sta.Unix())
			prv = cur
		}

		nxt := last.FindPathTo(end)
		if nxt == nil {
			break
		}

		if nxt.BlockSize==0 {
			println("ParseTillBlock: ", nxt.Height, nxt.BlockHash.String(), "- not yet commited")
			break
		}

		crec, trusted, er = ch.Blocks.BlockGetInternal(nxt.BlockHash, true)
		if er != nil {
			panic("Db.BlockGet(): "+er.Error())
		}
		tot_bytes += uint64(len(crec.Data))
		l, _ := ch.Blocks.BlockLength(nxt.BlockHash, false)
		total_size_to_process -= uint64(l)

		bl, er := btc.NewBlock(crec.Data)
		if er != nil {
			ch.DeleteBranch(nxt, nil)
			break
		}
		bl.Height = nxt.Height

		// Recover the flags to be used when verifying scripts for non-trusted blocks (stored orphaned blocks)
		ch.ApplyBlockFlags(bl)

		// Do not recover MedianPastTime as it is only checked in PostCheckBlock()
		// that had to be done before the block was stored on disk.

		er = bl.BuildTxList()
		if er != nil {
			ch.DeleteBranch(nxt, nil)
			break
		}

		bl.Trusted = trusted

		changes, sigopscost, er := ch.ProcessBlockTransactions(bl, nxt.Height, end.Height)
		if er != nil {
			println("ProcessBlockTransactionsB", nxt.BlockHash.String(), nxt.Height, er.Error())
			ch.DeleteBranch(nxt, nil)
			break
		}
		nxt.SigopsCost = sigopscost
		if !trusted {
			ch.Blocks.BlockTrusted(bl.Hash.Hash[:])
		}

		ch.Unspent.CommitBlockTxs(changes, bl.Hash.Hash[:])

		ch.SetLast(nxt)
		last = nxt

		if ch.CB.BlockMinedCB != nil {
			bl.Height = nxt.Height
			bl.LastKnownHeight = end.Height
			ch.CB.BlockMinedCB(bl)
		}
	}

	if !AbortNow && last != end {
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

// GetMedianTimePast returns the median time of the last 11 blocks.
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


// FindFarthestNode looks for the farthest node.
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


// FindPathTo returns the next node that leads to the given destination.
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


// HasAllParents checks whether the given node has all its parent blocks already comitted.
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


// OnActiveBranch returns true if the given node is on the active branch.
func (ch *Chain) OnActiveBranch(dst *BlockTreeNode) bool {
	top := ch.LastBlock()
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


// MoveToBlock performs a channel reorg.
func (ch *Chain) MoveToBlock(dst *BlockTreeNode) {
	cur := dst
	for cur.Height > ch.LastBlock().Height {
		cur = cur.Parent

		// if cur.TxCount is zero, it means we dont yet have this block's data
		if cur.TxCount==0 {
			fmt.Println("MoveToBlock cannot continue A")
			fmt.Println("Trying to go:", dst.BlockHash.String())
			fmt.Println("Cannot go at:", cur.BlockHash.String())
			return
		}
	}

	// At this point both "ch.blockTreeEnd" and "cur" should be at the same height
	for tmp := ch.LastBlock(); tmp != cur; tmp = tmp.Parent {
		if cur.Parent.TxCount==0 {
			fmt.Println("MoveToBlock cannot continue B")
			fmt.Println("Trying to go:", dst.BlockHash.String())
			fmt.Println("Cannot go at:", cur.Parent.BlockHash.String())
			return
		}
		cur = cur.Parent
	}

	// At this point "cur" is at the highest common block
	for ch.LastBlock() != cur {
		if AbortNow {
			return
		}
		ch.UndoLastBlock()
	}
	ch.ParseTillBlock(dst)
}


func (ch *Chain) UndoLastBlock() {
	last := ch.LastBlock()
	fmt.Println("Undo block", last.Height, last.BlockHash.String(), last.BlockSize>>10, "KB")

	crec, _, er := ch.Blocks.BlockGetInternal(last.BlockHash, true)
	if er != nil {
		panic(er.Error())
	}

	bl, er := btc.NewBlock(crec.Data)
	if er != nil {
		panic("UndoLastBlock: NewBlock() should not fail with block from disk")
	}
	
	er = bl.BuildTxList()
	if er != nil {
		panic("UndoLastBlock: BuildTxList() should not fail with block from disk")
	}

	ch.Unspent.UndoBlockTxs(bl, last.Parent.BlockHash.Hash[:])
	ch.SetLast(last.Parent)
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
