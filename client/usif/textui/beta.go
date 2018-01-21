package textui

import (
	//"fmt"
	//"sort"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/client/network"
)


func GetMemPoolChildren(it network.BIDX) (res map[network.BIDX]bool) {
	tx := network.TransactionsToSend[it]
	if tx==nil {
		panic("tx not found in TransactionsToSend")
	}

	var po btc.TxPrevOut
	po.Hash = tx.Hash.Hash

	res = make(map[network.BIDX]bool)

	for po.Vout = 0; po.Vout < uint32(len(tx.TxOut)); po.Vout++ {
		uidx := po.UIdx()
		if val, ok := network.SpentOutputs[uidx]; ok {
			// TODO: remove the check
			if _, ok := network.TransactionsToSend[val]; !ok {
				panic("SpentOutput not in mempool")
			}
			res[val] = true
		}
	}
	return
}

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
/*
                CTxMemPoolModifiedEntry modEntry(desc);
                modEntry.nSizeWithAncestors -= it->GetTxSize();
                modEntry.nModFeesWithAncestors -= it->GetModifiedFee();
                modEntry.nSigOpCostWithAncestors -= it->GetSigOpCost();
*/
                mapModifiedTx[desc] = true
			} else {
				println("modEntry-modify")
			}
		}
	}
    return
}
/*
    for (const CTxMemPool::txiter it : alreadyAdded) {
        CTxMemPool::setEntries descendants;
        mempool.CalculateDescendants(it, descendants);
        // Insert all descendants (not yet in block) into the modified set
        for (CTxMemPool::txiter desc : descendants) {
            if (alreadyAdded.count(desc))
                continue;
            ++childs_updated;
            modtxiter mit = mapModifiedTx.find(desc);
            if (mit == mapModifiedTx.end()) {
                CTxMemPoolModifiedEntry modEntry(desc);
                modEntry.nSizeWithAncestors -= it->GetTxSize();
                modEntry.nModFeesWithAncestors -= it->GetModifiedFee();
                modEntry.nSigOpCostWithAncestors -= it->GetSigOpCost();
                mapModifiedTx.insert(modEntry);
            } else {
                mapModifiedTx.modify(mit, update_for_parent_inclusion(it));
            }
        }
    }
*/


func new_block(par string) {
	network.TxMutex.Lock()
	defer network.TxMutex.Unlock()

	inBlock := network.GetSortedMempool()
	rec_0 := network.TransactionsToSend[inBlock[0]]
	rec_l := network.TransactionsToSend[inBlock[len(inBlock)-1]]
	println("sorted", len(inBlock), ":", rec_0.MemInputCnt, "...", rec_l.MemInputCnt)
	println(" or:", 1000*rec_0.Fee/uint64(rec_0.Weight()/4), "...", 1000*rec_l.Fee/uint64(rec_l.Weight()/4))
	println(" or:", rec_0.Hash.String(), "...", rec_l.Hash.String())

	mapModifiedTx := make(map[network.BIDX]bool)

	alreadyAdded := make(map[network.BIDX]bool, len(inBlock))
	for _, v := range inBlock {
		alreadyAdded[v] = true
	}
	UpdatePackagesForAdded(alreadyAdded, mapModifiedTx)
	//CalculateDescendants(inBlock[0], mapModifiedTx)
	println("mapModifiedTx", len(mapModifiedTx))
}

func init () {
	newUi("newblock nb", true, new_block, "build a new block")
}
