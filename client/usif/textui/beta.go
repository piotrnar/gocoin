package textui

import (
	//"fmt"
	"sort"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/client/network"
)


type OneTxsPackage struct {
	Parent *network.OneTxToSend
	Children []*network.OneTxToSend
	Weight int
	Fee uint64
}

func (pkg *OneTxsPackage) Ver() bool {
	mned := make(map[network.BIDX]bool, 1 + len(pkg.Children))
	mned[pkg.Parent.Hash.BIdx()] = true

	//println("testing", pkg.Parent.Hash.String())
	for _, tx := range pkg.Children {
		//println("Child", ii, tx.Hash.String(), "meminps:", tx.MemInputCnt, "...")
		if tx.MemInputs == nil || tx.MemInputCnt==0 {
			return false
		}

		var cnt int
		for i := range tx.MemInputs {
			bidx := btc.BIdx(tx.TxIn[i].Input.Hash[:])
			if tx.MemInputs[i] {
				//println("... input", i, tx.TxIn[i].Input.String(), "-is mem:", mned[bidx], "cnt")
				if ok, _ := mned[bidx]; !ok {
					//println("error")
					return false
				}
				cnt++
				if cnt==tx.MemInputCnt {
					return true
				}
			}
			mned[bidx] = true
		}
	}
	return true
}


func LookForPackages(txs []*network.OneTxToSend) (result []*OneTxsPackage) {
	for _, tx := range txs {
		var pkg OneTxsPackage
		pkg.Children = tx.GetAllChildren()
		if len(pkg.Children) > 0 {
			pkg.Parent = tx
			pkg.Weight = tx.Weight()
			pkg.Fee = tx.Fee
			for _, t := range pkg.Children {
				pkg.Weight += t.Weight()
				pkg.Fee += t.Fee
				result = append(result, &pkg)
			}
		}
	}
	sort.Slice(txs, func(i, j int) bool {
		return txs[i].Fee * uint64(txs[j].Weight()) > txs[j].Fee * uint64(txs[i].Weight())
	})
	return
}

/*

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
	/*
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
	*/

	pkgs := LookForPackages(txs)
	println(len(pkgs), "packages found")
	for i, pkg := range pkgs {
		if !pkg.Ver() {
			println(i, ") FAAILED", pkg.Parent.Hash.String(), "with", len(pkg.Children), "children")
			for _, ch := range pkg.Children {
				println("   ", ch.Hash.String())
			}
			break
		}
		println(i, ")", len(pkg.Children), "spkb:", 1000 * uint64(pkg.Fee) / uint64(pkg.Weight))
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
	println("Groups SPB:", float64(tot_fee) / float64(tot_wg) * 4.0)
}

func init () {
	newUi("newblock nb", true, new_block, "build a new block")
	newUi("txchild ch", true, gettxchildren, "show all the children fo the given tx")
}
