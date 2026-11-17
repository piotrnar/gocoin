package chain

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/piotrnar/gocoin/lib/btc"
)

// The tests in this file build a small chain made of fake blocks (with no proof of work,
// as they are passed straight to PostCheckBlock and AcceptBlock), to check how
// the transactions get applied, rejected and moved between the branches.

const (
	testBitsLow   = 0x1d00ffff // difficulty 1
	testBitsHigh  = 0x1c00ffff // difficulty 256
	testBlockOuts = 1000       // number of the spendable outputs in the first block
)

var (
	scrTrue  = []byte{0x51} // OP_TRUE - anyone can spend
	scrFalse = []byte{0x00} // OP_FALSE - nobody can spend
)

type testChain struct {
	*Chain
	t     *testing.T
	dir   string
	nonce uint32
}

func openTestChain(t *testing.T, dir string) *testChain {
	genesis := btc.NewUint256FromString("000000000019d6689c085ae165831e934ff763ae46a2a6c172b3f1b60a8ce26f")
	ch := NewChainExt(dir, genesis, false, nil, &BlockDBOpts{MaxCachedBlocks: 10})
	t.Cleanup(ch.Close) // in case the test fails before closing it
	return &testChain{Chain: ch, t: t, dir: dir}
}

func testCoinbase(height uint32, tag byte, outs []*btc.TxOut) *btc.Tx {
	tx := &btc.Tx{Version: 1}
	ti := &btc.TxIn{Sequence: 0xffffffff, ScriptSig: []byte{3, byte(height), byte(height >> 8), tag}}
	ti.Input.Vout = 0xffffffff
	tx.TxIn = []*btc.TxIn{ti}
	if outs == nil {
		outs = []*btc.TxOut{{Value: 50e8, Pk_script: scrTrue}}
	}
	tx.TxOut = outs
	tx.SetHash(tx.SerializeNew())
	return tx
}

// testSpend makes a tx that spends the given outputs of one transaction, into a single output.
func testSpend(txid *btc.Uint256, vouts []uint32, value uint64) *btc.Tx {
	tx := &btc.Tx{Version: 1}
	for _, vout := range vouts {
		ti := &btc.TxIn{Sequence: 0xffffffff}
		ti.Input.Hash = txid.Hash
		ti.Input.Vout = vout
		tx.TxIn = append(tx.TxIn, ti)
	}
	tx.TxOut = []*btc.TxOut{{Value: value, Pk_script: scrTrue}}
	tx.SetHash(tx.SerializeNew())
	return tx
}

func vouts(from, to uint32) (res []uint32) {
	for ; from < to; from++ {
		res = append(res, from)
	}
	return
}

func (ch *testChain) makeBlock(parent *BlockTreeNode, bits uint32, txs []*btc.Tx) *btc.Block {
	ch.nonce++
	raw := make([]byte, 80, 100e3)
	binary.LittleEndian.PutUint32(raw[0:4], 4)
	copy(raw[4:36], parent.BlockHash.Hash[:])
	mtr := make([][32]byte, len(txs), 3*len(txs))
	for i, tx := range txs {
		mtr[i] = tx.Hash.Hash
	}
	merkle, _ := btc.CalcMerkle(mtr)
	copy(raw[36:68], merkle)
	binary.LittleEndian.PutUint32(raw[68:72], 1500000000+600*(parent.Height+1))
	binary.LittleEndian.PutUint32(raw[72:76], bits)
	binary.LittleEndian.PutUint32(raw[76:80], ch.nonce)
	raw = btc.AppendVlen(raw, uint64(len(txs)))
	for _, tx := range txs {
		raw = append(raw, tx.SerializeNew()...)
	}
	bl, er := btc.NewBlock(raw)
	if er != nil {
		ch.t.Fatal(er)
	}
	bl.Height = parent.Height + 1
	return bl
}

// add makes a block of the given transactions (coinbase is added) and passes it to the chain.
func (ch *testChain) add(parent *BlockTreeNode, bits uint32, txs ...*btc.Tx) (*BlockTreeNode, error) {
	return ch.addBlock(ch.makeBlock(parent, bits, append([]*btc.Tx{testCoinbase(parent.Height+1, byte(ch.nonce), nil)}, txs...)))
}

func (ch *testChain) addBlock(bl *btc.Block) (node *BlockTreeNode, er error) {
	if er = ch.PostCheckBlock(bl); er != nil {
		ch.t.Fatal("PostCheckBlock:", er)
	}
	hash := bl.Hash
	if er = ch.AcceptBlock(bl); er == nil {
		ch.BlockIndexAccess.Lock()
		node = ch.BlockIndex[hash.BIdx()]
		ch.BlockIndexAccess.Unlock()
		if node == nil {
			ch.t.Fatal("accepted block not in the index")
		}
	}
	return
}

func (ch *testChain) mustAdd(parent *BlockTreeNode, bits uint32, txs ...*btc.Tx) *BlockTreeNode {
	node, er := ch.add(parent, bits, txs...)
	if er != nil {
		ch.t.Fatal("block at height", parent.Height+1, "rejected:", er)
	}
	return node
}

// mustReject checks that the block made of the given txs is rejected, with the tip and index left untouched.
func (ch *testChain) mustReject(what string, parent *BlockTreeNode, txs ...*btc.Tx) {
	tip := ch.LastBlock()
	cnt := len(ch.BlockIndex)
	_, er := ch.add(parent, testBitsLow, txs...)
	if er == nil || !strings.Contains(er.Error(), what) {
		ch.t.Fatal("expected error with", what, "but got:", er)
	}
	if ch.LastBlock() != tip || len(ch.BlockIndex) != cnt {
		ch.t.Fatal("rejected block (", what, ") has changed the chain")
	}
}

func (ch *testChain) unspent(txid *btc.Uint256, vout uint32) bool {
	return ch.Unspent.UnspentGet(&btc.TxPrevOut{Hash: txid.Hash, Vout: vout}) != nil
}

func (ch *testChain) checkTip(what string, tip *BlockTreeNode) {
	if ch.LastBlock() != tip {
		ch.t.Fatal(what, "- wrong tip: height", ch.LastBlock().Height, "instead of", tip.Height)
	}
}

// build makes the chain used by all the tests: a block with many spendable outputs
// (plus two unspendable ones at the end), followed by enough blocks for it to mature.
func (ch *testChain) build() (first *btc.Tx, tip *BlockTreeNode) {
	outs := make([]*btc.TxOut, testBlockOuts, testBlockOuts+2)
	for i := range outs {
		outs[i] = &btc.TxOut{Value: 1000, Pk_script: scrTrue}
	}
	outs = append(outs, &btc.TxOut{Value: 1000, Pk_script: scrFalse}, &btc.TxOut{Value: 1000, Pk_script: scrFalse})
	first = testCoinbase(1, 0, outs)
	tip, er := ch.addBlock(ch.makeBlock(ch.BlockTreeRoot, testBitsLow, []*btc.Tx{first}))
	if er != nil {
		ch.t.Fatal(er)
	}
	for i := 0; i < COINBASE_MATURITY; i++ {
		tip = ch.mustAdd(tip, testBitsLow)
		if i == 50 {
			ch.Idle() // let some of the blocks be written to the disk
		}
	}
	ch.checkTip("after building", tip)
	return
}

func TestChainAcceptAndReject(t *testing.T) {
	ch := openTestChain(t, t.TempDir()+"/")
	first, tip := ch.build()
	base := tip

	// A block with many inputs to verify (in ten transactions)
	var txs []*btc.Tx
	for i := uint32(0); i < 10; i++ {
		txs = append(txs, testSpend(&first.Hash, vouts(i*50, i*50+50), 50*1000-uint64(i)))
	}
	// ... one of them spending an output created by the same block
	txs = append(txs, testSpend(&txs[3].Hash, []uint32{0}, 1))
	tip = ch.mustAdd(tip, testBitsLow, txs...)
	ch.checkTip("after spending", tip)
	if tip.Height != COINBASE_MATURITY+2 || tip.TxCount != 12 {
		t.Fatal("wrong block", tip.Height, tip.TxCount)
	}
	for vout := uint32(0); vout < testBlockOuts+2; vout++ {
		if ch.unspent(&first.Hash, vout) != (vout >= 500) {
			t.Fatal("wrong state of output", vout)
		}
	}
	if !ch.unspent(&txs[0].Hash, 0) || ch.unspent(&txs[3].Hash, 0) || !ch.unspent(&txs[10].Hash, 0) {
		t.Fatal("wrong state of the new outputs")
	}

	// Now the blocks that must be rejected
	ch.mustReject("VerifyScripts failed", tip, // one bad script among many good ones
		testSpend(&first.Hash, vouts(500, 700), 1), testSpend(&first.Hash, append(vouts(700, 800), testBlockOuts), 1))
	ch.mustReject("VerifyScripts failed", tip, testSpend(&first.Hash, []uint32{testBlockOuts, testBlockOuts + 1}, 1))
	ch.mustReject("double spend", tip, // after a lot of scripts have been queued for verifying
		testSpend(&first.Hash, vouts(500, 900), 1), testSpend(&first.Hash, []uint32{600}, 1))
	ch.mustReject("double spend", tip, testSpend(&first.Hash, []uint32{600, 601, 600}, 1))
	ch.mustReject("Unknown input", tip, testSpend(&first.Hash, []uint32{10}, 1)) // spent by the previous block
	ch.mustReject("Unknown input", tip, testSpend(&txs[3].Hash, []uint32{0}, 2))
	ch.mustReject("BIP30", tip, txs[10]) // the very same tx as mined before, still unspent
	ch.mustReject("more spent", tip, testSpend(&first.Hash, []uint32{600}, 1001))
	ch.mustReject("prematured coinbase", tip, testSpend(ch.coinbaseOf(tip), []uint32{0}, 1))
	ch.mustReject("own coinbase", tip, testSpend(&testCoinbase(tip.Height+1, byte(ch.nonce), nil).Hash, []uint32{0}, 1))

	// BIP30: a coinbase which is a copy of an earlier one, that has not been spent
	old := testCoinbase(base.Height, 77, nil)
	b77 := ch.makeBlock(tip, testBitsLow, []*btc.Tx{old})
	if _, er := ch.addBlock(b77); er != nil {
		t.Fatal(er)
	}
	tip = ch.LastBlock()
	if _, er := ch.addBlock(ch.makeBlock(tip, testBitsLow, []*btc.Tx{old})); er == nil || !strings.Contains(er.Error(), "BIP30") {
		t.Fatal("overwriting of an unspent tx not rejected:", er)
	}
	ch.checkTip("after BIP30", tip)

	// too much money created
	if _, er := ch.addBlock(ch.makeBlock(tip, testBitsLow, []*btc.Tx{testCoinbase(tip.Height+1, 1,
		[]*btc.TxOut{{Value: 50e8 + 1, Pk_script: scrTrue}})})); er == nil {
		t.Fatal("block creating too much money not rejected")
	}
	ch.checkTip("after too big coinbase", tip)

	// The same chain must be there after closing and opening it again
	hash, cnt := tip.BlockHash.Hash, len(ch.BlockIndex)
	ch.Close()
	ch = openTestChain(t, ch.dir)
	if ch.LastBlock().BlockHash.Hash != hash || len(ch.BlockIndex) != cnt {
		t.Fatal("wrong chain after reopening:", ch.LastBlock().Height, len(ch.BlockIndex), cnt)
	}
	for vout := uint32(0); vout < testBlockOuts+2; vout++ {
		if ch.unspent(&first.Hash, vout) != (vout >= 500) {
			t.Fatal("wrong state of output", vout, "after reopening")
		}
	}
	ch.mustAdd(ch.LastBlock(), testBitsLow, testSpend(&first.Hash, vouts(500, testBlockOuts), 1))
	ch.Close()
}

// coinbaseOf returns the ID of the coinbase transaction of the given block.
func (ch *testChain) coinbaseOf(n *BlockTreeNode) *btc.Uint256 {
	dat, _, er := ch.Blocks.BlockGet(n.BlockHash)
	if er != nil {
		ch.t.Fatal(er)
	}
	bl, er := btc.NewBlock(dat)
	if er == nil {
		er = bl.BuildTxList()
	}
	if er != nil {
		ch.t.Fatal(er)
	}
	return &bl.Txs[0].Hash
}

func TestChainReorg(t *testing.T) {
	ch := openTestChain(t, t.TempDir()+"/")
	first, base := ch.build()

	// branch A: spends the outputs 0..99
	txA := testSpend(&first.Hash, vouts(0, 100), 5)
	a1 := ch.mustAdd(base, testBitsLow, txA)
	ch.checkTip("branch A", a1)

	// branch B: spends the outputs 50..149 and has the same work, so it only gets stored
	txB := testSpend(&first.Hash, vouts(50, 150), 6)
	b1 := ch.mustAdd(base, testBitsLow, txB)
	ch.checkTip("branch B stored", a1)
	if b1.TxCount == 0 || !ch.unspent(&first.Hash, 120) || ch.unspent(&first.Hash, 20) {
		t.Fatal("wrong state before the reorg")
	}

	// ... until it gets one more block
	txB2 := testSpend(&txB.Hash, []uint32{0}, 3)
	b2 := ch.mustAdd(b1, testBitsLow, txB2)
	ch.checkTip("reorg to B", b2)
	if !ch.unspent(&first.Hash, 20) || ch.unspent(&first.Hash, 120) || ch.unspent(&txA.Hash, 0) ||
		ch.unspent(&txB.Hash, 0) || !ch.unspent(&txB2.Hash, 0) {
		t.Fatal("wrong state after the reorg to B")
	}
	if !ch.OnActiveBranch(b1) || ch.OnActiveBranch(a1) || a1.FindFirstFather(b2) != base {
		t.Fatal("wrong branches after the reorg to B")
	}

	// back to A - a shorter branch, but with more work
	a2 := ch.mustAdd(a1, testBitsHigh)
	ch.checkTip("reorg to A", a2)
	if ch.unspent(&first.Hash, 20) || !ch.unspent(&first.Hash, 120) || !ch.unspent(&txA.Hash, 0) ||
		ch.unspent(&txB.Hash, 0) || ch.unspent(&txB2.Hash, 0) {
		t.Fatal("wrong state after the reorg to A")
	}

	// The same after closing and opening it again (the blocks of branch B are still there)
	ch.Close()
	ch = openTestChain(t, ch.dir)
	if ch.LastBlock().BlockHash.Hash != a2.BlockHash.Hash {
		t.Fatal("wrong tip after reopening")
	}
	if n := ch.BlockIndex[b2.BlockHash.BIdx()]; n == nil || n.Parent == nil || n.Parent.Parent == nil ||
		n.Parent.Parent.BlockHash.Hash != base.BlockHash.Hash {
		t.Fatal("branch B missing after reopening")
	}
	if ch.unspent(&first.Hash, 20) || !ch.unspent(&first.Hash, 120) {
		t.Fatal("wrong state after reopening")
	}
	ch.Close()
}

// A reorg into a branch that turns out to be invalid must bring us back to where we were,
// also when there is a block of which only the header is known, on top of our branch.
func TestChainFailedReorg(t *testing.T) {
	ch := openTestChain(t, t.TempDir()+"/")
	first, base := ch.build()

	txA := testSpend(&first.Hash, vouts(0, 100), 5)
	a1 := ch.mustAdd(base, testBitsLow, txA)

	// the header of the next block of our branch (its data is "still being downloaded")
	hdr := ch.makeBlock(a1, testBitsLow, []*btc.Tx{testCoinbase(a1.Height+1, 99, nil)})
	ch.BlockIndexAccess.Lock()
	hnode := ch.AcceptHeader(hdr)
	ch.BlockIndexAccess.Unlock()
	if n, _ := ch.BlockTreeRoot.FindFarthestNode(); n != hnode {
		t.Fatal("the header-only node should be the farthest one")
	}

	// Branch B: its first block has an invalid script, which is only found during the reorg
	b1 := ch.mustAdd(base, testBitsLow, testSpend(&first.Hash, []uint32{200, testBlockOuts}, 1))
	ch.checkTip("invalid block stored", a1)
	cnt := len(ch.BlockIndex)

	_, er := ch.add(b1, testBitsLow)
	if er == nil {
		t.Fatal("reorg to the invalid branch reported no error")
	}
	ch.checkTip("after the failed reorg", a1)
	if len(ch.BlockIndex) != cnt-1 || ch.BlockIndex[b1.BlockHash.BIdx()] != nil {
		t.Fatal("the invalid branch was not removed:", len(ch.BlockIndex), cnt)
	}
	if ch.BlockIndex[hnode.BlockHash.BIdx()] != hnode {
		t.Fatal("the header-only node is gone")
	}
	if ch.unspent(&first.Hash, 20) || !ch.unspent(&first.Hash, 200) || !ch.unspent(&txA.Hash, 0) {
		t.Fatal("wrong state after the failed reorg")
	}

	// when the data of the awaited block arrives, the chain shall go on
	if er := ch.PostCheckBlock(hdr); er != nil {
		t.Fatal(er)
	}
	if er := ch.CommitBlock(hdr, hnode); er != nil {
		t.Fatal(er)
	}
	ch.checkTip("after the awaited block", hnode)
	ch.Close()
}
