package textui

import (
	"os"
	"fmt"
	"time"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/client/usif"
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
	if _, ok := network.TransactionsToSend[txid.BIdx()]; !ok {
		network.TxMutex.Unlock()
		fmt.Println("No such transaction ID in the memory pool.")
		list_txs("")
		return
	}
	delete(network.TransactionsToSend, txid.BIdx())
	network.TxMutex.Unlock()
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


func list_txs(par string) {
	fmt.Println("Transactions in the memory pool:")
	cnt := 0
	network.TxMutex.Lock()
	for _, v := range network.TransactionsToSend {
		cnt++
		var oe, snt string
		if v.Own!=0 {
			oe = " *OWN*"
		} else {
			oe = ""
		}

		snt = fmt.Sprintf("INV sent %d times,   ", v.Invsentcnt)

		if v.SentCnt==0 {
			snt = "TX never sent"
		} else {
			snt = fmt.Sprintf("TX sent %d times, last %s ago", v.SentCnt,
				time.Now().Sub(v.Lastsent).String())
		}
		fmt.Printf("%5d) %s - %d bytes - %s%s\n", cnt, v.Tx.Hash.String(), len(v.Data), snt, oe)
	}
	network.TxMutex.Unlock()
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

func init () {
	newUi("txload tx", true, load_tx, "Load transaction data from the given file, decode it and store in memory")
	newUi("txsend stx", true, send_tx, "Broadcast transaction from memory pool (identified by a given <txid>)")
	newUi("tx1send stx1", true, send1_tx, "Broadcast transaction to a single random peer (identified by a given <txid>)")
	newUi("txsendall stxa", true, send_all_tx, "Broadcast all the transactions (what you see after ltx)")
	newUi("txdel dtx", true, del_tx, "Remove a transaction from memory pool (identified by a given <txid>)")
	newUi("txdecode td", true, dec_tx, "Decode a transaction from memory pool (identified by a given <txid>)")
	newUi("txlist ltx", true, list_txs, "List all the transaction loaded into memory pool")
	newUi("txlistban ltxb", true, baned_txs, "List the transaction that we have rejected")
}
