package txpool

import (
	"testing"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/lib/utxo"
)

// testSetup prepares an empty mempool on top of an empty, memory-only UTXO db.
func testSetup() *utxo.UnspentDB {
	db := new(utxo.UnspentDB)
	for i := range db.HashMap {
		db.HashMap[i] = make(map[utxo.UtxoKeyType]*[]byte)
	}
	common.BlockChain = &chain.Chain{Unspent: db}
	common.CFG.TXPool.RejectRecCnt = 100
	common.CFG.TXPool.MaxTxWeight = 400e3
	common.CFG.TXPool.AllowMemInputs = true
	common.MaxRejectedSizeBytes = 100e6
	common.MaxNoUtxoSizeBytes = 100e6
	InitMempool()
	return db
}

// testAddUtxo adds a confirmed tx with a single output of the given value to the UTXO db.
func testAddUtxo(db *utxo.UnspentDB, id byte, value uint64) (txid [32]byte) {
	txid[0], txid[31] = id, 0xaa
	rec := &utxo.UtxoRec{TxID: txid, InBlock: 100, Outs: []*utxo.UtxoTxOut{{PKScr: []byte{0x51}, Value: value}}}
	var k utxo.UtxoKeyType
	copy(k[:], txid[:])
	db.HashMap[k[0]][k] = utxo.Serialize(rec, nil)
	return
}

// testTx makes a tx spending the given inputs, with nouts outputs of outval each.
func testTx(outval uint64, nouts int, ins ...btc.TxPrevOut) *btc.Tx {
	t := &btc.Tx{Version: 2}
	for _, in := range ins {
		t.TxIn = append(t.TxIn, &btc.TxIn{Input: in, Sequence: 0xfffffffd})
	}
	for i := 0; i < nouts; i++ {
		t.TxOut = append(t.TxOut, &btc.TxOut{Value: outval, Pk_script: []byte{0x51}})
	}
	raw := t.Serialize()
	tx, _ := btc.NewTx(raw)
	tx.SetHash(raw)
	return tx
}

// testSubmit passes the tx through HandleNetTx, like if it came from a peer, and returns the result.
// Txs are submitted as trusted, to skip the script verification (which needs more of the chain's state).
func testSubmit(tx *btc.Tx) (res byte) {
	TxMutex.Lock()
	TransactionsPending[tx.Hash.BIdx()] = true
	TxMutex.Unlock()
	HandleNetTx(&TxRcvd{Tx: tx, Trusted: true, FeedbackCB: func(n *TxRcvd, _ *OneTxToSend) { res = n.Result }})
	return
}

func testMempoolOK(t *testing.T) {
	t.Helper()
	TxMutex.Lock()
	bad := MempoolCheck()
	TxMutex.Unlock()
	if bad {
		t.Fatal("MempoolCheck reports errors")
	}
}

func testInPool(tx *btc.Tx) bool {
	_, ok := TransactionsToSend[tx.Hash.BIdx()]
	return ok
}

// An orphan waiting for a non-existing output of a tx that gets mined without having been
// in our mempool used to be retried forever, hanging BlockMined() with TxMutex locked.
func TestOrphanOfMinedTxWithBadVout(t *testing.T) {
	testSetup()
	p := testTx(1000, 2, btc.TxPrevOut{Hash: [32]byte{1, 2, 3}, Vout: 0}) // has 2 outputs, we never see it
	c1 := testTx(500, 1, btc.TxPrevOut{Hash: p.Hash.Hash, Vout: 7})
	c2 := testTx(400, 1, btc.TxPrevOut{Hash: p.Hash.Hash, Vout: 9})
	for _, x := range []*btc.Tx{c1, c2} {
		if res := testSubmit(x); res != TX_REJECTED_NO_TXOU {
			t.Fatal("orphan: expected NO_TXOU, got", ReasonToString(res))
		}
	}
	if len(WaitingForInputs) != 1 {
		t.Fatal("orphans not on the waiting list")
	}

	done := make(chan bool)
	go func() {
		BlockMined(&btc.Block{Txs: []*btc.Tx{testTx(1, 1, btc.TxPrevOut{}), p}})
		done <- true
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("BlockMined() hangs") // note that TxMutex stays locked, so the other tests would hang too
	}

	for _, x := range []*btc.Tx{c1, c2} {
		if txr := TransactionsRejected[x.Hash.BIdx()]; txr == nil || txr.Reason != TX_REJECTED_BAD_INPUT || txr.Tx != nil {
			t.Fatal("orphan with bad vout not marked as BAD_INPUT")
		}
	}
	if len(WaitingForInputs) != 0 || WaitingForInputsSize != 0 {
		t.Fatal("waiting list not empty:", len(WaitingForInputs), WaitingForInputsSize)
	}
	testMempoolOK(t)
}

// Orphans must get accepted when their parent arrives - except for the one with a bad vout.
func TestOrphansAcceptedWithParent(t *testing.T) {
	db := testSetup()
	u := testAddUtxo(db, 1, 100000)
	p := testTx(45000, 2, btc.TxPrevOut{Hash: u, Vout: 0})
	c1 := testTx(40000, 1, btc.TxPrevOut{Hash: p.Hash.Hash, Vout: 0})
	c2 := testTx(40000, 1, btc.TxPrevOut{Hash: p.Hash.Hash, Vout: 1})
	g := testTx(35000, 1, btc.TxPrevOut{Hash: c1.Hash.Hash, Vout: 0}) // grandchild
	bad := testTx(100, 1, btc.TxPrevOut{Hash: p.Hash.Hash, Vout: 5})
	for _, x := range []*btc.Tx{g, c1, bad, c2} {
		if res := testSubmit(x); res != TX_REJECTED_NO_TXOU {
			t.Fatal("orphan: expected NO_TXOU, got", ReasonToString(res))
		}
	}
	if res := testSubmit(p); res != 0 {
		t.Fatal("parent rejected:", ReasonToString(res))
	}
	for _, x := range []*btc.Tx{p, c1, c2, g} {
		if !testInPool(x) {
			t.Fatal("tx missing from the mempool")
		}
	}
	if len(TransactionsToSend) != 4 || len(WaitingForInputs) != 0 {
		t.Fatal("bad counts:", len(TransactionsToSend), len(WaitingForInputs))
	}
	if txr := TransactionsRejected[bad.Hash.BIdx()]; txr == nil || txr.Reason != TX_REJECTED_BAD_INPUT {
		t.Fatal("orphan with bad vout not marked as BAD_INPUT")
	}
	testMempoolOK(t)
}

// A replacement must not spend outputs of the txs it replaces (nor of their children),
// as it would stay in the mempool with a non-existing parent.
func TestRBFSpendingReplacedTx(t *testing.T) {
	db := testSetup()
	u := testAddUtxo(db, 1, 100000)
	p := testTx(90000, 1, btc.TxPrevOut{Hash: u, Vout: 0})
	ch := testTx(85000, 1, btc.TxPrevOut{Hash: p.Hash.Hash, Vout: 0})
	if testSubmit(p) != 0 || testSubmit(ch) != 0 {
		t.Fatal("setup txs rejected")
	}

	for _, parent := range []*btc.Tx{p, ch} {
		c := testTx(100000, 1, btc.TxPrevOut{Hash: u, Vout: 0}, btc.TxPrevOut{Hash: parent.Hash.Hash, Vout: 0})
		if res := testSubmit(c); res != TX_REJECTED_BAD_INPUT {
			t.Fatal("replacement spending a replaced tx: expected BAD_INPUT, got", ReasonToString(res))
		}
		if len(TransactionsToSend) != 2 || !testInPool(p) || !testInPool(ch) {
			t.Fatal("mempool changed by a rejected replacement")
		}
		testMempoolOK(t)
	}

	// an ordinary replacement must still work, removing the replaced tx with its child
	r := testTx(50000, 1, btc.TxPrevOut{Hash: u, Vout: 0})
	if res := testSubmit(r); res != 0 {
		t.Fatal("ordinary replacement rejected:", ReasonToString(res))
	}
	if len(TransactionsToSend) != 1 || !testInPool(r) {
		t.Fatal("ordinary replacement did not replace")
	}
	testMempoolOK(t)
}

// Spending the same output twice used to be accepted, with the input's value counted twice.
func TestDuplicateInput(t *testing.T) {
	db := testSetup()
	u := testAddUtxo(db, 2, 100000)
	d := testTx(150000, 1, btc.TxPrevOut{Hash: u, Vout: 0}, btc.TxPrevOut{Hash: u, Vout: 0})
	if res := testSubmit(d); res != TX_REJECTED_BAD_INPUT {
		t.Fatal("duplicate input: expected BAD_INPUT, got", ReasonToString(res))
	}
	if len(TransactionsToSend) != 0 {
		t.Fatal("tx with a duplicate input is in the mempool")
	}
	testMempoolOK(t)
}
