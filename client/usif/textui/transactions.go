package textui

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/usif"
	"github.com/piotrnar/gocoin/lib/btc"
)

func load_tx(par string) {
	if par == "" {
		fmt.Println("Specify a name of a transaction file")
		return
	}
	f, e := os.Open(par)
	if e != nil {
		println(e.Error())
		return
	}
	n, _ := f.Seek(0, io.SeekStart)
	f.Seek(0, io.SeekEnd)
	buf := make([]byte, n)
	f.Read(buf)
	f.Close()
	fmt.Println(usif.LoadRawTx(buf))
}

func send_tx(par string) {
	txid := btc.NewUint256FromString(par)
	if txid == nil {
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
	if txid == nil {
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
	if txid == nil {
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
	tx.Delete(true, 0)
	fmt.Println("Transaction", txid.String(), "and all its children removed from the memory pool")
}

func dec_tx(par string) {
	txid := btc.NewUint256FromString(par)
	if txid == nil {
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
	if txid == nil {
		fmt.Println("You must specify a valid transaction ID for this command.")
		return
	}
	if tx, ok := network.TransactionsToSend[txid.BIdx()]; ok {
		fn := tx.Hash.String() + ".txt"
		os.WriteFile(fn, []byte(hex.EncodeToString(tx.Raw)), 0600)
		fmt.Println("Saved to", fn)
	} else {
		fmt.Println("No such transaction ID in the memory pool.")
	}
}

func mempool_stats(par string) {
	fmt.Print(usif.MemoryPoolFees())
}

func list_txs(par string) {
	var er error
	var maxweigth uint64
	maxweigth, er = strconv.ParseUint(par, 10, 64)
	if er != nil || maxweigth > 4e6 {
		maxweigth = 4e6
	}
	fmt.Println("Listing txs in mempool up to weight:", maxweigth)
	cnt := 0
	network.TxMutex.Lock()
	defer network.TxMutex.Unlock()

	sorted := network.GetSortedMempool()

	var totlen, totweigth uint64
	for cnt = 0; cnt < len(sorted); cnt++ {
		v := sorted[cnt]
		totweigth += uint64(v.Weight())
		totlen += uint64(len(v.Raw))

		if totweigth > maxweigth {
			break
		}

		var snt string
		if v.SentCnt == 0 {
			snt += "tx never"
		} else {
			snt += fmt.Sprintf("tx %d times, last %s ago", v.SentCnt,
				time.Since(v.Lastsent).String())
		}
		if v.Local {
			snt += " *OWN*"
		}

		fmt.Printf("%5d) ...%7d/%7d %s %6d bytes / %4.1fspb - INV snt %d times, %s\n",
			cnt, totlen, totweigth, v.Tx.Hash.String(), len(v.Raw), v.SPB(), v.Invsentcnt, snt)

	}
}

func baned_txs(par string) {
	fmt.Println("Rejected transactions:")
	cnt := 0
	network.TxMutex.Lock()
	for k, v := range network.TransactionsRejected {
		cnt++
		fmt.Println("", cnt, btc.NewUint256(k[:]).String(), "-", v.Size, "bytes",
			"-", v.Reason, "-", time.Since(v.Time).String(), "ago")
	}
	network.TxMutex.Unlock()
}

func send_all_tx(par string) {
	var tmp []*network.OneTxToSend
	network.TxMutex.Lock()
	for _, v := range network.TransactionsToSend {
		if v.Local {
			tmp = append(tmp, v)
		}
	}
	network.TxMutex.Unlock()
	for _, v := range tmp {
		cnt := network.NetRouteInv(1, &v.Tx.Hash, nil)
		v.Invsentcnt += cnt
		fmt.Println("INV for TxID", v.Tx.Hash.String(), "sent to", cnt, "node(s)")
	}
}

func save_mempool(par string) {
	network.MempoolSave(true)
}

func check_txs(par string) {
	network.TxMutex.Lock()
	err := network.MempoolCheck()
	network.TxMutex.Unlock()
	if !err {
		fmt.Println("Memory Pool seems to be consistent")
	}
}

func load_mempool(par string) {
	if par == "" {
		par = common.GocoinHomeDir + "mempool.dmp"
	}
	var abort bool
	__exit := make(chan bool)
	__done := make(chan bool)
	go func() {
		for {
			select {
			case s := <-common.KillChan:
				fmt.Println(s)
				abort = true
			case <-__exit:
				__done <- true
				return
			}
		}
	}()
	fmt.Println("Press Ctrl+C to abort...")
	network.MempoolLoadNew(par, &abort)
	__exit <- true
	<-__done
	if abort {
		fmt.Println("Aborted")
	}
}

func get_mempool(par string) {
	conid, e := strconv.ParseUint(par, 10, 32)
	if e != nil {
		fmt.Println("Specify ID of the peer")
		return
	}

	fmt.Println("Getting mempool from connection ID", conid, "...")
	network.GetMP(uint32(conid))
}

func push_old_txs(par string) {
	var invs, cnt uint32
	var weight uint64
	var max_spb float64
	var er error
	var commit bool
	ss := strings.SplitN(par, " ", 2)
	if len(ss) >= 1 {
		max_spb, er = strconv.ParseFloat(ss[0], 64)
		if er != nil {
			max_spb = 10.0
		}
		commit = len(ss) >= 2 && ss[1] == "yes"
	}
	fmt.Printf("Looking for txs last seen over a day ago with SPB above %.1f\n", max_spb)
	network.TxMutex.Lock()
	for _, tx := range network.TransactionsToSend {
		if tx.MemInputCnt == 0 && time.Since(tx.Lastseen) > 24*time.Hour {
			spb := tx.SPB()
			if spb >= max_spb {
				wg := tx.Weight()
				if commit {
					invs += network.NetRouteInvExt(network.MSG_TX, &tx.Hash, nil, uint64(1000.0*spb))
				} else {
					fmt.Printf("%d) %s  %.1f spb, %.1f kW,  %.1f day\n", cnt+1, tx.Hash.String(), spb,
						float64(wg)/1000.0, float64(time.Since(tx.Lastseen))/float64(24*time.Hour))
				}
				cnt++
				weight += uint64(wg)
			}
		}
	}
	fmt.Println("Found", cnt, "/", len(network.TransactionsToSend), "txs matching the criteria")
	network.TxMutex.Unlock()
	fmt.Println("Total weigth:", weight, "   number of invs sent:", invs)
	if !commit {
		fmt.Printf("Execute 'pusholdtxs %.1f yes' to send all the invs\n", max_spb)
	}
}

func init() {
	newUi("txload tx", true, load_tx, "Load transaction data from the given file, decode it and store in memory")
	newUi("txsend stx", true, send_tx, "Broadcast transaction from memory pool (identified by a given <txid>)")
	newUi("tx1send stx1", true, send1_tx, "Broadcast transaction to a single random peer (identified by a given <txid>)")
	newUi("txsendall stxa", true, send_all_tx, "Broadcast all the transactions (what you see after ltx)")
	newUi("txdel dtx", true, del_tx, "Remove a transaction from memory pool (identified by a given <txid>)")
	newUi("txdecode td", true, dec_tx, "Decode a transaction from memory pool (identified by a given <txid>)")
	newUi("txlist ltx", true, list_txs, "List all the transaction loaded into memory pool up to <max_weigth> (default 4M)")
	newUi("txlistban ltxb", true, baned_txs, "List the transaction that we have rejected")
	newUi("mempool mp", true, mempool_stats, "Show the mempool statistics")
	newUi("savetx txsave", true, save_tx, "Save raw transaction from memory pool to disk")
	newUi("txmpsave mps", true, save_mempool, "Save memory pool to disk")
	newUi("txcheck txc", true, check_txs, "Verify consistency of mempool")
	newUi("txmpload mpl", true, load_mempool, "Load transaction from the given file (must be in mempool.dmp format)")
	newUi("getmp mpg", true, get_mempool, "Send getmp message to the peer with the given ID")
	newUi("pusholdtxs pot", true, push_old_txs, "Push old txs <SPB> [yes]")
}
