package textui

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sort"
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
		return
	}
	network.TxMutex.Lock()
	defer network.TxMutex.Unlock()

	if tx, ok := network.TransactionsToSend[txid.BIdx()]; ok {
		tx.Delete(true, 0)
		fmt.Println("Tx", txid.String(), "removed from ToSend")
		return
	}
	if txr, ok := network.TransactionsRejected[txid.BIdx()]; ok {
		network.DeleteRejected(txr.Id.BIdx())
		fmt.Println("TxR", txid.String(), "removed from Rejected")
		return
	}
	fmt.Println("No such transaction ID in the memory pool.")
}

func local_tx(par string) {
	ps := strings.SplitN(par, " ", 3)
	txid := btc.NewUint256FromString(ps[0])
	if txid == nil {
		fmt.Println("You must specify a valid transaction ID for this command.")
		return
	}
	local := len(ps) == 1 || ps[1] != "0"
	if tx := network.TransactionsToSend[txid.BIdx()]; tx != nil {
		if tx.Local != local {
			tx.Local = local
		} else {
			fmt.Println("This transaction is already marked as such.")
		}
	} else {
		fmt.Println("No such transaction ID in the memory pool.")
	}
}

func decode_tx(pars string) {
	var tx *btc.Tx
	ps := strings.SplitN(pars, " ", 3)
	txid := btc.NewUint256FromString(ps[0])
	if txid == nil {
		fmt.Println("You must specify a valid transaction ID for this command.")
		return
	}
	var par string
	if len(ps) > 1 {
		par = ps[1]
	}
	t2s := network.TransactionsToSend[txid.BIdx()]
	txr := network.TransactionsRejected[txid.BIdx()]
	if t2s == nil && txr == nil {
		fmt.Println("No such transaction ID in the memory pool.")
		return
	}
	if t2s != nil {
		tx = t2s.Tx
	} else {
		fmt.Println("*** Transaction Rejected ***")
		tx = txr.Tx
		if tx == nil {
			fmt.Println("Transaction data not available.")
			par = "int"
		}
	}

	var done bool
	if par == "raw" || par == "all" {
		fmt.Println(hex.EncodeToString(tx.Raw))
		done = true
	}
	if par == "int" || par == "all" {
		if done {
			fmt.Println()
		}
		if t2s != nil {
			fmt.Println("Invs sent cnt:", t2s.Invsentcnt)
			fmt.Println("Tx sent cnt:", t2s.SentCnt)
			fmt.Println("Frst seen:", t2s.Firstseen.Format("2006-01-02 15:04:05"))
			fmt.Println("Last seen:", t2s.Lastseen.Format("2006-01-02 15:04:05"))
			if t2s.SentCnt > 0 {
				fmt.Println("Last sent:", t2s.Lastsent.Format("2006-01-02 15:04:05"))
			}
			fmt.Println("Volume:", t2s.Volume)
			fmt.Println("Fee:", t2s.Fee)
			fmt.Println("MemInputCnt:", t2s.MemInputCnt, " ", t2s.MemInputs)
			fmt.Println("SigopsCost:", t2s.SigopsCost)
			fmt.Println("VerifyTime:", t2s.VerifyTime.String())
			fmt.Println("Local:", t2s.Local)
			fmt.Println("Blocked:", t2s.Blocked)
			fmt.Println("Final:", t2s.Final)
		}
		if txr != nil {
			fmt.Println("Reason:", txr.Reason, network.ReasonToString(txr.Reason))
			fmt.Println("Time:", txr.Time.Format("2006-01-02 15:04:05"))
			fmt.Println("Size:", txr.Size)
			if txr.Waiting4 != nil {
				fmt.Println("Waiting for:", txr.Waiting4.String())
			}
		}
		done = true
	}
	if !done || par == "all" {
		if done {
			fmt.Println()
		}
		usif.DecodeTx(os.Stdout, tx)
	}
}

func save_tx(par string) {
	txid := btc.NewUint256FromString(par)
	if txid == nil {
		fmt.Println("You must specify a valid transaction ID for this command.")
		return
	}
	var tx *btc.Tx

	if t2s := network.TransactionsToSend[txid.BIdx()]; t2s != nil {
		tx = t2s.Tx
	} else {
		if txr := network.TransactionsRejected[txid.BIdx()]; txr != nil {
			tx = txr.Tx
		}
	}
	if tx != nil {
		fn := tx.Hash.String() + ".tx"
		os.WriteFile(fn, tx.Raw, 0600)
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
	var reason byte
	if par != "" {
		if val, er := strconv.ParseUint(par, 10, 64); er != nil || val < 1 || val > 255 {
			println("Rejection reason must be a value between 1 and 255")
			return
		} else {
			reason = byte(val)
		}
	}
	fmt.Println("Listing Rejected transactions", reason, ":")
	cnt := 0
	network.TxMutex.Lock()
	sorted := network.GetSortedRejected()
	for _, v := range sorted {
		if reason != 0 && reason != v.Reason {
			continue
		}
		var bts string = "bytes"
		cnt++
		if v.Tx == nil {
			bts = "v-bts"
		}
		fmt.Println("", cnt, v.Id.String(), "-", v.Size, bts,
			"-", v.Reason, "-", time.Since(v.Time).String(), "ago")
	}
	network.TxMutex.Unlock()
}

func txr_purge(par string) {
	var minage time.Duration = time.Hour
	var commit bool
	ss := strings.Split(par, " ")
	for _, s := range ss {
		if s == "commit" {
			commit = true
			continue
		}
		if tmp, er := strconv.ParseUint(par, 10, 64); er == nil {
			minage = time.Duration(tmp) * time.Minute
		} else {
			fmt.Println("Argument must be either commit or a positive integer")
			return
		}
	}

	tim := time.Now().Add(-minage)

	fmt.Println("Purging data of all transactions rejected before", tim.Format(("2006-01-02 15:04:05")))

	todo := make([]network.BIDX, 0, 100)
	network.TxMutex.Lock()
	for k, txr := range network.TransactionsRejected {
		if txr.Tx != nil && txr.Time.Before(tim) {
			todo = append(todo, k)
			var ds string
			if txr.Tx != nil {
				ds = fmt.Sprint(len(txr.Tx.Raw), " bytes")
			} else {
				ds = fmt.Sprint(txr.Size, " v-bts")
			}
			fmt.Printf("%4d) %s  %s  %s\n", len(todo), txr.Id.String(), network.ReasonToString(txr.Reason), ds)
		}
	}
	if len(todo) > 0 {
		if commit {
			for _, k := range todo {
				network.DeleteRejected(k)
			}
			fmt.Println(len(todo), "rejected txs deleted")
			common.CountSafeAdd("TxRDelUiTot", uint64(len(todo)))
		}
	} else {
		fmt.Println("Nothing found")
	}
	network.TxMutex.Unlock()
}

func txr_stats(par string) {
	type rect struct {
		totsize, memsize uint64
		totcnt, memcnt   uint32
		from, to         time.Time
	}
	cnts := make(map[byte]*rect)
	var reasons []int

	network.TxMutex.Lock()
	fmt.Println(len(network.TransactionsRejected), "transactions with total in-memory size of", network.TransactionsRejectedSize)
	for _, v := range network.TransactionsRejected {
		var rec *rect
		if rec = cnts[v.Reason]; rec == nil {
			reasons = append(reasons, int(v.Reason))
			rec = new(rect)
		}
		rec.totsize += uint64(v.Size)
		rec.totcnt++
		if v.Tx != nil {
			rec.memsize += uint64(len(v.Raw))
			rec.memcnt++
		}
		if rec.from.IsZero() {
			rec.from = v.Time
			rec.to = v.Time
		} else {
			if v.Time.Before(rec.from) {
				rec.from = v.Time
			} else if rec.to.Before(v.Time) {
				rec.to = v.Time
			}
		}
		cnts[v.Reason] = rec
	}
	sort.Ints(reasons)
	for _, r := range reasons {
		rea := byte(r)
		rec := cnts[rea]
		fmt.Println("  Reason:", rea, network.ReasonToString(rea))
		fmt.Println("    Total Size:", rec.totsize, "in", rec.totcnt, "recs", "   InMem Size:", rec.memsize, "in", rec.memcnt, "recs")
		fmt.Println("    Time from", rec.from.Format("2006-01-02 15:04:05"), "to", rec.to.Format("2006-01-02 15:04:05"))
	}
	cnt := 0
	for _, lst := range network.RejectedUsedUTXOs {
		cnt += len(lst)
	}
	fmt.Println("RejectedUsedUTXOs count:", cnt, "in", len(network.RejectedUsedUTXOs), "records")
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
	network.TxMutex.Lock()
	network.MempoolSave(true)
	network.TxMutex.Unlock()
}

func check_txs(par string) {
	network.TxMutex.Lock()
	err := network.MempoolCheck()
	network.TxMutex.Unlock()
	if !err {
		fmt.Println("Memory Pool seems to be consistent")
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

func mempool_purge(par string) {
	network.InitMempool()
	fmt.Println("Done")
}

func push_old_txs(par string) {
	var invs uint32
	var weight uint64
	var max_spb float64
	var er error
	var push, purge bool
	var txs_found []*network.OneTxToSend
	ss := strings.SplitN(par, " ", 2)
	if len(ss) >= 1 {
		max_spb, er = strconv.ParseFloat(ss[0], 64)
		if er != nil {
			max_spb = 10.0
		}
		if len(ss) >= 2 {
			if ss[1] == "push" {
				push = true
			} else if ss[1] == "purge" {
				purge = true
			} else {
				fmt.Println("The second argument must be eiter push or purge")
			}
		}

	}
	fmt.Printf("Looking for txs last seen over a day ago with SPB above %.1f\n", max_spb)
	network.TxMutex.Lock()
	for _, tx := range network.TransactionsToSend {
		if tx.MemInputCnt == 0 && time.Since(tx.Lastseen) > 24*time.Hour {
			spb := tx.SPB()
			if spb >= max_spb {
				wg := tx.Weight()
				txs_found = append(txs_found, tx)
				weight += uint64(wg)
				if !push && !purge {
					fmt.Printf("%d) %s  %.1f spb, %.1f kW,  %.1f day\n", len(txs_found), tx.Hash.String(), spb,
						float64(wg)/1000.0, float64(time.Since(tx.Lastseen))/float64(24*time.Hour))
				}
			}
		}
	}
	totlen := len(network.TransactionsToSend)
	fmt.Println("Found", len(txs_found), "/", totlen, "txs matching the criteria, with total weight of", weight)
	if push || purge {
		for _, tx := range txs_found {
			if push {
				invs += network.NetRouteInvExt(network.MSG_TX, &tx.Hash, nil, uint64(1000.0*tx.SPB()))
			} else if purge {
				tx.Delete(true, 0)
			}
		}
		fmt.Println("Number of invs sent:", invs)
		fmt.Println("Number of txs purged:", totlen-len(network.TransactionsToSend))
	} else {
		fmt.Println("Add push to broadcast them to peers, or purge to delete them from mempool")
	}
	network.TxMutex.Unlock()
	if !push {
		fmt.Printf("Execute 'pusholdtxs %.1f yes' to send all the invs\n", max_spb)
	}
}

func init() {
	newUi("mpcheck mpc", true, check_txs, "Verify consistency of mempool")
	newUi("mpget mpg", true, get_mempool, "Send getmp message to the peer with the given ID")
	newUi("mpurge", true, mempool_purge, "Purge memory pool (restart from empty)")
	newUi("mpsave mps", true, save_mempool, "Save memory pool to disk")
	newUi("mpstat mp", true, mempool_stats, "Show the mempool statistics")
	newUi("savetx txs", true, save_tx, "Save raw tx from memory pool to disk: <txid>")
	newUi("tx1send stx1", true, send1_tx, "Broadcast tx to a single random peer: <txid>")
	newUi("txdecode td", true, decode_tx, "Decode tx from memory pool: <txid> [int|raw|all]")
	newUi("txdel", true, del_tx, "Remove tx from memory: <txid>")
	newUi("txlist ltx", true, list_txs, "List tx from memory pool up to: <max_weigth> (default 4M)")
	newUi("txload txl", true, load_tx, "Load tx data from the given file, decode it and store in memory")
	newUi("txlocal txloc", true, local_tx, "Mark tx as local: <txid> [0|1]")
	newUi("txold to", true, push_old_txs, "Push or delete txs not seen for 1+ day: <SPB> [push|purge]")
	newUi("txrlist rtl", true, baned_txs, "List the tx that we have rejected: [<reason>]")
	newUi("txrpurge rtp", true, txr_purge, "Purge txs from rejected list: [<min_age_in_minutes>] [commit]")
	newUi("txrstat rts", true, txr_stats, "Show stats of the rejected txs")
	newUi("txsend stx", true, send_tx, "Broadcast tx from memory pool: <txid>")
	newUi("txsendall stxa", true, send_all_tx, "Broadcast all the local txs (what you see after ltx)")
}
