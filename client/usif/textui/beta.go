package textui

import (
	"fmt"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/lib/btc"
	"time"
)


func get_total_block_fees(txs []*network.OneTxToSend) (totfees uint64, totwgh int) {
	for _, tx := range txs {
		wght := tx.Weight()
		if totwgh + wght > 4e6 {
			break
		}
		totfees += tx.Fee
		totwgh += wght
	}
	return
}

func new_block(par string) {
	sta := time.Now()
	txs := network.GetSortedMempool()
	println(len(txs), "_txs got in", time.Now().Sub(sta).String())

	sta = time.Now()
	cpfp := network.GetSortedMempoolNew()
	println(len(cpfp), "cpfp got in", time.Now().Sub(sta).String())

	println("All sorted.  txs:", len(txs), "  cpfp:", len(cpfp))

	var totwgh int
	var totfees, totfees2 uint64
	totfees, totwgh = get_total_block_fees(txs)
	println("Fees from OLD sorting:", btc.UintToBtc(totfees), totwgh)

	totfees2, totwgh = get_total_block_fees(cpfp)
	fmt.Printf("Fees from NEW sorting: %s %d\n", btc.UintToBtc(totfees2), totwgh)
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

func init() {
	newUi("newblock nb", true, new_block, "build a new block")
	newUi("txchild ch", true, gettxchildren, "show all the children fo the given tx")
}
