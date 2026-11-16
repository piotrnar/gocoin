package txpool

import (
	"encoding/binary"
	"os"
	"testing"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/lib/script"
)

// A replacement must pay more in total than what it removes, not only have a higher fee rate.
func TestRBFTotalFee(t *testing.T) {
	db := testSetup()
	u := testAddUtxo(db, 1, 1000000)
	big := testTx(19800, 50, btc.TxPrevOut{Hash: u, Vout: 0}) // 50 outputs, fee 10000
	if testSubmit(big) != 0 {
		t.Fatal("setup tx rejected")
	}
	small := testTx(995000, 1, btc.TxPrevOut{Hash: u, Vout: 0}) // much higher fee rate, but the fee is only 5000
	if res := testSubmit(small); res != TX_REJECTED_RBF_LOWFEE {
		t.Fatal("replacement paying less in total: expected RBF_LOWFEE, got", ReasonToString(res))
	}
	if !testInPool(big) {
		t.Fatal("original tx removed")
	}
	small = testTx(980000, 1, btc.TxPrevOut{Hash: u, Vout: 0}) // fee 20000
	if res := testSubmit(small); res != 0 || !testInPool(small) || testInPool(big) {
		t.Fatal("replacement paying more in total rejected:", ReasonToString(res))
	}
	testMempoolOK(t)
}

// The number of unconfirmed txs connected with each other is limited.
func TestClusterLimit(t *testing.T) {
	db := testSetup()
	common.CFG.TXPool.MaxClusterTxs = 6
	defer func() { common.CFG.TXPool.MaxClusterTxs = 0 }()

	// two separate chains of three txs each
	var tips [2]*btc.Tx
	for n := range tips {
		prev := btc.TxPrevOut{Hash: testAddUtxo(db, 1+n, 1000000)}
		for i := 0; i < 3; i++ {
			tips[n] = testTx(uint64(450000>>i), 2, prev)
			if res := testSubmit(tips[n]); res != 0 {
				t.Fatal("chain tx rejected:", ReasonToString(res))
			}
			prev = btc.TxPrevOut{Hash: tips[n].Hash.Hash, Vout: 0}
		}
	}

	// a tx joining both chains would make a cluster of seven
	join := testTx(100000, 1, btc.TxPrevOut{Hash: tips[0].Hash.Hash, Vout: 0}, btc.TxPrevOut{Hash: tips[1].Hash.Hash, Vout: 0})
	if res := testSubmit(join); res != TX_REJECTED_CLUSTER_BIG || testInPool(join) {
		t.Fatal("tx joining two chains: expected CLUSTER_BIG, got", ReasonToString(res))
	}
	if len(TransactionsToSend) != 6 {
		t.Fatal("mempool changed by a rejected tx")
	}

	// each chain can still grow up to the limit, but not further
	prev := btc.TxPrevOut{Hash: tips[0].Hash.Hash, Vout: 0}
	for i := 4; i <= 7; i++ {
		tx := testTx(uint64(40000>>i), 2, prev)
		res := testSubmit(tx)
		if i <= 6 && res != 0 {
			t.Fatal("tx number", i, "of the chain rejected:", ReasonToString(res))
		}
		if i == 7 && res != TX_REJECTED_CLUSTER_BIG {
			t.Fatal("tx number 7 of the chain: expected CLUSTER_BIG, got", ReasonToString(res))
		}
		if i == 6 {
			// replacing the last tx does not make the cluster any bigger
			tx = testTx(uint64(30000>>i), 2, prev)
			if res = testSubmit(tx); res != 0 {
				t.Fatal("replacement at the limit rejected:", ReasonToString(res))
			}
		}
		prev = btc.TxPrevOut{Hash: tx.Hash.Hash, Vout: 0}
	}

	// the limit also counts the txs that are next to each other (the parent with five children)
	p := testTx(100000, 8, btc.TxPrevOut{Hash: testAddUtxo(db, 10, 1000000)})
	if testSubmit(p) != 0 {
		t.Fatal("parent rejected")
	}
	for i := 0; i < 6; i++ {
		res := testSubmit(testTx(90000, 1, btc.TxPrevOut{Hash: p.Hash.Hash, Vout: uint32(i)}))
		if i < 5 && res != 0 {
			t.Fatal("child", i, "rejected:", ReasonToString(res))
		}
		if i == 5 && res != TX_REJECTED_CLUSTER_BIG {
			t.Fatal("sixth child: expected CLUSTER_BIG, got", ReasonToString(res))
		}
	}

	// the user can still load such a tx
	if !SubmitLocalTx(join, join.Raw) || !testInPool(join) {
		t.Fatal("local tx over the cluster limit rejected")
	}
	testMempoolOK(t)
}

// Txs that cannot be mined in the next block (nLockTime, BIP68) must not be accepted, unless loaded locally.
func TestNonFinalTx(t *testing.T) {
	db := testSetup()
	tip := &chain.BlockTreeNode{Height: 1000}
	binary.LittleEndian.PutUint32(tip.BlockHeader[68:72], 1600000000) // the block's timestamp
	common.BlockChain.SetLast(tip)
	common.Last.ScriptFlags = script.VER_CSV
	defer func() { common.Last.ScriptFlags = 0 }()

	var id int
	check := func(what string, exp byte, sequence, locktime, coin_height uint32) {
		t.Helper()
		id++
		u := testAddUtxoExt(db, id, 100000, coin_height, []byte{0x51})
		tx := testTxExt(90000, 1, sequence, locktime, btc.TxPrevOut{Hash: u, Vout: 0})
		if res := testSubmit(tx); res != exp {
			t.Fatal(what, "- expected", ReasonToString(exp), "got", ReasonToString(res))
		}
		if _, rej := TransactionsRejected[tx.Hash.BIdx()]; rej {
			t.Fatal(what, "- must not be remembered as rejected")
		}
	}
	const no_bip68 = 0xfffffffd // relative lock-time disabled, but nLockTime still works
	check("lock height reached", 0, no_bip68, 1000, 100)
	check("lock height not reached", TX_REJECTED_NOT_FINAL, no_bip68, 1001, 100)
	check("lock height not reached, but final sequence", 0, 0xffffffff, 1001, 100)
	check("lock time reached", 0, no_bip68, 1599999999, 100)
	check("lock time not reached", TX_REJECTED_NOT_FINAL, no_bip68, 1600000000, 100)
	check("relative lock reached", 0, 11, 0, 990)
	check("relative lock not reached", TX_REJECTED_NOT_FINAL, 12, 0, 990)

	// a relative lock on an output that is not mined yet can never be reached in the next block
	u := testAddUtxo(db, 100, 100000)
	p := testTx(90000, 1, btc.TxPrevOut{Hash: u, Vout: 0})
	if testSubmit(p) != 0 {
		t.Fatal("parent rejected")
	}
	c := testTxExt(80000, 1, 1, 0, btc.TxPrevOut{Hash: p.Hash.Hash, Vout: 0})
	if res := testSubmit(c); res != TX_REJECTED_NOT_FINAL {
		t.Fatal("relative lock on unconfirmed input - expected NOT_FINAL, got", ReasonToString(res))
	}

	// ... but the user can still load such a tx
	if !SubmitLocalTx(c, c.Raw) || !testInPool(c) {
		t.Fatal("non-final local tx rejected")
	}
	testMempoolOK(t)
}

// Txs from the network have their scripts verified with TxMutex unlocked.
func TestUntrustedTx(t *testing.T) {
	db := testSetup()
	dbg_err := script.DBG_ERR
	submit := func(tx *btc.Tx) (res byte) {
		TxMutex.Lock()
		TransactionsPending[tx.Hash.BIdx()] = true
		TxMutex.Unlock()
		HandleNetTx(&TxRcvd{Tx: tx, FeedbackCB: func(n *TxRcvd, _ *OneTxToSend) { res = n.Result }})
		if len(TransactionsPending) != 0 {
			t.Fatal("tx left as pending")
		}
		return
	}
	good := testAddUtxoExt(db, 1, 100000, 100, []byte{0x51}) // OP_TRUE
	bad := testAddUtxoExt(db, 2, 100000, 100, []byte{0x00})  // OP_FALSE
	p := testTx(45000, 2, btc.TxPrevOut{Hash: good, Vout: 0})
	if res := submit(p); res != 0 || !testInPool(p) {
		t.Fatal("tx with valid scripts rejected:", ReasonToString(res))
	}
	c := testTx(80000, 1, btc.TxPrevOut{Hash: p.Hash.Hash, Vout: 0}, btc.TxPrevOut{Hash: p.Hash.Hash, Vout: 1})
	if res := submit(c); res != 0 || !testInPool(c) {
		t.Fatal("child tx with valid scripts rejected:", ReasonToString(res))
	}
	x := testTx(90000, 1, btc.TxPrevOut{Hash: bad, Vout: 0})
	if res := submit(x); res != TX_REJECTED_SCRIPT_FAIL || testInPool(x) {
		t.Fatal("tx with invalid script: expected SCRIPT_FAIL, got", ReasonToString(res))
	}
	if script.DBG_ERR != dbg_err {
		t.Fatal("script.DBG_ERR not restored")
	}
	testMempoolOK(t)
}

// The mempool may change while the scripts are being verified with TxMutex unlocked.
func TestMempoolChangedWhileVerifying(t *testing.T) {
	db := testSetup()
	defer func() { testHookUnlocked = nil }()
	submit := func(tx *btc.Tx, hook func()) (res byte) {
		TxMutex.Lock()
		TransactionsPending[tx.Hash.BIdx()] = true
		TxMutex.Unlock()
		var called bool
		testHookUnlocked = func() {
			testHookUnlocked = nil
			called = true
			if NeedThisTxExt(&tx.Hash, nil) != 3 {
				t.Error("tx not pending while its scripts are being verified")
			}
			hook()
		}
		HandleNetTx(&TxRcvd{Tx: tx, FeedbackCB: func(n *TxRcvd, _ *OneTxToSend) { res = n.Result }})
		if !called {
			t.Fatal("TxMutex was not unlocked for verifying the scripts")
		}
		return
	}

	// the parent disappears
	p := testTx(90000, 1, btc.TxPrevOut{Hash: testAddUtxo(db, 1, 100000)})
	if testSubmit(p) != 0 {
		t.Fatal("parent rejected")
	}
	c := testTx(80000, 1, btc.TxPrevOut{Hash: p.Hash.Hash, Vout: 0})
	res := submit(c, func() {
		TxMutex.Lock()
		TransactionsToSend[p.Hash.BIdx()].Delete(false, 0)
		TxMutex.Unlock()
	})
	if res != TX_REJECTED_NO_TXOU || testInPool(c) || len(TransactionsToSend) != 0 {
		t.Fatal("parent removed while verifying: expected NO_TXOU, got", ReasonToString(res))
	}
	if txr := TransactionsRejected[c.Hash.BIdx()]; txr == nil || txr.Waiting4 == nil {
		t.Fatal("child of the removed parent is not waiting for it")
	}
	testMempoolOK(t)

	// a conflicting tx, paying more, gets in
	u := testAddUtxo(db, 2, 100000)
	x := testTx(90000, 1, btc.TxPrevOut{Hash: u})
	y := testTx(50000, 1, btc.TxPrevOut{Hash: u})
	res = submit(x, func() {
		if !SubmitLocalTx(y, y.Raw) {
			t.Error("conflicting tx rejected")
		}
	})
	if res != TX_REJECTED_RBF_LOWFEE || testInPool(x) || !testInPool(y) {
		t.Fatal("better conflicting tx added while verifying: expected RBF_LOWFEE, got", ReasonToString(res))
	}
	testMempoolOK(t)

	// a conflicting tx, paying less, gets in - so it is the one that gets replaced
	u = testAddUtxo(db, 3, 100000)
	x = testTx(50000, 1, btc.TxPrevOut{Hash: u})
	y = testTx(90000, 1, btc.TxPrevOut{Hash: u})
	res = submit(x, func() {
		if !SubmitLocalTx(y, y.Raw) {
			t.Error("conflicting tx rejected")
		}
	})
	if res != 0 || !testInPool(x) || testInPool(y) {
		t.Fatal("worse conflicting tx added while verifying: expected it to be replaced, got", ReasonToString(res))
	}
	testMempoolOK(t)

	// the whole mempool gets wiped
	p = testTx(90000, 1, btc.TxPrevOut{Hash: testAddUtxo(db, 4, 100000)})
	if testSubmit(p) != 0 {
		t.Fatal("parent rejected")
	}
	c = testTx(80000, 1, btc.TxPrevOut{Hash: p.Hash.Hash, Vout: 0})
	res = submit(c, InitMempool)
	if res != TX_REJECTED_NO_TXOU || len(TransactionsToSend) != 0 || len(TransactionsPending) != 0 {
		t.Fatal("mempool wiped while verifying: expected NO_TXOU, got", ReasonToString(res))
	}
	testMempoolOK(t)
}

// Sorting the mempool must not affect the minimum fee, as long as nothing gets removed.
func TestSortingKeepsMinFee(t *testing.T) {
	db := testSetup()
	CurrentFeeAdjustedSPKB = 0
	for i := 1; i <= 5; i++ {
		u := testAddUtxo(db, i, 100000)
		if testSubmit(testTx(uint64(100000-i*5000), 1, btc.TxPrevOut{Hash: u, Vout: 0})) != 0 {
			t.Fatal("tx rejected")
		}
	}
	TxMutex.Lock()
	cnt := len(GetSortedMempoolRBF())
	TxMutex.Unlock()
	if cnt != 5 {
		t.Fatal("bad number of sorted txs:", cnt)
	}
	if CurrentFeeAdjustedSPKB != 0 {
		t.Fatal("GetSortedMempoolRBF() changed CurrentFeeAdjustedSPKB to", CurrentFeeAdjustedSPKB)
	}
}

func TestMempoolSaveLoad(t *testing.T) {
	db := testSetup()
	common.GocoinHomeDir = t.TempDir() + string(os.PathSeparator)
	common.Last.Block = &chain.BlockTreeNode{BlockHash: btc.NewUint256(make([]byte, 32))}
	defer func() { common.Last.Block = nil }()

	t1 := testTx(90000, 1, btc.TxPrevOut{Hash: testAddUtxo(db, 1, 100000), Vout: 0})
	t2 := testTx(80000, 1, btc.TxPrevOut{Hash: testAddUtxo(db, 2, 100000), Vout: 0})
	t3 := testTx(70000, 1, btc.TxPrevOut{Hash: t2.Hash.Hash, Vout: 0})
	orphan := testTx(500, 1, btc.TxPrevOut{Hash: [32]byte{9, 9, 9}, Vout: 0})
	for _, x := range []*btc.Tx{t1, t2, t3} {
		if testSubmit(x) != 0 {
			t.Fatal("tx rejected")
		}
	}
	testSubmit(orphan)
	sent := time.Unix(1700000000, 0)
	TransactionsToSend[t2.Hash.BIdx()].Lastsent = sent

	MempoolSave(true)
	if !MempoolLoad() {
		t.Fatal("MempoolLoad failed")
	}
	if len(TransactionsToSend) != 3 || len(TransactionsRejected) != 1 || len(WaitingForInputs) != 1 {
		t.Fatal("bad counts after loading:", len(TransactionsToSend), len(TransactionsRejected), len(WaitingForInputs))
	}
	if r := TransactionsToSend[t1.Hash.BIdx()]; r == nil || !r.Lastsent.IsZero() {
		t.Fatal("tx that was never sent has Lastsent set after loading")
	}
	if r := TransactionsToSend[t2.Hash.BIdx()]; r == nil || !r.Lastsent.Equal(sent) {
		t.Fatal("Lastsent not preserved")
	}
	if r := TransactionsToSend[t3.Hash.BIdx()]; r == nil || r.MemInputCnt != 1 {
		t.Fatal("MemInputs not recovered")
	}
	testMempoolOK(t)
}
