package btc

import (
	"fmt"
	"errors"
	"encoding/hex"
)


func (ch *Chain) ProcessBlockTransactions(bl *Block, height uint32) (changes *BlockChanges, e error) {
	changes = new(BlockChanges)
	changes.Height = height
	changes.DeledTxs = make(map[TxPrevOut]*TxOut)
	changes.AddedTxs = make(map[TxPrevOut]*TxOut)
	e = ch.commitTxs(bl, changes)
	return
}


func (ch *Chain)AcceptBlock(bl *Block) (e error) {

	prevblk, ok := ch.BlockIndex[NewUint256(bl.Parent).BIdx()]
	if !ok {
		panic("This should not happen")
	}

	// create new BlockTreeNode
	cur := new(BlockTreeNode)
	cur.BlockHash = bl.Hash
	cur.Parent = prevblk
	cur.Height = prevblk.Height + 1
	cur.Bits = bl.Bits
	cur.Timestamp = bl.BlockTime

	// Add this block to the block index
	ch.BlockIndexAccess.Lock()
	prevblk.addChild(cur)
	ch.BlockIndex[cur.BlockHash.BIdx()] = cur
	ch.BlockIndexAccess.Unlock()

	if ch.BlockTreeEnd==prevblk {
		// Append the end of the longest
		if don(DBG_BLOCKS) {
			fmt.Printf("Adding block %s @ %d\n", cur.BlockHash.String(), cur.Height)
		}
		var changes *BlockChanges
		changes, e = ch.ProcessBlockTransactions(bl, cur.Height)
		if e != nil {
			println("ProcessBlockTransactions ", cur.BlockHash.String(), cur.Height, e.Error())
			ch.BlockIndexAccess.Lock()
			cur.Parent.delChild(cur)
			delete(ch.BlockIndex, cur.BlockHash.BIdx())
			ch.BlockIndexAccess.Unlock()
		} else {
			bl.Trusted = true
			ch.Blocks.BlockAdd(cur.Height, bl)
			ch.Unspent.CommitBlockTxs(changes, bl.Hash.Hash[:])
			if !ch.DoNotSync {
				ch.Blocks.Sync()
				ch.Unspent.Sync()
			}
			ch.BlockTreeEnd = cur
			// Store as trusted block in the persistent storage
		}
	} else {
		// Store block in the persistent storage
		ch.Blocks.BlockAdd(cur.Height, bl)
		if don(DBG_BLOCKS|DBG_ORPHAS) {
			fmt.Printf("Orphan block %s @ %d\n", cur.BlockHash.String(), cur.Height)
		}
		if cur.Height > ch.BlockTreeEnd.Height {
			ch.MoveToBlock(cur)
		}
	}

	return
}


func verify(sig []byte, prv []byte, i int, tx *Tx) {
	taskDone <- VerifyTxScript(sig, prv, i, tx)
}


func getUnspIndex(po *TxPrevOut) (idx [8]byte) {
	copy(idx[:], po.Hash[:8])
	idx[0] ^= byte(po.Vout)
	idx[1] ^= byte(po.Vout>>8)
	idx[2] ^= byte(po.Vout>>16)
	idx[3] ^= byte(po.Vout>>32)
	return
}


func (ch *Chain)commitTxs(bl *Block, changes *BlockChanges) (e error) {
	sumblockin := GetBlockReward(changes.Height)
	sumblockout := uint64(0)

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

	for i := range bl.Txs {
		var txoutsum, txinsum uint64
		if don(DBG_TX) {
			fmt.Printf("tx %d/%d:\n", i+1, len(bl.Txs))
		}

		// Check each tx for a valid input
		if i>0 {
			scripts_ok := true
			for j:=0; j<useThreads; j++ {
				taskDone <- true
			}

			for j:=0; j<len(bl.Txs[i].TxIn) /*&& e==nil*/; j++ {
				inp := &bl.Txs[i].TxIn[j].Input
				if _, ok := changes.DeledTxs[*inp]; ok {
					println("txin", inp.String(), "already spent in this block")
					e = errors.New("Input spent more then once in same block")
					break
				}
				if don(DBG_TX) {
					idx := getUnspIndex(inp)
					println(" idx", hex.EncodeToString(idx[:]))
				}
				tout := ch.PickUnspent(inp)
				if tout==nil {
					if don(DBG_TX) {
						println("PickUnspent failed")
					}
					t, ok := blUnsp[inp.Hash]
					if !ok {
						e = errors.New("Unknown input")
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
					//println("TxInput from the current block", inp.String())
				} else {
					if don(DBG_TX) {
						println("PickUnspent OK")
					}
				}

				if !(<-taskDone) {
					println("VerifyScript error 1")
					scripts_ok = false
					break
				}

				if bl.Trusted {
					taskDone <- true
				} else {
					go func(i, j int, pks []byte) {
						taskDone <- VerifyTxScript(bl.Txs[i].TxIn[j].ScriptSig,
							pks, j, bl.Txs[i])
					}(i, j, tout.Pk_script)
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
				scripts_ok = <- taskDone
			}
			for j:=1; j<useThreads; j++ {
				if !(<- taskDone) {
					println("VerifyScript error 2")
					scripts_ok = false
				}
			}
			if !scripts_ok {
				return errors.New("VerifyScripts failed")
			}
		} else {
			if don(DBG_TX) {
				fmt.Printf("  mined %.8f\n", float64(sumblockin)/1e8)
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
		if i>0 && txoutsum > txinsum {
			return errors.New("More spent than at the input")
		}
		if e != nil {
			break // If any input fails, do not continue
		}
	}

	if sumblockin < sumblockout {
		return errors.New(fmt.Sprintf("Out:%d > In:%d", sumblockout, sumblockin))
	} else if don(DBG_WASTED) && sumblockin != sumblockout {
		fmt.Printf("%.8f BTC wasted in block %d\n", float64(sumblockin-sumblockout)/1e8, changes.Height)
	}

	return nil
}
