package textui

import (
	//"fmt"
	//"sort"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/client/network"
)


/*
func CalculateDescendants(entryit network.BIDX, setDescendants map[network.BIDX]bool) {
	//println("CalculateDescendants of", network.TransactionsToSend[entryit].Hash.String(), "...", len(setDescendants))
	var stage []network.BIDX
	if _, ok := setDescendants[entryit]; !ok {
		stage = []network.BIDX{entryit}
	}
	for len(stage) > 0 {
		it := stage[len(stage)-1]
		setDescendants[it] = true
		stage = stage[:len(stage)-1]

		setChildren := GetMemPoolChildren(it)
		for childiter, _ := range setChildren {
			if _, ok := setDescendants[childiter]; !ok {
				stage = append(stage, childiter)
			}
		}
	}
	//println("... ->", len(setDescendants))
}

func UpdatePackagesForAdded(alreadyAdded map[network.BIDX]bool, mapModifiedTx map[network.BIDX]bool) (childs_updated int) {
	for it, _ := range alreadyAdded {
		descendants := make(map[network.BIDX]bool)
		CalculateDescendants(it, descendants)
		// Insert all descendants (not yet in block) into the modified set
		for desc, _ := range descendants {
			if _, ok := alreadyAdded[desc]; ok {
				continue
			}
			childs_updated++
			if _, ok := mapModifiedTx[desc]; !ok {
				println("modEntry-insert")
                mapModifiedTx[desc] = true
			} else {
				println("modEntry-modify")
			}
		}
	}
    return
}
*/

func new_block(par string) {
	txs := network.GetSortedMempool()
	var most_children *network.OneTxToSend
	var most_children_cnt int
	for _, tx := range txs {
		tmp := tx.GetAllChildren()
		if len(tmp) > most_children_cnt {
			most_children_cnt = len(tmp)
			most_children = tx
		}
	}
	if most_children != nil {
		println("txid", most_children.Hash.String(), "has", most_children_cnt, "children")
		gettxchildren(most_children.Hash.String())
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
	if t2s==nil {
		println(txid.String(), "not im mempool")
	}
	chlds := t2s.GetAllChildren()
	println("has", len(chlds), "children")
	var tot_wg, tot_fee uint64
	for _, tx := range chlds {
		println(" -", tx.Hash.String(), len(tx.GetChildren()), tx.SPB(), "@", tx.Weight())
		tot_wg += uint64(tx.Weight())
		tot_fee += tx.Fee
		//gettxchildren(tx.Hash.String())
	}
	println("Groups SPB:", float64(tot_fee) / float64(tot_wg) * 4.0)
}

func init () {
	newUi("newblock nb", true, new_block, "build a new block")
	newUi("txchild ch", true, gettxchildren, "show all the children fo the given tx")
}
