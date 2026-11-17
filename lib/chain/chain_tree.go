package chain

import (
	"encoding/binary"
	"fmt"
	"sort"

	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/sys"
)

type BlockTreeNode struct {
	BlockHash   *btc.Uint256
	Parent      *BlockTreeNode
	Childs      []*BlockTreeNode
	Height      uint32
	BlockSize   uint32 // if this is zero, only header is known so far
	TxCount     uint32
	SigopsCost  uint32
	Trusted     sys.SyncBool
	BlockHeader [80]byte
}

func (ch *Chain) ParseTillBlock(end *BlockTreeNode) {
	var crec *BlckCachRec
	var er error
	var trusted bool
	var tot_bytes uint64

	last := ch.LastBlock()
	if !ch.testnet() {
		fmt.Println("Applying txs from block", last.Height, last.BlockHash.String(), "\n  to", end.Height, end.BlockHash.String())
	}
	for !AbortNow && last != end {
		nxt := ch.FindPathToLocked(last, end)
		if nxt == nil {
			break
		}

		if nxt.BlockSize == 0 {
			fmt.Println("ParseTillBlock: ", nxt.Height, nxt.BlockHash.String(), "- not yet committed")
			break
		}

		crec, trusted, er = ch.Blocks.BlockGetInternal(nxt.BlockHash, true)
		if er != nil {
			panic("Db.BlockGet(): " + er.Error())
		}
		tot_bytes += uint64(len(crec.Data))

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

		bl.Trusted.Store(trusted)

		changes, sigopscost, er := ch.ProcessBlockTransactions(bl, nxt.Height, end.Height)
		if er != nil {
			bl.Clean()
			fmt.Println("ProcessBlockTransactionsB", nxt.BlockHash.String(), nxt.Height, er.Error())
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
		bl.Clean()
	}

	if !AbortNow && last != end {
		// Go to the best block that we can actually get to (ignoring the nodes with only a header known).
		ch.BlockIndexAccess.Lock()
		nend, _ := ch.BlockTreeRoot.findFarthestCommitted()
		ch.BlockIndexAccess.Unlock()
		if nend == last || nend == end {
			// Nothing better to go to - stay where we are, instead of trying the same thing forever.
			fmt.Println("ParseTillBlock failed - staying at", last.Height)
			return
		}
		fmt.Println("ParseTillBlock failed - now go to", nend.Height)
		ch.MoveToBlock(nend)
	}
}

// findFarthestCommitted works like FindFarthestNode, except that it only follows
// the nodes which have their block's data (not only the header) already committed.
func (n *BlockTreeNode) findFarthestCommitted() (res *BlockTreeNode, pow float64) {
	return n.findFarthest(func(x *BlockTreeNode) []*BlockTreeNode {
		for i, c := range x.Childs {
			if c.TxCount == 0 { // only the header is known - leave it out
				childs := append([]*BlockTreeNode(nil), x.Childs[:i]...)
				for _, c := range x.Childs[i+1:] {
					if c.TxCount != 0 {
						childs = append(childs, c)
					}
				}
				return childs
			}
		}
		return x.Childs
	})
}

func (n *BlockTreeNode) BlockVersion() uint32 {
	return binary.LittleEndian.Uint32(n.BlockHeader[0:4])
}

func (n *BlockTreeNode) Timestamp() uint32 {
	return binary.LittleEndian.Uint32(n.BlockHeader[68:72])
}

func (n *BlockTreeNode) Bits() uint32 {
	return binary.LittleEndian.Uint32(n.BlockHeader[72:76])
}

// GetMedianTimePast returns the median time of the last 11 blocks.
func (pindex *BlockTreeNode) GetMedianTimePast() uint32 {
	var pmedian [MedianTimeSpan]int
	pbegin := MedianTimeSpan
	pend := MedianTimeSpan
	for i := 0; i < MedianTimeSpan && pindex != nil; i++ {
		pbegin--
		pmedian[pbegin] = int(pindex.Timestamp())
		pindex = pindex.Parent
	}
	sort.Ints(pmedian[pbegin:pend])
	return uint32(pmedian[pbegin+((pend-pbegin)/2)])
}

// FindFarthestNode looks for the farthest node - the one with the most work on the way to it.
// The second returned value is the amount of this work (sum of the blocks' difficulties).
func (n *BlockTreeNode) FindFarthestNode() (*BlockTreeNode, float64) {
	return n.findFarthest(func(x *BlockTreeNode) []*BlockTreeNode {
		return x.Childs
	})
}

// findFarthest does the job of FindFarthestNode, for the tree made of the nodes returned by kids().
// It does not use recursion, as the tree is as deep as the blockchain is long.
// Only a node having more than one child takes a record on the stack, and the work
// is summed up separately for each branch, so its precision does not depend on the chain's length.
func (n *BlockTreeNode) findFarthest(kids func(*BlockTreeNode) []*BlockTreeNode) (*BlockTreeNode, float64) {
	type fork struct {
		childs  []*BlockTreeNode
		next    int            // index of the next child to check
		best    *BlockTreeNode // the best node found so far, among the already checked children
		bestPow float64
		pow     float64 // work between the previous fork and this one (including)
	}
	var stack []fork
	var run float64 // work since the last fork
	cur := n
	for {
		childs := kids(cur)
		for len(childs) == 1 {
			run += btc.GetDifficulty(cur.Bits())
			cur = childs[0]
			childs = kids(cur)
		}
		run += btc.GetDifficulty(cur.Bits())
		if len(childs) > 1 {
			stack = append(stack, fork{childs: childs, next: 1, pow: run})
			cur = childs[0]
			run = 0
			continue
		}

		// the end of a branch - go back to the nearest fork that still has a child to check
		res, pow := cur, run
		for {
			if len(stack) == 0 {
				return res, pow
			}
			f := &stack[len(stack)-1]
			if f.best == nil || pow > f.bestPow {
				f.best, f.bestPow = res, pow
			}
			if f.next < len(f.childs) {
				cur = f.childs[f.next]
				f.next++
				run = 0
				break
			}
			res, pow = f.best, f.bestPow+f.pow
			stack = stack[:len(stack)-1]
		}
	}
}

// FindPathTo returns the next node that leads to the given destination.
func (n *BlockTreeNode) FindPathTo(end *BlockTreeNode) *BlockTreeNode {
	if n == end {
		return nil
	}

	if end.Height <= n.Height {
		panic("end block is not higher then current")
	}

	if len(n.Childs) == 0 {
		panic("unknown path to block " + end.BlockHash.String())
	}

	if len(n.Childs) == 1 {
		return n.Childs[0] // if there is only one child, do it fast
	}

	for {
		// more then one children: go from the end until you reach the current node
		if end.Parent == n {
			return end
		}
		if end.Height <= n.Height {
			panic("reached the starting node height, but no hit")
		}
		end = end.Parent
	}
}

// FindPathToLocked calls from.FindPathTo(end) with BlockIndexAccess locked.
// Do not call it with BlockIndexAccess already locked.
func (ch *Chain) FindPathToLocked(from, end *BlockTreeNode) *BlockTreeNode {
	ch.BlockIndexAccess.Lock()
	defer ch.BlockIndexAccess.Unlock()
	return from.FindPathTo(end)
}

// FindFarthestNodeLocked calls n.FindFarthestNode() with BlockIndexAccess locked.
// Do not call it with BlockIndexAccess already locked.
func (ch *Chain) FindFarthestNodeLocked(n *BlockTreeNode) *BlockTreeNode {
	ch.BlockIndexAccess.Lock()
	defer ch.BlockIndexAccess.Unlock()
	res, _ := n.FindFarthestNode()
	return res
}

// BranchNodesLocked returns n and all its descendants (children before parents),
// collected with BlockIndexAccess locked.
// Do not call it with BlockIndexAccess already locked.
func (ch *Chain) BranchNodesLocked(n *BlockTreeNode) (res []*BlockTreeNode) {
	ch.BlockIndexAccess.Lock()
	res = n.appendBranchNodes(res)
	ch.BlockIndexAccess.Unlock()
	return
}

func (n *BlockTreeNode) appendBranchNodes(res []*BlockTreeNode) []*BlockTreeNode {
	for _, c := range n.Childs {
		res = c.appendBranchNodes(res)
	}
	return append(res, n)
}

// HasAllParents checks whether the given node has all its parent blocks already committed.
func (ch *Chain) HasAllParents(dst *BlockTreeNode) bool {
	for {
		dst = dst.Parent
		if ch.OnActiveBranch(dst) {
			return true
		}
		if dst == nil || dst.TxCount == 0 {
			return false
		}
	}
}

// FindFirstFather returns the first (with highest block height) father of the two nodes
func (us *BlockTreeNode) FindFirstFather(him *BlockTreeNode) *BlockTreeNode {
	for us.Height > him.Height {
		if us = us.Parent; us == nil {
			return nil
		}
	}
	for him.Height > us.Height {
		if him = him.Parent; him == nil {
			return nil
		}
	}
	for him != us {
		us = us.Parent
		him = him.Parent
		if us == nil || him == nil {
			return nil
		}
	}
	return him
}

// OnActiveBranch returns true if the given node is on the active branch.
func (ch *Chain) OnActiveBranch(dst *BlockTreeNode) bool {
	top := ch.LastBlock()
	for {
		if dst == top {
			return true
		}
		if dst.Height >= top.Height {
			return false
		}
		top = top.Parent
	}
}

// MoveToBlock performs a channel reorg.
func (ch *Chain) MoveToBlock(dst *BlockTreeNode) {
	cur := dst
	lastblock := ch.LastBlock()
	for cur.Height > lastblock.Height {
		cur = cur.Parent
		// if cur.TxCount is zero, it means we dont yet have this block's data
		if cur.TxCount == 0 {
			fmt.Println("MoveToBlock cannot continue A1")
			fmt.Println("Trying to go:", dst.BlockHash.String(), dst.Height)
			fmt.Println("Cannot go at:", cur.BlockHash.String(), cur.Height)
			return
		}
	}

	for lastblock.Height > cur.Height { // this is a rare case when the new branch has less blocks but more POW
		lastblock = lastblock.Parent
		// if cur.TxCount is zero, it means we dont yet have this block's data
		if lastblock.TxCount == 0 {
			fmt.Println("MoveToBlock cannot continue A2")
			fmt.Println("Trying to go:", dst.BlockHash.String(), dst.Height)
			fmt.Println("Cannot go at:", cur.BlockHash.String(), cur.Height)
			return
		}
	}

	// At this point both "lastblock" and "cur" should be at the same height
	for tmp := lastblock; tmp != cur; tmp = tmp.Parent {
		if cur.Parent.TxCount == 0 {
			fmt.Println("MoveToBlock cannot continue B")
			fmt.Println("Trying to go:", dst.BlockHash.String(), dst.Height)
			fmt.Println("Cannot go at:", cur.Parent.BlockHash.String(), cur.Parent.Height)
			return
		}
		cur = cur.Parent
	}

	// At this point "cur" is at the highest common block

	lastblock = ch.LastBlock() // recover lastblock, in case it was changed by the "rare case"
	for lastblock != cur {
		if AbortNow {
			return
		}
		ch.UndoLastBlock()
		lastblock = lastblock.Parent
	}
	ch.ParseTillBlock(dst)
}

func (ch *Chain) UndoLastBlock() {
	last := ch.LastBlock()
	if !ch.testnet() {
		fmt.Println("Undo block", last.Height, last.BlockHash.String(), last.BlockSize>>10, "KB")
	}

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
	bl.Height = last.Height

	ch.Unspent.UndoBlockTxs(bl, last.Parent.BlockHash.Hash[:])
	if ch.CB.BlockUndoneCB != nil {
		ch.CB.BlockUndoneCB(bl)
	}
	ch.SetLast(last.Parent)
}

// make sure ch.BlockIndexAccess is locked before calling it
func (cur *BlockTreeNode) delAllChildren(ch *Chain, deleteCallback func(*btc.Uint256)) {
	for i := range cur.Childs {
		if deleteCallback != nil {
			deleteCallback(cur.Childs[i].BlockHash)
		}
		cur.Childs[i].delAllChildren(ch, deleteCallback)
		delete(ch.BlockIndex, cur.Childs[i].BlockHash.BIdx())
		ch.Blocks.BlockInvalid(cur.Childs[i].BlockHash.Hash[:])
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

func (n *BlockTreeNode) addChild(c *BlockTreeNode) {
	n.Childs = append(n.Childs, c)
}

func (n *BlockTreeNode) delChild(c *BlockTreeNode) {
	newChds := make([]*BlockTreeNode, len(n.Childs)-1)
	xxx := 0
	for i := range n.Childs {
		if n.Childs[i] != c {
			newChds[xxx] = n.Childs[i]
			xxx++
		}
	}
	if xxx != len(n.Childs)-1 {
		panic("Child not found")
	}
	n.Childs = newChds
}
