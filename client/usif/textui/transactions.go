package textui

import (
	"os"
	"fmt"
	"time"
	"sort"
	"strconv"
	"io/ioutil"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/client/usif"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
)


func load_tx(par string) {
	if par=="" {
		fmt.Println("Specify a name of a transaction file")
		return
	}
	f, e := os.Open(par)
	if e != nil {
		println(e.Error())
		return
	}
	n, _ := f.Seek(0, os.SEEK_END)
	f.Seek(0, os.SEEK_SET)
	buf := make([]byte, n)
	f.Read(buf)
	f.Close()
	fmt.Println(usif.LoadRawTx(buf))
}


func send_tx(par string) {
	txid := btc.NewUint256FromString(par)
	if txid==nil {
		fmt.Println("You must specify a valid transaction ID for this command.")
		list_txs("")
		return
	}
	network.TxMutex.Lock()
	if ptx, ok := network.TransactionsToSend[txid.BIdx()]; ok {
		network.TxMutex.Unlock()
		cnt := network.NetRouteInv(1, txid, nil)
		ptx.Invsentcnt += cnt
		fmt.Println("INV for TxID", txid.String(), "sent to", cnt, "node(s)")
		fmt.Println("If it does not appear in the chain, you may want to redo it.")
	} else {
		network.TxMutex.Unlock()
		fmt.Println("No such transaction ID in the memory pool.")
		list_txs("")
	}
}


func send1_tx(par string) {
	txid := btc.NewUint256FromString(par)
	if txid==nil {
		fmt.Println("You must specify a valid transaction ID for this command.")
		list_txs("")
		return
	}
	network.TxMutex.Lock()
	if ptx, ok := network.TransactionsToSend[txid.BIdx()]; ok {
		network.TxMutex.Unlock()
		usif.SendInvToRandomPeer(1, txid)
		ptx.Invsentcnt++
		fmt.Println("INV for TxID", txid.String(), "sent to a random node")
		fmt.Println("If it does not appear in the chain, you may want to redo it.")
	} else {
		network.TxMutex.Unlock()
		fmt.Println("No such transaction ID in the memory pool.")
		list_txs("")
	}
}


func del_tx(par string) {
	txid := btc.NewUint256FromString(par)
	if txid==nil {
		fmt.Println("You must specify a valid transaction ID for this command.")
		list_txs("")
		return
	}
	network.TxMutex.Lock()
	defer network.TxMutex.Unlock()
	tx, ok := network.TransactionsToSend[txid.BIdx()]
	if !ok {
		network.TxMutex.Unlock()
		fmt.Println("No such transaction ID in the memory pool.")
		list_txs("")
		return
	}
	network.DeleteToSend(tx)
	fmt.Println("Transaction", txid.String(), "removed from the memory pool")
}


func dec_tx(par string) {
	txid := btc.NewUint256FromString(par)
	if txid==nil {
		fmt.Println("You must specify a valid transaction ID for this command.")
		list_txs("")
		return
	}
	if tx, ok := network.TransactionsToSend[txid.BIdx()]; ok {
		s, _, _, _, _ := usif.DecodeTx(tx.Tx)
		fmt.Println(s)
	} else {
		fmt.Println("No such transaction ID in the memory pool.")
	}
}


func save_tx(par string) {
	txid := btc.NewUint256FromString(par)
	if txid==nil {
		fmt.Println("You must specify a valid transaction ID for this command.")
		list_txs("")
		return
	}
	if tx, ok := network.TransactionsToSend[txid.BIdx()]; ok {
		fn := tx.Hash.String() + ".tx"
		ioutil.WriteFile(fn, tx.Data, 0600)
		fmt.Println("Saved to", fn)
	} else {
		fmt.Println("No such transaction ID in the memory pool.")
	}
}


func mempool_stats(par string) {
	fmt.Print(usif.MemoryPoolFees())
}

func list_txs(par string) {
	limitbytes, _ := strconv.ParseUint(par, 10, 64)
	fmt.Println("Transactions in the memory pool:", limitbytes)
	cnt := 0
	network.TxMutex.Lock()
	defer network.TxMutex.Unlock()

	sorted := make(network.SortedTxToSend, len(network.TransactionsToSend))
	for _, v := range network.TransactionsToSend {
		sorted[cnt] = v
		cnt++
	}
	sort.Sort(sorted)

	var totlen uint64
	for cnt=0; cnt<len(sorted); cnt++ {
		v := sorted[cnt]
		totlen += uint64(len(v.Data))

		if limitbytes!=0 && totlen>limitbytes {
			break
		}

		var oe, snt string
		if v.Own!=0 {
			oe = " *OWN*"
		} else {
			oe = ""
		}

		snt = fmt.Sprintf("INV sent %d times,   ", v.Invsentcnt)

		if v.SentCnt==0 {
			snt = "never sent"
		} else {
			snt = fmt.Sprintf("sent %d times, last %s ago", v.SentCnt,
				time.Now().Sub(v.Lastsent).String())
		}

		spb := float64(v.Fee)/float64(len(v.Data))

		fmt.Println(fmt.Sprintf("%5d) ...%10d %s  %6d bytes / %6.1fspb - %s%s", cnt, totlen, v.Tx.Hash.String(), len(v.Data), spb, snt, oe))

	}
}


func baned_txs(par string) {
	fmt.Println("Rejected transactions:")
	cnt := 0
	network.TxMutex.Lock()
	for k, v := range network.TransactionsRejected {
		cnt++
		fmt.Println("", cnt, btc.NewUint256(k[:]).String(), "-", v.Size, "bytes",
			"-", v.Reason, "-", time.Now().Sub(v.Time).String(), "ago")
	}
	network.TxMutex.Unlock()
}


func send_all_tx(par string) {
	network.TxMutex.Lock()
	for k, v := range network.TransactionsToSend {
		if v.Own!=0 {
			cnt := network.NetRouteInv(1, btc.NewUint256(k[:]), nil)
			v.Invsentcnt += cnt
			fmt.Println("INV for TxID", v.Hash.String(), "sent to", cnt, "node(s)")
		}
	}
	network.TxMutex.Unlock()
}

func clean_txs(par string) {
	var any_done bool
	var total_done int
	sta := time.Now()
	network.TxMutex.Lock()
	for {
		any_done = false
		for _, tx := range network.TransactionsToSend {
			var remove_it bool
			for i := range tx.TxIn {
				var po *btc.TxOut
				inpid := btc.NewUint256(tx.TxIn[i].Input.Hash[:])
				if txinmem, ok := network.TransactionsToSend[inpid.BIdx()]; ok {
					if int(tx.TxIn[i].Input.Vout) < len(txinmem.TxOut) {
						po = txinmem.TxOut[tx.TxIn[i].Input.Vout]
					}
				} else {
					po, _ = common.BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
				}
				if po==nil {
					remove_it = true
					break
				}
			}

			if remove_it {
				network.DeleteToSend(tx)
				any_done = true
				total_done++
			}
		}
		if !any_done {
			break
		}
	}
	network.TxMutex.Unlock()
	fmt.Println("Removed", total_done, "txs with unknown inputs in", time.Now().Sub(sta).String())
}

func init () {
	newUi("txload tx", true, load_tx, "Load transaction data from the given file, decode it and store in memory")
	newUi("txsend stx", true, send_tx, "Broadcast transaction from memory pool (identified by a given <txid>)")
	newUi("tx1send stx1", true, send1_tx, "Broadcast transaction to a single random peer (identified by a given <txid>)")
	newUi("txsendall stxa", true, send_all_tx, "Broadcast all the transactions (what you see after ltx)")
	newUi("txdel dtx", true, del_tx, "Remove a transaction from memory pool (identified by a given <txid>)")
	newUi("txdecode td", true, dec_tx, "Decode a transaction from memory pool (identified by a given <txid>)")
	newUi("txlist ltx", true, list_txs, "List all the transaction loaded into memory pool up to 1MB space <max_size>")
	newUi("txlistban ltxb", true, baned_txs, "List the transaction that we have rejected")
	newUi("mempool mp", true, mempool_stats, "Show the mempool statistics")
	newUi("txsave", true, save_tx, "Save raw transaction from memory pool to disk")
	newUi("txclean", true, clean_txs, "Remove txs with unknown inputs from the mempool")
}
