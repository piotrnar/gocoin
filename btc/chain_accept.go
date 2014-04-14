package btc

import (
	"fmt"
	"errors"
)

// TrustedTxChecker is meant to speed up verifying transactions that had
// been verified already by the client while being taken to its memory pool
var TrustedTxChecker func(*Uint256) bool


func (ch *Chain) ProcessBlockTransactions(bl *Block, height uint32) (changes *BlockChanges, e error) {
	changes = new(BlockChanges)
	changes.Height = height
	changes.DeledTxs = make(map[TxPrevOut]*TxOut)
	changes.AddedTxs = make(map[TxPrevOut]*TxOut)
	e = ch.commitTxs(bl, changes)
	return
}


// This function either appends a new block at the end of the existing chain
// in which case it also applies all the transactions to the unspent database.
// If the block does is not the heighest, it is added to the chain, but maked
// as an orphan - its transaction will be verified only if the chain would swap
// to its branch later on.
func (ch *Chain)AcceptBlock(bl *Block) (e error) {

	prevblk, ok := ch.BlockIndex[NewUint256(bl.ParentHash()).BIdx()]
	if !ok {
		panic("This should not happen")
	}

	// create new BlockTreeNode
	cur := new(BlockTreeNode)
	cur.BlockHash = bl.Hash
	cur.Parent = prevblk
	cur.Height = prevblk.Height + 1
	cur.TxCount = uint32(bl.TxCount)
	copy(cur.BlockHeader[:], bl.Raw[:80])

	// Add this block to the block index
	ch.BlockIndexAccess.Lock()
	prevblk.addChild(cur)
	ch.BlockIndex[cur.BlockHash.BIdx()] = cur
	ch.BlockIndexAccess.Unlock()

	if ch.BlockTreeEnd==prevblk {
		// The head of out chain - apply the transactions
		if don(DBG_BLOCKS) {
			fmt.Printf("Adding block %s @ %d\n", cur.BlockHash.String(), cur.Height)
		}
		var changes *BlockChanges
		changes, e = ch.ProcessBlockTransactions(bl, cur.Height)
		if e != nil {
			// ProcessBlockTransactions failed, so trash the block.
			println("ProcessBlockTransactions ", cur.BlockHash.String(), cur.Height, e.Error())
			ch.BlockIndexAccess.Lock()
			cur.Parent.delChild(cur)
			delete(ch.BlockIndex, cur.BlockHash.BIdx())
			ch.BlockIndexAccess.Unlock()
		} else {
			// ProcessBlockTransactions succeeded, so save the block as "trusted".
			bl.Trusted = true
			ch.Blocks.BlockAdd(cur.Height, bl)
			// Apply the block's trabnsactions to the unspent database:
			changes.LastKnownHeight = bl.LastKnownHeight
			ch.Unspent.CommitBlockTxs(changes, bl.Hash.Hash[:])
			if !ch.DoNotSync {
				ch.Blocks.Sync()
			}
			ch.BlockTreeEnd = cur // Advance the head
		}
	} else {
		// The block's parent is not the current head of the chain...

		// Save the block, though do not makt it as "trusted" just yet
		ch.Blocks.BlockAdd(cur.Height, bl)

		// If it has a bigger height than the current head,
		// ... move the coin state into a new branch.
		if cur.Height > ch.BlockTreeEnd.Height {
			ch.MoveToBlock(cur)
		} else if don(DBG_BLOCKS|DBG_ORPHAS) {
			fmt.Printf("Orphan block %s @ %d\n", cur.BlockHash.String(), cur.Height)
		}
	}

	return
}


// This isusually the most time consuming process when applying a new block
func (ch *Chain)commitTxs(bl *Block, changes *BlockChanges) (e error) {
	sumblockin := GetBlockReward(changes.Height)
	var txoutsum, txinsum, sumblockout uint64

	if don(DBG_TX) {
		fmt.Printf("Commiting %d transactions\n", len(bl.Txs))
	}

	// Add each tx outs from the current block to the temporary pool
	blUnsp := make(map[[32]byte] []*TxOut, len(bl.Txs))
	for i := range bl.Txs {
		outs := make([]*TxOut, len(bl.Txs[i].TxOut))
		for j := range bl.Txs[i].TxOut {
			bl.Txs[i].TxOut[j].BlockHeight = changes.Height
			outs[j] = bl.Txs[i].TxOut[j]
		}
		blUnsp[bl.Txs[i].Hash.Hash] = outs
	}

	// create a channnel to receive results from VerifyScript threads:
	done := make(chan bool, UseThreads)

	for i := range bl.Txs {
		if don(DBG_TX) {
			fmt.Printf("tx %d/%d:\n", i+1, len(bl.Txs))
		}
		txoutsum, txinsum = 0, 0

		// Check each tx for a valid input, except from the first one
		if i>0 {
			tx_trusted := bl.Trusted
			if !tx_trusted && TrustedTxChecker!=nil && TrustedTxChecker(bl.Txs[i].Hash) {
				tx_trusted = true
			}

			scripts_ok := true

			for j:=0; j<UseThreads; j++ {
				done <- true
			}

			for j:=0; j<len(bl.Txs[i].TxIn) /*&& e==nil*/; j++ {
				inp := &bl.Txs[i].TxIn[j].Input
				if _, ok := changes.DeledTxs[*inp]; ok {
					println("txin", inp.String(), "already spent in this block")
					e = errors.New("Input spent more then once in same block")
					break
				}
				tout := ch.PickUnspent(inp)
				if tout==nil {
					if don(DBG_TX) {
						println("PickUnspent failed")
					}
					t, ok := blUnsp[inp.Hash]
					if !ok {
						e = errors.New("Unknown input TxID: " + NewUint256(inp.Hash[:]).String())
						break
					}

					if inp.Vout>=uint32(len(t)) {
						println("Vout too big", len(t), inp.String())
						e = errors.New("Vout too big")
						break
					}

					if t[inp.Vout] == nil {
						println("Vout already spent", inp.String())
						e = errors.New("Vout already spent")
						break
					}

					tout = t[inp.Vout]
					t[inp.Vout] = nil // and now mark it as spent:
					if don(DBG_TX) {
						println("TxInput from the current block", inp.String())
					}
				} else {
					if don(DBG_TX) {
						println("PickUnspent OK")
					}
				}

				if !(<-done) {
					println("VerifyScript error 1")
					scripts_ok = false
					break
				}

				if tx_trusted {
					done <- true
				} else {
				    go func (sig []byte, prv []byte, i int, tx *Tx) {
						done <- VerifyTxScript(sig, prv, i, tx, bl.BlockTime()>=BIP16SwitchTime)
					}(bl.Txs[i].TxIn[j].ScriptSig, tout.Pk_script, j, bl.Txs[i])
				}

				// Verify Transaction script:
				txinsum += tout.Value
				changes.DeledTxs[*inp] = tout

				if don(DBG_TX) {
					fmt.Printf("  in %d: %.8f BTC @ %s\n", j+1, float64(tout.Value)/1e8,
						bl.Txs[i].TxIn[j].Input.String())
				}

			}

			if scripts_ok {
				scripts_ok = <- done
			}
			for j:=1; j<UseThreads; j++ {
				if !(<- done) {
					println("VerifyScript error 2")
					scripts_ok = false
				}
			}
			if len(done) != 0 {
				panic("ASSERT: The channel should be empty gere")
			}

			if !scripts_ok {
				return errors.New("VerifyScripts failed")
			}
		} else {
			if don(DBG_TX) {
				fmt.Printf("  mined %.8f\n", float64(sumblockin)/1e8)
			}
			// For coinbase tx we need to check (like satoshi) whether the script size is between 2 and 100 bytes
			// (Previously we made sure in CheckBlock() that this was a coinbase type tx)
			if len(bl.Txs[0].TxIn[0].ScriptSig)<2 || len(bl.Txs[0].TxIn[0].ScriptSig)>100 {
				return errors.New(fmt.Sprint("Coinbase script has a wrong length", len(bl.Txs[0].TxIn[0].ScriptSig)))
			}
		}
		sumblockin += txinsum

		for j := range bl.Txs[i].TxOut {
			if don(DBG_TX) {
				fmt.Printf("  out %d: %12.8f\n", j+1, float64(bl.Txs[i].TxOut[j].Value)/1e8)
			}
			txoutsum += bl.Txs[i].TxOut[j].Value
			txa := new(TxPrevOut)
			copy(txa.Hash[:], bl.Txs[i].Hash.Hash[:])
			txa.Vout = uint32(j)
			_, spent := changes.DeledTxs[*txa]
			if spent {
				delete(changes.DeledTxs, *txa)
			} else {
				changes.AddedTxs[*txa] = bl.Txs[i].TxOut[j]
			}
		}
		sumblockout += txoutsum

		if don(DBG_TX) {
			fmt.Sprintf("  %12.8f -> %12.8f  (%.8f)\n",
				float64(txinsum)/1e8, float64(txoutsum)/1e8,
				float64(txinsum-txoutsum)/1e8)
		}

		if don(DBG_TX) && i>0 {
			fmt.Printf(" fee : %.8f\n", float64(txinsum-txoutsum)/1e8)
		}
		if e != nil {
			return // If any input fails, do not continue
		}
		if i>0 && txoutsum > txinsum {
			return errors.New(fmt.Sprintf("More spent (%.8f) than at the input (%.8f) in TX %s",
				float64(txoutsum)/1e8, float64(txinsum)/1e8, bl.Txs[i].Hash.String()))
		}
	}

	if sumblockin < sumblockout {
		return errors.New(fmt.Sprintf("Out:%d > In:%d", sumblockout, sumblockin))
	} else if don(DBG_WASTED) && sumblockin != sumblockout {
		fmt.Printf("%.8f BTC wasted in block %d\n", float64(sumblockin-sumblockout)/1e8, changes.Height)
	}

	return nil
}
