package textui

import (
	"os"
	"fmt"
	"time"
	"errors"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/client/config"
	"github.com/piotrnar/gocoin/client/network"
)


func DecodeTx(tx *btc.Tx) (s string, missinginp bool, totinp, totout uint64, e error) {
	s += fmt.Sprintln("Transaction details (for your information):")
	s += fmt.Sprintln(len(tx.TxIn), "Input(s):")
	for i := range tx.TxIn {
		s += fmt.Sprintf(" %3d %s", i, tx.TxIn[i].Input.String())
		var po *btc.TxOut

		if txinmem, ok := network.TransactionsToSend[tx.TxIn[i].Input.Hash]; ok {
			s += fmt.Sprint(" mempool")
			if int(tx.TxIn[i].Input.Vout) >= len(txinmem.TxOut) {
				s += fmt.Sprintf(" - Vout TOO BIG (%d/%d)!", int(tx.TxIn[i].Input.Vout), len(txinmem.TxOut))
			} else {
				po = txinmem.TxOut[tx.TxIn[i].Input.Vout]
			}
		} else {
			po, _ = config.BlockChain.Unspent.UnspentGet(&tx.TxIn[i].Input)
			if po != nil {
				s += fmt.Sprintf("%8d", po.BlockHeight)
			}
		}
		if po != nil {
			ok := btc.VerifyTxScript(tx.TxIn[i].ScriptSig, po.Pk_script, i, tx, true)
			if !ok {
				s += fmt.Sprintln("\nERROR: The transacion does not have a valid signature.")
				e = errors.New("Invalid signature")
				return
			}
			totinp += po.Value
			s += fmt.Sprintf(" %15.8f BTC @ %s\n", float64(po.Value)/1e8,
				btc.NewAddrFromPkScript(po.Pk_script, config.AddrVersion).String())
		} else {
			s += fmt.Sprintln(" - UNKNOWN INPUT")
			missinginp = true
		}
	}
	s += fmt.Sprintln(len(tx.TxOut), "Output(s):")
	for i := range tx.TxOut {
		totout += tx.TxOut[i].Value
		s += fmt.Sprintf(" %15.8f BTC to %s\n", float64(tx.TxOut[i].Value)/1e8,
			btc.NewAddrFromPkScript(tx.TxOut[i].Pk_script, config.AddrVersion).String())
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


func LoadRawTx(buf []byte) (s string) {
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
	s, missinginp, totinp, totout, er = DecodeTx(tx)
	if er != nil {
		return
	}

	network.TxMutex.Lock()
	if missinginp {
		network.TransactionsToSend[tx.Hash.Hash] = &network.OneTxToSend{Tx:tx, Data:txd, Own:2, Firstseen:time.Now(),
			Volume:totout}
	} else {
		network.TransactionsToSend[tx.Hash.Hash] = &network.OneTxToSend{Tx:tx, Data:txd, Own:1, Firstseen:time.Now(),
			Volume:totinp, Fee:totinp-totout}
	}
	network.TxMutex.Unlock()
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
	fmt.Println(LoadRawTx(buf))
}


func send_tx(par string) {
	txid := btc.NewUint256FromString(par)
	if txid==nil {
		fmt.Println("You must specify a valid transaction ID for this command.")
		list_txs("")
		return
	}
	network.TxMutex.Lock()
	if ptx, ok := network.TransactionsToSend[txid.Hash]; ok {
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


func del_tx(par string) {
	txid := btc.NewUint256FromString(par)
	if txid==nil {
		fmt.Println("You must specify a valid transaction ID for this command.")
		list_txs("")
		return
	}
	network.TxMutex.Lock()
	if _, ok := network.TransactionsToSend[txid.Hash]; !ok {
		network.TxMutex.Unlock()
		fmt.Println("No such transaction ID in the memory pool.")
		list_txs("")
		return
	}
	delete(network.TransactionsToSend, txid.Hash)
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
	if tx, ok := network.TransactionsToSend[txid.Hash]; ok {
		s, _, _, _, _ := DecodeTx(tx.Tx)
		fmt.Println(s)
	} else {
		fmt.Println("No such transaction ID in the memory pool.")
	}
}


func list_txs(par string) {
	fmt.Println("Transactions in the memory pool:")
	cnt := 0
	network.TxMutex.Lock()
	for k, v := range network.TransactionsToSend {
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
		fmt.Printf("%5d) %s - %d bytes - %s%s\n", cnt,
			btc.NewUint256(k[:]).String(), len(v.Data), snt, oe)
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
			fmt.Println("INV for TxID", btc.NewUint256(k[:]).String(), "sent to", cnt, "node(s)")
		}
	}
	network.TxMutex.Unlock()
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
