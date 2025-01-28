package textui

import (
	"fmt"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/lib/btc"
)

func get_total_block_fees(txs []*network.OneTxToSend) (totfees uint64, totwgh, tcnt int) {
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
	sta := time.Now()
	txs := network.GetSortedMempool()
	println(len(txs), "OLD tx_sort got in", time.Since(sta).String())
	network.VerifyMempoolSort(txs)

	sta = time.Now()
	cpfp := network.GetSortedMempoolRBF()
	println(len(cpfp), "NEW tx_sort got in", time.Since(sta).String())
	network.VerifyMempoolSort(cpfp)

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
	txid := btc.NewUint256FromString(par)
	if txid == nil {
		println("Specify valid txid")
		return
	}
	bidx := txid.BIdx()
	t2s := network.TransactionsToSend[bidx]
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
	network.TxMutex.Lock()

	sta := time.Now()
	tx1 := network.GetSortedMempoolSlow()
	tim1 := time.Since(sta)

	sta = time.Now()
	tx2 := network.GetSortedMempool()
	tim2 := time.Since(sta)

	sta = time.Now()
	tx3 := network.GetSortedMempoolRBF()
	tim3 := time.Since(sta)

	defer network.TxMutex.Unlock()

	println("execution times:", tim1.String(), tim2.String(), tim3.String())

	if len(tx1) != len(tx2) || len(tx1) != len(tx3) {
		println("Transaction count mismatch:", len(tx1), len(tx2), len(tx3))
		return
	}

	network.VerifyMempoolSort(tx1)
	network.VerifyMempoolSort(tx3)
	println("all good so far -", len(tx1), "txs each")
	for i := range tx1 {
		if tx1[i] != tx2[i] {
			println("tx1 / tx2 different at index", i)
			return
		}
	}
	println("both lists are identical")

	if par != "" {
		for i, t := range tx2 {
			fmt.Printf("%d5) %p idx:%d  spb:%.5f  mic:%d  %s  %p <-> %p\n",
				i, t, t.SortIndex, t.SPB(), t.MemInputCnt, btc.BIdxString(t.Hash.BIdx()), t.Better, t.Worse)
		}
	}
}

func init() {
	newUi("newblock nb", true, new_block, "Build a new block")
	newUi("txchild ch", true, gettxchildren, "show all mempool children of the given: <txid>")
	newUi("txsortest tt", true, sort_test, "Test the enw tx sprting functionality: [list]")
}
