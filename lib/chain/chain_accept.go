package chain

import (
	"errors"
	"fmt"
	"runtime"
	"slices"
	"sync"
	"sync/atomic"

	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/script"
	"github.com/piotrnar/gocoin/lib/utxo"
)

// TrustedTxChecker is meant to speed up verifying transactions that had
// been verified already by the client while being taken to its memory pool
var TrustedTxChecker func(*btc.Tx) bool

func (ch *Chain) ProcessBlockTransactions(bl *btc.Block, height, lknown uint32) (changes *utxo.BlockChanges, sigopscost uint32, e error) {
	changes = new(utxo.BlockChanges)
	changes.Height = height
	changes.LastKnownHeight = lknown
	changes.DeledTxs = make(map[[32]byte][]bool, bl.TotalInputs)
	sigopscost, e = ch.commitTxs(bl, changes)
	return
}

// AcceptBlock either appends a new block at the end of the existing chain
// in which case it also applies all the transactions to the unspent database.
// If the block does is not the heighest, it is added to the chain, but maked
// as an orphan - its transaction will be verified only if the chain would swap
// to its branch later on.
func (ch *Chain) AcceptBlock(bl *btc.Block) (e error) {
	ch.BlockIndexAccess.Lock()
	cur := ch.AcceptHeader(bl)
	ch.BlockIndexAccess.Unlock()
	return ch.CommitBlock(bl, cur)
}

// Make sure to call this function with ch.BlockIndexAccess locked
func (ch *Chain) AcceptHeader(bl *btc.Block) (cur *BlockTreeNode) {
	prevblk, ok := ch.BlockIndex[btc.NewUint256(bl.ParentHash()).BIdx()]
	if !ok {
		panic("This should not happen")
	}

	// create new BlockTreeNode
	cur = new(BlockTreeNode)
	cur.BlockHash = bl.Hash
	cur.Parent = prevblk
	cur.Height = prevblk.Height + 1
	copy(cur.BlockHeader[:], bl.Raw[:80])

	// Add this block to the block index
	prevblk.addChild(cur)
	ch.BlockIndex[cur.BlockHash.BIdx()] = cur

	return
}

func (ch *Chain) CommitBlock(bl *btc.Block, cur *BlockTreeNode) (e error) {
	cur.BlockSize = uint32(len(bl.Raw))
	cur.TxCount = uint32(bl.TxCount)
	if ch.LastBlock() == cur.Parent {
		// The head of out chain - apply the transactions
		var changes *utxo.BlockChanges
		var sigopscost uint32
		changes, sigopscost, e = ch.ProcessBlockTransactions(bl, cur.Height, bl.LastKnownHeight)
		if e != nil {
			// ProcessBlockTransactions failed, so trash the block.
			//fmt.Println("ProcessBlockTransactionsA", cur.BlockHash.String(), cur.Height, e.Error())
			ch.BlockIndexAccess.Lock()
			cur.Parent.delChild(cur)
			delete(ch.BlockIndex, cur.BlockHash.BIdx())
			ch.BlockIndexAccess.Unlock()
		} else {
			cur.SigopsCost = sigopscost
			// ProcessBlockTransactions succeeded, so save the block as "trusted".
			bl.Trusted.Set()
			ch.Blocks.BlockAdd(cur.Height, bl)
			// Apply the block's trabnsactions to the unspent database:
			ch.Unspent.CommitBlockTxs(changes, bl.Hash.Hash[:])
			ch.SetLast(cur) // Advance the head
			if ch.CB.BlockMinedCB != nil {
				ch.CB.BlockMinedCB(bl)
			}
		}
		bl.Clean()
	} else {
		// The block's parent is not the current head of the chain...

		// Save the block, though do not mark it as "trusted" just yet
		ch.Blocks.BlockAdd(cur.Height, bl)

		// If it has more POW than the current head, move the head to it
		if cur.MorePOW(ch.LastBlock()) {
			ch.MoveToBlock(cur)
			if ch.LastBlock() != cur {
				e = errors.New("CommitBlock: MoveToBlock failed")
			}
		} else {
			if !ch.testnet() {
				fmt.Println("Orphaned block", bl.Hash.String(), cur.Height)
			}
		}
	}

	return
}

// enforceBIP30 tells whether transactions of the block at the given height must be checked
// for not overwriting the existing ones that have not been fully spent (BIP30).
// Since BIP34 a coinbase (so also any other tx) cannot have the same ID as an earlier one,
// until the height at which the number that BIP34 puts in the coinbase could match
// a script of some coinbase mined before BIP34 (BIP34ImpliesBIP30Limit).
// Unlike satoshi's client, we assume that the block at BIP34Height is the known one.
func (ch *Chain) enforceBIP30(height uint32, hash *btc.Uint256) bool {
	if ch.testnet4() && ch.testnet() {
		return true // BIP34 was active from the first block, with no reference block hash
	}
	if height >= BIP34ImpliesBIP30Limit {
		return true
	}
	if height > ch.Consensus.BIP34Height {
		return false
	}
	if !ch.testnet() {
		// The two blocks that had violated the rule before it was introduced
		switch height {
		case 91842:
			return hash.String() != "00000000000a4d0a398161ffc163c503763b1f4360639393e0e4c8e300e0caec"
		case 91880:
			return hash.String() != "00000000000743f190a18c5577a3c2d2a1f610ae9601ac046a38084ccb7cd721"
		}
	}
	return true
}

// commitTxs is ususually the most time consuming process when applying a new block.
func (ch *Chain) commitTxs(bl *btc.Block, changes *utxo.BlockChanges) (sigopscost uint32, e error) {
	sumblockin := btc.GetBlockReward(changes.Height)
	var txoutsum, txinsum, sumblockout uint64

	if changes.Height+ch.Unspent.UnwindBufLen >= changes.LastKnownHeight {
		changes.UndoData = make(map[[32]byte]*utxo.UtxoRec)
	}

	blUnsp := make(map[[32]byte][]*btc.TxOut, len(bl.Txs))

	var seq_locks seqLocks
	check_seq_locks := (bl.VerifyFlags & script.VER_CSV) != 0
	check_bip30 := !bl.Trusted.Get() && ch.enforceBIP30(changes.Height, bl.Hash)

	var wg sync.WaitGroup
	var wait4compl bool
	var ver_err_cnt uint32

	// The scripts are verified by a fixed number of workers (started when the first one is needed)
	type verJob struct {
		tx  *btc.Tx
		idx int
	}
	var ver_jobs chan verJob
	var ver_abort atomic.Bool

	defer func() { // this is for when it returns from inside the range bl.Txs loop
		if ver_jobs != nil {
			if e != nil {
				ver_abort.Store(true) // the block is invalid anyway, so skip the scripts still waiting
			}
			close(ver_jobs) // this makes the workers exit, after they are done with the queue
		}
		if wait4compl {
			wg.Wait()
		}
	}()

	for i, tx := range bl.Txs {
		tx.AllocVerVars()
		txoutsum, txinsum = 0, 0

		sigopscost += uint32(btc.WITNESS_SCALE_FACTOR * tx.GetLegacySigOpCount())

		// BIP30 - do not allow overwriting of a transaction that still has unspent outputs
		if check_bip30 && ch.Unspent.TxPresent(&tx.Hash) {
			po := btc.TxPrevOut{Hash: tx.Hash.Hash}
			for j := range tx.TxOut {
				// satoshi's client does not keep unspendable outputs in its UTXO set, so skip them
				if scr := tx.TxOut[j].Pk_script; len(scr) > 0 && scr[0] == btc.OP_RETURN || len(scr) > script.MAX_SCRIPT_SIZE {
					continue
				}
				if po.Vout = uint32(j); ch.Unspent.UnspentGet(&po) != nil {
					e = errors.New("commitTxs(): tried to overwrite transaction " + tx.Hash.String() + " - RPC_Result:bad-txns-BIP30")
					return
				}
			}
		}

		// Check each tx for a valid input, except from the first one
		if i > 0 {
			tx_trusted := bl.Trusted.Get()
			if !tx_trusted {
				if TrustedTxChecker != nil && TrustedTxChecker(tx) {
					tx_trusted = true
				} else {
					tx.Spent_outputs = make([]*btc.TxOut, len(tx.TxIn))
				}
			}

			// first collect all the inputs, their amounts and spend scripts
			for j := range tx.TxIn {
				var coin_height uint32
				inp := &tx.TxIn[j].Input
				spent_map, was_spent := changes.DeledTxs[inp.Hash]
				if was_spent {
					if int(inp.Vout) >= len(spent_map) {
						fmt.Println("txin", inp.String(), "did not have vout", inp.Vout)
						e = errors.New("tx VOut too big")
						return
					}

					if spent_map[inp.Vout] {
						fmt.Println("txin", inp.String(), "already spent in this block")
						e = errors.New("double spend inside the block")
						return
					}
				}
				tout := ch.Unspent.UnspentGet(inp)
				if tout == nil {
					t, ok := blUnsp[inp.Hash]
					if !ok {
						e = errors.New("Unknown input TxID: " + btc.NewUint256(inp.Hash[:]).String())
						return
					}

					if inp.Vout >= uint32(len(t)) {
						fmt.Println("Vout too big", len(t), inp.String())
						e = errors.New("vout too big")
						return
					}

					if t[inp.Vout] == nil {
						fmt.Println("Vout already spent", inp.String())
						e = errors.New("vout already spent")
						return
					}

					if t[inp.Vout].WasCoinbase {
						e = errors.New("Cannot spend block's own coinbase in TxID: " + btc.NewUint256(inp.Hash[:]).String())
						return
					}

					tout = t[inp.Vout]
					t[inp.Vout] = nil // and now mark it as spent:
					coin_height = changes.Height
				} else {
					if tout.WasCoinbase && changes.Height-tout.BlockHeight < COINBASE_MATURITY {
						e = errors.New("Trying to spend prematured coinbase: " + btc.NewUint256(inp.Hash[:]).String())
						return
					}
					// it is confirmed already so delete it later
					if !was_spent {
						spent_map = make([]bool, tout.VoutCount)
						changes.DeledTxs[inp.Hash] = spent_map
					}
					spent_map[inp.Vout] = true
					coin_height = tout.BlockHeight

					if changes.UndoData != nil {
						var urec *utxo.UtxoRec
						urec = changes.UndoData[inp.Hash]
						if urec == nil {
							urec = new(utxo.UtxoRec)
							urec.TxID = inp.Hash
							urec.Coinbase = tout.WasCoinbase
							urec.InBlock = tout.BlockHeight
							urec.Outs = make([]*utxo.UtxoTxOut, tout.VoutCount)
							changes.UndoData[inp.Hash] = urec
						}
						tmp := new(utxo.UtxoTxOut)
						tmp.Value = tout.Value
						tmp.PKScr = make([]byte, len(tout.Pk_script))
						copy(tmp.PKScr, tout.Pk_script)
						urec.Outs[inp.Vout] = tmp
					}
				}

				if !tx_trusted {
					tx.Spent_outputs[j] = tout
				}

				// BIP68 - relative lock-time of version 2 (or higher) transactions
				if check_seq_locks && tx.Version >= 2 {
					seq_locks.add(tx.TxIn[j].Sequence, coin_height)
				}

				if (bl.VerifyFlags & script.VER_P2SH) != 0 {
					if btc.IsP2SH(tout.Pk_script) {
						sigopscost += uint32(btc.WITNESS_SCALE_FACTOR * btc.GetP2SHSigOpCount(tx.TxIn[j].ScriptSig))
					}
				}

				if (bl.VerifyFlags & script.VER_WITNESS) != 0 {
					sigopscost += uint32(tx.CountWitnessSigOps(j, tout.Pk_script))
				}

				txinsum += tout.Value
			}

			// second, verify the scrips:
			if !tx_trusted { // run VerifyTxScript() in parallel tasks
				if ver_jobs == nil {
					workers := runtime.GOMAXPROCS(0)
					ver_jobs = make(chan verJob, 16*workers)
					for ; workers > 0; workers-- {
						go func() {
							for job := range ver_jobs {
								tx, i := job.tx, job.idx
								if !ver_abort.Load() && !script.VerifyTxScript(tx.Spent_outputs[i].Pk_script,
									&script.SigChecker{Amount: tx.Spent_outputs[i].Value, Idx: i, Tx: tx}, bl.VerifyFlags) {
									atomic.AddUint32(&ver_err_cnt, 1)
								}
								wg.Done()
							}
						}()
					}
				}
				wait4compl = true
				for j := range tx.TxIn {
					wg.Add(1)
					ver_jobs <- verJob{tx: tx, idx: j}
				}
			}
		} else {
			// For coinbase tx we need to check (like satoshi) whether the script size is between 2 and 100 bytes
			// (Previously we made sure in CheckBlock() that this was a coinbase type tx)
			if len(tx.TxIn[0].ScriptSig) < 2 || len(tx.TxIn[0].ScriptSig) > 100 {
				e = errors.New(fmt.Sprint("Coinbase script has a wrong length ", len(tx.TxIn[0].ScriptSig)))
				return
			}
		}

		// Make sure the sums cannot wrap around. This is also enforced by
		// CheckTransaction(), but that one is skipped for trusted blocks.
		if txinsum > btc.MAX_MONEY {
			e = errors.New("commitTxs(): input values out of range - RPC_Result:bad-txns-inputvalues-outofrange")
			return
		}
		sumblockin += txinsum

		for j := range tx.TxOut {
			if tx.TxOut[j].Value > btc.MAX_MONEY {
				e = errors.New("commitTxs(): txout.Value too high - RPC_Result:bad-txns-vout-toolarge")
				return
			}
			txoutsum += tx.TxOut[j].Value
			if txoutsum > btc.MAX_MONEY {
				e = errors.New("commitTxs(): txout total too high - RPC_Result:bad-txns-txouttotal-toolarge")
				return
			}
		}
		sumblockout += txoutsum

		if sumblockin > btc.MAX_MONEY || sumblockout > btc.MAX_MONEY {
			e = errors.New("commitTxs(): block values out of range - RPC_Result:bad-txns-inputvalues-outofrange")
			return
		}

		if i > 0 {
			tx.CalculatedFee = txinsum - txoutsum
			if txoutsum > txinsum {
				e = fmt.Errorf("more spent (%.8f) than at the input (%.8f) in TX %s",
					float64(txoutsum)/1e8, float64(txinsum)/1e8, tx.Hash.String())
				return
			}
		}

		// Add each tx outs from the currently executed TX to the temporary pool
		// Clone required as signature verification goroutines access this data
		// concurrently while main thread marks outputs spent (t[inp.Vout] = nil)
		blUnsp[tx.Hash.Hash] = slices.Clone(tx.TxOut)
	}

	if !bl.Trusted.Get() {
		wait4compl = false
		wg.Wait()
		if ver_err_cnt > 0 {
			fmt.Println("VerifyScript failed", ver_err_cnt, "time (s)")
			e = errors.New(fmt.Sprint("VerifyScripts failed ", ver_err_cnt, "time (s)"))
			return
		}
		if check_seq_locks {
			if e = ch.checkSequenceLocks(&seq_locks, changes.Height); e != nil {
				return
			}
		}
	}

	if sumblockin < sumblockout { // could do it only for non-trusted blocks but this check is cheap
		e = fmt.Errorf("out:%d > in:%d", sumblockout, sumblockin)
		return
	}

	if sigopscost > btc.MAX_BLOCK_SIGOPS_COST {
		// could do it only for non-trusted blocks but this check is cheap
		// (we had to calculate sigopscost for the caller anyway)
		e = errors.New("commitTxs(): too many sigops - RPC_Result:bad-blk-sigops")
		return
	}

	var rec *utxo.UtxoRec
	changes.AddList = make([]*utxo.UtxoRec, 0, len(blUnsp))
	for k, v := range blUnsp {
		for i := range v {
			if v[i] != nil {
				if rec == nil {
					rec = new(utxo.UtxoRec)
					rec.TxID = k
					rec.Coinbase = v[i].WasCoinbase
					rec.InBlock = changes.Height
					rec.Outs = make([]*utxo.UtxoTxOut, len(v))
				}
				rec.Outs[i] = &utxo.UtxoTxOut{Value: v[i].Value, PKScr: v[i].Pk_script}
			}
		}
		if rec != nil {
			changes.AddList = append(changes.AddList, rec)
			rec = nil
		}
	}

	return
}

// CheckTransactions checks transactions for consistency and finality.
// Return nil if OK, otherwise a descripive error.
func CheckTransactions(txs []*btc.Tx, height, btime uint32) (res error) {
	check := func(tx *btc.Tx) error {
		if er := tx.CheckTransaction(); er != nil {
			return er
		}
		if !tx.IsFinal(height, btime) {
			return errors.New("CheckTransactions() : not-final transaction - RPC_Result:bad-txns-nonfinal")
		}
		return nil
	}

	// Checking one tx takes very little, so do not start more tasks than it is worth.
	const min_txs_per_task = 256
	tasks := runtime.GOMAXPROCS(0)
	if n := len(txs) / min_txs_per_task; n < tasks {
		tasks = n
	}
	if tasks < 2 {
		for _, tx := range txs {
			if res = check(tx); res != nil {
				return
			}
		}
		return
	}

	var wg sync.WaitGroup
	var mut sync.Mutex
	var abort atomic.Bool // set when an error has already been reported

	per_task := (len(txs) + tasks - 1) / tasks
	for offs := 0; offs < len(txs); offs += per_task {
		end := offs + per_task
		if end > len(txs) {
			end = len(txs)
		}
		wg.Add(1)
		go func(txs []*btc.Tx) {
			defer wg.Done()
			for _, tx := range txs {
				if abort.Load() {
					return
				}
				if er := check(tx); er != nil {
					abort.Store(true)
					mut.Lock()
					if res == nil {
						res = er
					}
					mut.Unlock()
					return
				}
			}
		}(txs[offs:end])
	}
	wg.Wait() // wait for all the goroutines to complete

	return
}
