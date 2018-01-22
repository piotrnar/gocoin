package textui

import (
	"fmt"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/lib/btc"
	"time"
)

func new_block(par string) {
	sta := time.Now()
	txs := network.GetSortedMempool()
	println(len(txs), "txs got in", time.Now().Sub(sta).String())

	sta = time.Now()
	rbf := network.GetSortedMempoolNew()
	println(len(rbf), "rbf got in", time.Now().Sub(sta).String())

	println("All sorted.  txs:", len(txs), "  rbf:", len(rbf))

	var totwgh int
	var totfees, totfees2 uint64
	for _, tx := range txs {
		totfees += tx.Fee
		totwgh += tx.Weight()
		if totwgh > 4e6 {
			totwgh -= tx.Weight()
			break
		}
	}
	println("Fees from OLD sorting:", btc.UintToBtc(totfees), totwgh)

	totwgh = 0
	for _, tx := range rbf {
		totfees2 += tx.Fee
		totwgh += tx.Weight()
		if totwgh > 4e6 {
			totwgh -= tx.Weight()
			break
		}
	}
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
