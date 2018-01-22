package textui

import (
	"fmt"
	"sort"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/client/network"
	"time"
)


type OneTxsPackage struct {
	Txs []*network.OneTxToSend
	Weight int
	Fee uint64
}

func (pk *OneTxsPackage) SPW() float64 {
	return float64(pk.Fee) / float64(pk.Weight)
}

func (pk *OneTxsPackage) SPB() float64 {
	return pk.SPW() * 4.0
}

func (pk *OneTxsPackage) AnyIn(list map[*network.OneTxToSend]bool) (ok bool) {
	for _, par := range pk.Txs {
		if _, ok = list[par]; ok {
			return
		}
	}
	return
}

func (pkg *OneTxsPackage) Ver() bool {
	mned := make(map[network.BIDX]bool, len(pkg.Txs))
	for idx, tx := range append(pkg.Txs) {
		if tx.MemInputCnt > 0 {
			var cnt int
			for i := range tx.MemInputs {
				if tx.MemInputs[i] {
					if ok, _ := mned[btc.BIdx(tx.TxIn[i].Input.Hash[:])]; !ok {
						return false
					}
					cnt++
					if cnt==tx.MemInputCnt {
						break
					}
				}
			}
		}
		if idx == len(pkg.Txs)-1 {
			break
		}
		mned[tx.Hash.BIdx()] = true
	}
	return true
}


func LookForPackages(txs []*network.OneTxToSend) (result []*OneTxsPackage) {
	for _, tx := range txs {
		var pkg OneTxsPackage
		parents := tx.GetAllParents()
		if len(parents) > 0 {
			pkg.Txs = append(parents, tx)
			for _, t := range pkg.Txs {
				pkg.Weight += t.Weight()
				pkg.Fee += t.Fee
			}
			result = append(result, &pkg)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].Fee * uint64(result[j].Weight) > result[j].Fee * uint64(result[i].Weight)
	})
	return
}

func new_block(par string) {
	sta := time.Now()
	txs := network.GetSortedMempool()
	println(len(txs), "txs got in", time.Now().Sub(sta).String())

	sta = time.Now()
	pkgs := LookForPackages(txs)
	println(len(pkgs), "packages found in", time.Now().Sub(sta).String())
	for _, pkg := range pkgs {
		//println("=============================")
		//println(i, ")", pkg.Child.Hash.String(), len(pkg.Txs), "spkb:", 1000 * uint64(pkg.Fee) / uint64(pkg.Weight))
		/*for _, ch := range pkg.Parents {
			println("   ", ch.Hash.String())
		}*/
		if !pkg.Ver() {
			break
		}
	}

	//return
	result := make([]*network.OneTxToSend, len(txs))
	var txs_idx, pks_idx, res_idx int
	already_in := make(map[*network.OneTxToSend]bool, len(txs))
	for txs_idx < len(txs) {
		tx := txs[txs_idx]

		if pks_idx < len(pkgs) {
			pk := pkgs[pks_idx]
			if pk.SPW() > tx.SPW() {
				pks_idx++
				if pk.AnyIn(already_in) {
					continue
				}
				// all package's txs new: incude them all
				copy(result[res_idx:], pk.Txs)
				res_idx += len(pk.Txs)
				for _, _t := range pk.Txs {
					already_in[_t] = true
				}
				continue
			}
		}

		txs_idx++
		if _, ok := already_in[tx]; ok {
			continue
		}
		result[res_idx] = tx
		already_in[tx] = true
		res_idx++
	}
	println("All sorted.  res_idx:", res_idx, "  txs:", len(txs))

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
	for _, tx := range result {
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
		fmt.Printf("New method lose: %.3f%%\n", 100.0*float64(totfees-totfees2)/float64(totfees))
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
