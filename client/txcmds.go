package main

import (
	"os"
	"fmt"
	"time"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
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

	txd, er := hex.DecodeString(string(buf))
	if er != nil {
		txd = buf
		fmt.Println("Seems like the transaction is in a binary format")
	} else {
		fmt.Println("Looks like the transaction file contains hex data")
	}

	// At this place we should have raw transaction in txd
	tx, le := btc.NewTx(txd)
	if le != len(txd) {
		fmt.Println("WARNING: Tx length mismatch", le, len(txd))
	}
	tx.Hash = btc.NewSha2Hash(txd)
	fmt.Println("Transaction details (for your information):")
	fmt.Println(len(tx.TxIn), "Input(s):")
	var totinp, totout uint64
	var missinginp bool
	for i := range tx.TxIn {
		fmt.Printf(" %3d %s", i, tx.TxIn[i].Input.String())
		po, _ := BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
		if po != nil {
			ok := btc.VerifyTxScript(tx.TxIn[i].ScriptSig, po.Pk_script, i, tx)
			if !ok {
				fmt.Println("\nERROR: The transacion does not have a valid signature.")
				return
			}
			totinp += po.Value
			fmt.Printf(" %15.8f BTC @ %s\n", float64(po.Value)/1e8,
				btc.NewAddrFromPkScript(po.Pk_script, AddrVersion).String())
		} else {
			fmt.Println(" - UNKNOWN INPUT")
			missinginp = true
		}
	}
	fmt.Println(len(tx.TxOut), "Output(s):")
	for i := range tx.TxOut {
		totout += tx.TxOut[i].Value
		fmt.Printf(" %15.8f BTC to %s\n", float64(tx.TxOut[i].Value)/1e8,
			btc.NewAddrFromPkScript(tx.TxOut[i].Pk_script, AddrVersion).String())
	}
	if missinginp {
		fmt.Println("WARNING: There are missing inputs and we cannot calc input BTC amount.")
		fmt.Println("If there is somethign wrong with this transaction, you can loose money...")
	} else {
		fmt.Printf("All OK: %.8f BTC in -> %.8f BTC out, with %.8f BTC fee\n", float64(totinp)/1e8,
			float64(totout)/1e8, float64(totinp-totout)/1e8)
	}
	tx_mutex.Lock()
	TransactionsToSend[tx.Hash.Hash] = &OneTxToSend{data:txd, own:true}
	tx_mutex.Unlock()
	fmt.Println("Transaction added to the memory pool. Please double check its details above.")
	fmt.Println("If it does what you intended, execute: stx " + tx.Hash.String())
}


func send_tx(par string) {
	txid := btc.NewUint256FromString(par)
	if txid==nil {
		fmt.Println("You must specify a valid transaction ID for this command.")
		list_txs("")
		return
	}
	tx_mutex.Lock()
	if ptx, ok := TransactionsToSend[txid.Hash]; ok {
		tx_mutex.Unlock()
		cnt := NetRouteInv(1, txid, nil)
		ptx.sentCount += cnt
		ptx.lastTime = time.Now()
		fmt.Println("INV for TxID", txid.String(), "sent to", cnt, "node(s)")
		fmt.Println("If it does not appear in the chain, you may want to redo it.")
	} else {
		tx_mutex.Unlock()
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
	tx_mutex.Lock()
	if _, ok := TransactionsToSend[txid.Hash]; !ok {
		tx_mutex.Unlock()
		fmt.Println("No such transaction ID in the memory pool.")
		list_txs("")
		return
	}
	delete(TransactionsToSend, txid.Hash)
	tx_mutex.Unlock()
	fmt.Println("Transaction", txid.String(), "removed from the memory pool")
}


func list_txs(par string) {
	fmt.Println("Transactions in the memory pool:")
	cnt := 0
	tx_mutex.Lock()
	for k, v := range TransactionsToSend {
		cnt++
		var oe, snt string
		if v.own {
			oe = "OWN"
		} else {
			oe = "ext"
		}

		if v.sentCount==0 {
			snt = "never sent"
		} else {
			snt = fmt.Sprintf("sent %d times, last %s ago", v.sentCount,
				time.Now().Sub(v.lastTime).String())
		}
		fmt.Printf("%5d) %s: %s - %d bytes - %s\n", cnt, oe,
			btc.NewUint256(k[:]).String(), len(v.data), snt)
	}
	tx_mutex.Unlock()
}


func baned_txs(par string) {
	fmt.Println("Rejected transactions:")
	cnt := 0
	tx_mutex.Lock()
	for k, v := range TransactionsRejected {
		cnt++
		fmt.Println("", cnt, btc.NewUint256(k[:]).String(), "-", time.Now().Sub(v).String(), "ago")
	}
	tx_mutex.Unlock()
}


func send_all_tx(par string) {
	tx_mutex.Lock()
	for k, v := range TransactionsToSend {
		if v.own {
			cnt := NetRouteInv(1, btc.NewUint256(k[:]), nil)
			v.sentCount += cnt
			v.lastTime = time.Now()
			fmt.Println("INV for TxID", btc.NewUint256(k[:]).String(), "sent to", cnt, "node(s)")
		}
	}
	tx_mutex.Unlock()
}

func init () {
	newUi("txload tx", true, load_tx, "Load transaction data from the given file, decode it and store in memory")
	newUi("txsend stx", true, send_tx, "Broadcast transaction from memory pool (identified by a given <txid>)")
	newUi("txsendall stxa", true, send_all_tx, "Broadcast all the transactions (what you see after ltx)")
	newUi("txdel dtx", true, del_tx, "Temove a transaction from memory pool (identified by a given <txid>)")
	newUi("txlist ltx", true, list_txs, "List all the transaction loaded into memory pool")
	newUi("txlistban ltxb", true, baned_txs, "List the transaction that we have rejected")
}