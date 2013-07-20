package main

import (
	"os"
	"fmt"
	"time"
	"errors"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
)


func tx2str(tx *btc.Tx) (s string, missinginp bool, totinp, totout uint64, e error) {
	s += fmt.Sprintln("Transaction details (for your information):")
	s += fmt.Sprintln(len(tx.TxIn), "Input(s):")
	for i := range tx.TxIn {
		s += fmt.Sprintf(" %3d %s", i, tx.TxIn[i].Input.String())
		var po *btc.TxOut

		if txinmem, ok := TransactionsToSend[tx.TxIn[i].Input.Hash]; ok {
			s += fmt.Sprint(" mempool")
			if int(tx.TxIn[i].Input.Vout) >= len(txinmem.TxOut) {
				s += fmt.Sprintf(" - Vout TOO BIG (%d/%d)!", int(tx.TxIn[i].Input.Vout), len(txinmem.TxOut))
			} else {
				po = txinmem.TxOut[tx.TxIn[i].Input.Vout]
			}
		} else {
			po, _ = BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
			if po != nil {
				s += fmt.Sprintf("%8d", po.BlockHeight)
			}
		}
		if po != nil {
			ok := btc.VerifyTxScript(tx.TxIn[i].ScriptSig, po.Pk_script, i, tx)
			if !ok {
				s += fmt.Sprintln("\nERROR: The transacion does not have a valid signature.")
				e = errors.New("Invalid signature")
				return
			}
			totinp += po.Value
			s += fmt.Sprintf(" %15.8f BTC @ %s\n", float64(po.Value)/1e8,
				btc.NewAddrFromPkScript(po.Pk_script, AddrVersion).String())
		} else {
			s += fmt.Sprintln(" - UNKNOWN INPUT")
			missinginp = true
		}
	}
	s += fmt.Sprintln(len(tx.TxOut), "Output(s):")
	for i := range tx.TxOut {
		totout += tx.TxOut[i].Value
		s += fmt.Sprintf(" %15.8f BTC to %s\n", float64(tx.TxOut[i].Value)/1e8,
			btc.NewAddrFromPkScript(tx.TxOut[i].Pk_script, AddrVersion).String())
	}
	if missinginp {
		s += fmt.Sprintln("WARNING: There are missing inputs and we cannot calc input BTC amount.")
		s += fmt.Sprintln("If there is somethign wrong with this transaction, you can loose money...")
	} else {
		s += fmt.Sprintf("All OK: %.8f BTC in -> %.8f BTC out, with %.8f BTC fee\n", float64(totinp)/1e8,
			float64(totout)/1e8, float64(totinp-totout)/1e8)
	}
	return
}


func load_raw_tx(buf []byte) (s string) {
	txd, er := hex.DecodeString(string(buf))
	if er != nil {
		txd = buf
	}

	// At this place we should have raw transaction in txd
	tx, le := btc.NewTx(txd)
	if tx==nil || le != len(txd) {
		s += fmt.Sprintln("Could not decode transaction file or it has some extra data")
		return
	}
	tx.Hash = btc.NewSha2Hash(txd)

	var missinginp bool
	var totinp, totout uint64
	s, missinginp, totinp, totout, er = tx2str(tx)
	if er != nil {
		return
	}

	tx_mutex.Lock()
	if missinginp {
		TransactionsToSend[tx.Hash.Hash] = &OneTxToSend{Tx:tx, data:txd, own:2, firstseen:time.Now(),
			volume:totout}
	} else {
		TransactionsToSend[tx.Hash.Hash] = &OneTxToSend{Tx:tx, data:txd, own:1, firstseen:time.Now(),
			volume:totinp, fee:totinp-totout}
	}
	tx_mutex.Unlock()
	s += fmt.Sprintln("Transaction added to the memory pool. Please double check its details above.")
	s += fmt.Sprintln("If it does what you intended, you can send it the network.\nUse TxID:", tx.Hash.String())
	return
}


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
	fmt.Println(load_raw_tx(buf))
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
		ptx.sentcnt += cnt
		ptx.lastsent = time.Now()
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


func dec_tx(par string) {
	txid := btc.NewUint256FromString(par)
	if txid==nil {
		fmt.Println("You must specify a valid transaction ID for this command.")
		list_txs("")
		return
	}
	if tx, ok := TransactionsToSend[txid.Hash]; ok {
		s, _, _, _, _ := tx2str(tx.Tx)
		fmt.Println(s)
	} else {
		fmt.Println("No such transaction ID in the memory pool.")
	}
}


func list_txs(par string) {
	fmt.Println("Transactions in the memory pool:")
	cnt := 0
	tx_mutex.Lock()
	for k, v := range TransactionsToSend {
		cnt++
		var oe, snt string
		if v.own!=0 {
			oe = " *OWN*"
		} else {
			oe = ""
		}

		if v.sentcnt==0 {
			snt = "never sent"
		} else {
			snt = fmt.Sprintf("sent %d times, last %s ago", v.sentcnt,
				time.Now().Sub(v.lastsent).String())
		}
		fmt.Printf("%5d) %s - %d bytes - %s%s\n", cnt,
			btc.NewUint256(k[:]).String(), len(v.data), snt, oe)
	}
	tx_mutex.Unlock()
}


func baned_txs(par string) {
	fmt.Println("Rejected transactions:")
	cnt := 0
	tx_mutex.Lock()
	for k, v := range TransactionsRejected {
		cnt++
		fmt.Println("", cnt, btc.NewUint256(k[:]).String(), "-", v.size, "bytes",
			"-", v.reason, "-", time.Now().Sub(v.Time).String(), "ago")
	}
	tx_mutex.Unlock()
}


func send_all_tx(par string) {
	tx_mutex.Lock()
	for k, v := range TransactionsToSend {
		if v.own!=0 {
			cnt := NetRouteInv(1, btc.NewUint256(k[:]), nil)
			v.sentcnt += cnt
			v.lastsent = time.Now()
			fmt.Println("INV for TxID", btc.NewUint256(k[:]).String(), "sent to", cnt, "node(s)")
		}
	}
	tx_mutex.Unlock()
}

func init () {
	newUi("txload tx", true, load_tx, "Load transaction data from the given file, decode it and store in memory")
	newUi("txsend stx", true, send_tx, "Broadcast transaction from memory pool (identified by a given <txid>)")
	newUi("txsendall stxa", true, send_all_tx, "Broadcast all the transactions (what you see after ltx)")
	newUi("txdel dtx", true, del_tx, "Remove a transaction from memory pool (identified by a given <txid>)")
	newUi("txdecode td", true, dec_tx, "Decode a transaction from memory pool (identified by a given <txid>)")
	newUi("txlist ltx", true, list_txs, "List all the transaction loaded into memory pool")
	newUi("txlistban ltxb", true, baned_txs, "List the transaction that we have rejected")
}