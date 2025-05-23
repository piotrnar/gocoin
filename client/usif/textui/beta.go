package textui

import (
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/txpool"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/utxo"
)

func get_total_block_fees(txs []*txpool.OneTxToSend) (totfees uint64, totwgh, tcnt int) {
	var quiet bool
	already_in := make(map[[32]byte]bool)
	for _, tx := range txs {
		wght := tx.Weight()
		if totwgh+wght > 4e6 {
			break
		}
		tcnt++
		totfees += tx.Fee
		totwgh += wght
		if quiet {
			continue
		}
		for _, txinp := range tx.TxIn {
			inp := &txinp.Input
			tout := common.BlockChain.Unspent.UnspentGet(inp)
			if tout == nil && !already_in[inp.Hash] {
				println(" *** block invalid - tx", tx.Hash.String(), "at offs", tcnt, "needs", btc.NewUint256(inp.Hash[:]).String())
				/*
					println("writing txs.txt")
					if f, _ := os.Create("txs.txt"); f != nil {
						for _, tt := range txs {
							fmt.Fprintln(f, tt.Hash.String())
						}
						f.Close()
					}
				*/
				quiet = true
			}
		}
		already_in[tx.Hash.Hash] = true
	}
	return
}

func new_block(par string) {
	txpool.TxMutex.Lock()
	defer txpool.TxMutex.Unlock()

	sta := time.Now()
	txs := txpool.GetSortedMempool()
	println(len(txs), "OLD tx_sort got in", time.Since(sta).String())
	txpool.VerifyMempoolSort(txs)

	sta = time.Now()
	cpfp := txpool.GetSortedMempoolRBF()
	println(len(cpfp), "NEW tx_sort got in", time.Since(sta).String())
	txpool.VerifyMempoolSort(cpfp)

	var totwgh, tcnt int
	var totfees, totfees2 uint64
	totfees, totwgh, tcnt = get_total_block_fees(txs)
	println("Fees from OLD sorting:", btc.UintToBtc(totfees), totwgh, tcnt)

	totfees2, totwgh, tcnt = get_total_block_fees(cpfp)
	fmt.Println("Fees from NEW sorting:", btc.UintToBtc(totfees2), totwgh, tcnt)

	if totfees2 > totfees {
		fmt.Printf("New method profit: %.3f%%\n", 100.0*float64(totfees2-totfees)/float64(totfees))
	} else {
		fmt.Printf("New method -LOSE-: %.3f%%\n", 100.0*float64(totfees-totfees2)/float64(totfees))
	}
}

func gettxchildren(par string) {
	txpool.TxMutex.Lock()
	defer txpool.TxMutex.Unlock()

	txid := btc.NewUint256FromString(par)
	if txid == nil {
		println("Specify valid txid")
		return
	}
	bidx := txid.BIdx()
	t2s := txpool.TransactionsToSend[bidx]
	if t2s == nil {
		println(txid.String(), "not im mempool")
		return
	}
	chlds := t2s.GetAllChildren()
	println("has", len(chlds), "all children")
	var tot_wg, tot_fee uint64
	for _, tx := range chlds {
		println(" -", tx.Hash.String(), len(tx.GetChildren()), tx.SPB(), "@", tx.Weight())
		tot_wg += uint64(tx.Weight())
		tot_fee += tx.Fee
		//gettxchildren(tx.Hash.String())
	}
	println("Groups SPB:", float64(tot_fee)/float64(tot_wg)*4.0)
}

func sort_test(par string) {
	txpool.TxMutex.Lock()
	defer txpool.TxMutex.Unlock()

	println("Doing mempool check first")
	if txpool.MempoolCheck() {
		println("Mempool check failed. Aborting.")
		return
	}
	println("Mempool looks OK")

	sta := time.Now()
	tx1 := txpool.GetSortedMempoolSlow()
	tim1 := time.Since(sta)

	sta = time.Now()
	tx2 := txpool.GetSortedMempool()
	tim2 := time.Since(sta)

	sta = time.Now()
	tx3 := txpool.GetSortedMempoolRBF()
	tim3 := time.Since(sta)

	println("Three sorted txs lists acquired.")
	println("Execution times  1-Slow:", tim1.String(), "  2-Fast:", tim2.String(), "  3-RBF:", tim3.String())

	if len(tx1) != len(tx2) || len(tx1) != len(tx3) {
		println("Transaction count mismatch:", len(tx1), len(tx2), len(tx3))
		return
	}
	println("All lists have", len(tx1), "txs each")

	v1 := txpool.VerifyMempoolSort(tx1)
	v2 := txpool.VerifyMempoolSort(tx1)
	v3 := txpool.VerifyMempoolSort(tx1)
	if v1 || v2 || v3 {
		println("1st list verify error:", v1)
		println("2nd list verify error:", v2)
		println("3rd list verify error:", v3)
		return
	} else {
		println("All lists verified OK")
	}

	for i := range tx1 {
		if tx1[i] != tx2[i] {
			println("The first two lists become different at index", i, "(but that is normal)")
			return
		}
	}
	println("The first two lists are identical (confirms sort index OK)")
}

func show_tdepends(s string) {
	d, er := hex.DecodeString(s)
	if er != nil || len(d) != utxo.UtxoIdxLen {
		println("Specify BIDX encoded as", 2*utxo.UtxoIdxLen, "hex digits")
		return
	}
	var bidx btc.BIDX
	for i := range bidx[:] {
		bidx[i] = d[7-i]
	}
	println("Looking for tx at BIDX", btc.BIdxString(bidx))

	txpool.TxMutex.Lock()
	defer txpool.TxMutex.Unlock()
	if t2s, ok := txpool.TransactionsToSend[bidx]; ok {
		println("TxID:", t2s.Hash.String(), "   MemInCnt:", t2s.MemInputCnt)
		for i, yes := range t2s.MemInputs {
			if yes {
				//uidx := t2s.TxIn[i].Input.UIdx()
				bbi := btc.BIdx(t2s.TxIn[i].Input.Hash[:])
				fmt.Printf(" * input %d/%d  %s\n", i+1, len(t2s.TxIn), btc.BIdxString(bbi))
			}
		}
	} else {
		println("tx not found in mempool")
	}
}

func DumpTxList(label, fn string, txs []*txpool.OneTxToSend) {
	if f, er := os.Create(fn); er == nil {
		fmt.Fprintln(f, label+" sorting:")
		for i, t := range txs {
			fmt.Fprintf(f, "%6d)  ptr:%p  spb:%.4f  memins:%d  bidx:%s  idx:%d\n", i+1, t,
				t.SPB(), t.MemInputCnt, btc.BIdxString(t.Hash.BIdx()), t.SortRank)
			for i, yes := range t.MemInputs {
				if yes {
					bbi := btc.BIdx(t.TxIn[i].Input.Hash[:])
					fmt.Fprintf(f, "       input %d/%d  depends on  %s\n", i+1, len(t.TxIn), btc.BIdxString(bbi))
				}
			}
		}
		f.Close()
		println(fn, "saved")
	} else {
		println(er.Error())
	}
}

func init() {
	newUi("newblock nb", false, new_block, "Build a new block")
	newUi("txchild ch", false, gettxchildren, "show all mempool children of the given: <txid>")
	newUi("txsortest tt", false, sort_test, "Test the new tx sorting functionality")
	newUi("txdepends tdep", false, show_tdepends, "Show txt dependant on this one")
}
