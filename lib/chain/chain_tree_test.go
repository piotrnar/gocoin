package chain

import (
	"math"
	"math/rand"
	"testing"

	"github.com/piotrnar/gocoin/lib/btc"
)

var testNodeId uint32

// newTestNode adds a node with the given difficulty bits to the tree.
func newTestNode(parent *BlockTreeNode, bits uint32, txs uint32) *BlockTreeNode {
	testNodeId++
	n := &BlockTreeNode{Parent: parent, TxCount: txs, BlockSize: txs}
	n.BlockHeader[0] = byte(testNodeId)
	n.BlockHeader[1] = byte(testNodeId >> 8)
	n.BlockHeader[2] = byte(testNodeId >> 16)
	n.BlockHeader[72] = byte(bits)
	n.BlockHeader[73] = byte(bits >> 8)
	n.BlockHeader[74] = byte(bits >> 16)
	n.BlockHeader[75] = byte(bits >> 24)
	n.BlockHash = btc.NewSha2Hash(n.BlockHeader[:])
	if parent != nil {
		n.Height = parent.Height + 1
		parent.addChild(n)
	}
	return n
}

// refFarthest is the straightforward (recursive) definition of what findFarthest shall return.
func refFarthest(n *BlockTreeNode, committed bool) (res *BlockTreeNode, pow float64) {
	res = n
	first := true
	for _, c := range n.Childs {
		if committed && c.TxCount == 0 {
			continue
		}
		if r, p := refFarthest(c, committed); first || p > pow {
			res, pow, first = r, p, false
		}
	}
	pow += btc.GetDifficulty(n.Bits())
	return
}

// samePow compares two sums of work, which may have been added up in a different order.
func samePow(a, b float64) bool {
	return math.Abs(a-b) <= 1e-9*math.Max(a, b)
}

func TestFindFarthestNode(t *testing.T) {
	bits := []uint32{0x1d00ffff, 0x1c7fff80, 0x1c00ffff, 0x1b0404cb}
	r := rand.New(rand.NewSource(1))
	for round := 0; round < 300; round++ {
		root := newTestNode(nil, bits[0], 0)
		nodes := []*BlockTreeNode{root}
		for cnt := 1 + r.Intn(60); cnt > 0; cnt-- {
			var parent *BlockTreeNode
			if r.Intn(3) != 0 {
				parent = nodes[len(nodes)-1] // mostly make it a chain
			} else {
				parent = nodes[r.Intn(len(nodes))]
			}
			var txs uint32 = 1
			if r.Intn(6) == 0 {
				txs = 0 // only the header
			}
			nodes = append(nodes, newTestNode(parent, bits[r.Intn(len(bits))], txs))
		}

		for _, n := range []*BlockTreeNode{root, nodes[len(nodes)/2]} {
			got, gpow := n.FindFarthestNode()
			exp, epow := refFarthest(n, false)
			if got != exp || !samePow(gpow, epow) {
				t.Fatal("FindFarthestNode: round", round, "got", got.Height, gpow, "expected", exp.Height, epow)
			}

			got, gpow = n.findFarthestCommitted()
			exp, epow = refFarthest(n, true)
			if got != exp || !samePow(gpow, epow) {
				t.Fatal("findFarthestCommitted: round", round, "got", got.Height, gpow, "expected", exp.Height, epow)
			}
			for x := got; x != n; x = x.Parent {
				if x.TxCount == 0 {
					t.Fatal("findFarthestCommitted: the path goes through a node with no data")
				}
			}
		}
	}
}

func TestFindFarthestNodeMoreWorkWins(t *testing.T) {
	root := newTestNode(nil, 0x1d00ffff, 0)
	fork := newTestNode(root, 0x1d00ffff, 1)
	// the longer branch, but made of the minimum difficulty blocks
	long := newTestNode(newTestNode(newTestNode(fork, 0x1d00ffff, 1), 0x1d00ffff, 1), 0x1d00ffff, 1)
	// the shorter one, but with more work
	short := newTestNode(fork, 0x1c00ffff, 1)
	if n, _ := root.FindFarthestNode(); n != short {
		t.Fatal("the branch with more work should win")
	}
	if !short.MorePOW(long) || long.MorePOW(short) {
		t.Fatal("MorePOW does not agree")
	}

	// a header-only node on top of the best branch is the farthest one, but not the farthest committed
	hdr := newTestNode(short, 0x1c00ffff, 0)
	newTestNode(hdr, 0x1c00ffff, 1) // it has the data, but cannot be reached
	if n, _ := root.FindFarthestNode(); n.Parent != hdr {
		t.Fatal("the node behind the header should be the farthest")
	}
	if n, _ := root.findFarthestCommitted(); n != short {
		t.Fatal("the last node with the data should be the farthest committed")
	}
}

// The tree is as deep as the chain is long, so it must not be walked using recursion.
func TestFindFarthestNodeDeepTree(t *testing.T) {
	root := newTestNode(nil, 0x1d00ffff, 0)
	n := root
	var side *BlockTreeNode
	for i := 0; i < 1000000; i++ {
		c := &BlockTreeNode{Parent: n, Height: n.Height + 1, TxCount: 1}
		c.BlockHeader[72], c.BlockHeader[73], c.BlockHeader[75] = 0xff, 0xff, 0x1d
		n.Childs = []*BlockTreeNode{c}
		if i%1000 == 500 { // a short side branch every now and then
			side = &BlockTreeNode{Parent: n, Height: n.Height + 1, TxCount: 1}
			side.BlockHeader = c.BlockHeader
			n.Childs = append(n.Childs, side)
		}
		n = c
	}
	if got, pow := root.FindFarthestNode(); got != n || pow != 1000001 {
		t.Fatal("wrong node", got.Height, pow)
	}
	if got, _ := root.findFarthestCommitted(); got != n {
		t.Fatal("wrong committed node", got.Height)
	}
}

func TestRemoveDetachedNodes(t *testing.T) {
	for round := 0; round < 50; round++ { // the result must not depend on the map's order
		ch := new(Chain)
		ch.BlockIndex = make(map[[btc.Uint256IdxLen]byte]*BlockTreeNode)
		add := func(parent *BlockTreeNode) *BlockTreeNode {
			n := newTestNode(parent, 0x1d00ffff, 1)
			ch.BlockIndex[n.BlockHash.BIdx()] = n
			return n
		}
		ch.BlockTreeRoot = add(nil)
		a := add(add(ch.BlockTreeRoot))
		b := add(ch.BlockTreeRoot)
		orphan := add(nil) // a block with no parent...
		o1 := add(add(orphan))
		o2 := add(orphan) // ... and its descendants

		ch.removeDetachedNodes()
		if len(ch.BlockIndex) != 4 {
			t.Fatal("wrong number of nodes left:", len(ch.BlockIndex))
		}
		for _, n := range []*BlockTreeNode{ch.BlockTreeRoot, a, a.Parent, b} {
			if ch.BlockIndex[n.BlockHash.BIdx()] != n {
				t.Fatal("attached node removed")
			}
		}
		for _, n := range []*BlockTreeNode{orphan, o1, o1.Parent, o2} {
			if _, ok := ch.BlockIndex[n.BlockHash.BIdx()]; ok {
				t.Fatal("detached node not removed")
			}
		}
	}
}

func TestEnforceBIP30(t *testing.T) {
	other := btc.NewUint256FromString("0000000000000000000000000000000000000000000000000000000000000001")
	main := &Chain{Genesis: btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")}
	main.Consensus.BIP34Height = 227931
	t3 := &Chain{Genesis: btc.NewUint256FromString("000000000933ea01ad0ee984209779baaec3ced90fa3f408719526f8d77f4943")}
	t3.Consensus.BIP34Height = 21111
	t4 := &Chain{Genesis: btc.NewUint256FromString("00000000da84f2bafbbc53dee25a72ae507ff4914b867c565be350b0da8bf043")}
	t4.Consensus.BIP34Height = 1
	if main.testnet() || !t3.testnet() || t3.testnet4() || !t4.testnet() || !t4.testnet4() {
		t.Fatal("wrong network detection")
	}
	ex1 := btc.NewUint256FromString("00000000000a4d0a398161ffc163c503763b1f4360639393e0e4c8e300e0caec")
	ex2 := btc.NewUint256FromString("00000000000743f190a18c5577a3c2d2a1f610ae9601ac046a38084ccb7cd721")
	for i, c := range []struct {
		ch      *Chain
		height  uint32
		hash    *btc.Uint256
		enforce bool
	}{
		{main, 1, other, true},
		{main, 227931, other, true},
		{main, 227932, other, false},
		{main, 900000, other, false},
		{main, BIP34ImpliesBIP30Limit - 1, other, false},
		{main, BIP34ImpliesBIP30Limit, other, true},
		{main, 91842, ex1, false}, // the two historical exceptions...
		{main, 91880, ex2, false},
		{main, 91842, other, true}, // ... but only these very blocks
		{main, 91880, ex1, true},
		{t3, 21111, other, true},
		{t3, 21112, other, false},
		{t3, 20000, ex1, true},
		{t3, 4000000, other, true},
		{t4, 1, other, true},
		{t4, 100000, other, true},
	} {
		if c.ch.enforceBIP30(c.height, c.hash) != c.enforce {
			t.Fatal("wrong result for case", i, "at height", c.height)
		}
	}
}

func TestCheckTransactions(t *testing.T) {
	good := func() *btc.Tx {
		tx := &btc.Tx{TxIn: []*btc.TxIn{{Sequence: 0xffffffff}}, TxOut: []*btc.TxOut{{Value: 1}}}
		tx.TxIn[0].Input.Hash[0] = 1
		return tx
	}
	for _, cnt := range []int{0, 1, 100, 5000} { // 5000 is enough to make it use several tasks
		txs := make([]*btc.Tx, cnt)
		for i := range txs {
			txs[i] = good()
		}
		if er := CheckTransactions(txs, 1000, 1700000000); er != nil {
			t.Fatal("valid txs rejected:", cnt, er)
		}
		if cnt == 0 {
			continue
		}
		for _, bad := range []int{0, cnt / 2, cnt - 1} {
			// a tx with no outputs
			txs[bad].TxOut = nil
			if er := CheckTransactions(txs, 1000, 1700000000); er == nil {
				t.Fatal("tx with no outputs accepted at", bad, "of", cnt)
			}
			txs[bad] = good()

			// a tx that is not final yet
			txs[bad].Lock_time = 1000
			txs[bad].TxIn[0].Sequence = 0
			er := CheckTransactions(txs, 1000, 1700000000)
			if er == nil {
				t.Fatal("non-final tx accepted at", bad, "of", cnt)
			}
			if CheckTransactions(txs, 1001, 1700000000) != nil {
				t.Fatal("final tx rejected at", bad, "of", cnt)
			}
			txs[bad] = good()
		}
	}
}
