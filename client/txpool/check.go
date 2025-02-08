package txpool

import (
	"encoding/hex"
	"fmt"
	"os"
	"runtime/debug"
	"slices"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
)

var MPCheckUTXO bool = true

func CheckForErrors() bool {
	return common.CheckForMempoolErrors.Get()
}

func (t *OneTxToSend) isInMap() (yes bool) {
	var tt *OneTxToSend
	tt, yes = TransactionsToSend[t.Hash.BIdx()]
	if yes && tt != t {
		println("ERROR: t2x in the map does not point back to itself", t.Hash.String(), "\n  ", tt.Hash.String())
		yes = false
	}
	return
}

func (pk *OneTxsPackage) checkForDups() bool {
	for i, t := range pk.Txs[:len(pk.Txs)-1] {
		if slices.Contains(pk.Txs[i+1:], t) {
			println("ERROR: pkg", pk.String(), "contains the same tx twice:", t.Hash.String())
			debug.PrintStack()
			os.Exit(1)
			return true
		}
	}
	return false
}

func checkFootprints() (dupa int) {
	var t2s_size, txr_size, w4i_size int
	for _, t2x := range TransactionsToSend {
		ts := t2x.SysSize()
		if ts != int(t2x.Footprint) {
			dupa++
			fmt.Println(dupa, "ERROR: T2S", t2x.Hash.String(), "mismatch footprint:", t2x.Footprint, ts)
		}
		t2s_size += ts
		if t2x.TxVerVars != nil {
			dupa++
			fmt.Println(dupa, "ERROR: T2S", t2x.Hash.String(), "does not seem to be clean")
		}
	}
	for _, txr := range TransactionsRejected {
		ts := txr.SysSize()
		if ts != int(txr.Footprint) {
			dupa++
			fmt.Println(dupa, "ERROR: TxR", txr.Hash.String(), "mismatch footprint:", txr.Footprint, ts)
		}
		txr_size += ts
		if txr.Waiting4 != nil {
			w4i_size += ts
		}
	}

	if TransactionsToSendSize != uint64(t2s_size) {
		dupa++
		fmt.Println("ERROR: TransactionsToSendSize mismatch:", TransactionsToSendSize, "  real:", t2s_size)
	}

	if TransactionsRejectedSize != uint64(txr_size) {
		dupa++
		fmt.Println("ERROR: TransactionsRejectedSize mismatch:", TransactionsRejectedSize, "  real:", txr_size)
	}

	if WaitingForInputsSize != uint64(w4i_size) {
		dupa++
		fmt.Println("ERROR: WaitingForInputsSize mismatch:", WaitingForInputsSize, "  real:", w4i_size)
	}

	return
}

func checkMempoolTxs() (dupa int) {
	var spent_cnt int

	for _, t2s := range TransactionsToSend {
		var micnt uint32
		if t2s.NoWitSize == 0 || t2s.Size == 0 {
			dupa++
			fmt.Println(dupa, "Tx", t2s.Hash.String(), "has broken size:", t2s.NoWitSize, t2s.Size)
		}

		for i, inp := range t2s.TxIn {
			spent_cnt++
			if outk, ok := SpentOutputs[inp.Input.UIdx()]; ok {
				if outk != t2s.Hash.BIdx() {
					dupa++
					fmt.Println(dupa, "Tx", t2s.Hash.String(), "input has a mismatch in SpentOutputs record", i, outk)
				}
			} else {
				dupa++
				fmt.Println(dupa, "Tx", t2s.Hash.String(), "input is not in SpentOutputs", i)
			}

			_, ok := TransactionsToSend[btc.BIdx(inp.Input.Hash[:])]
			var check_utxo_db bool

			if t2s.MemInputs == nil {
				if ok {
					dupa++
					fmt.Println(dupa, "Tx", t2s.Hash.String(), "MemInputs==nil but input is in mempool", i, inp.Input.String())
				}
				check_utxo_db = true
			} else {
				if t2s.MemInputs[i] {
					micnt++
					if !ok {
						dupa++
						fmt.Println(dupa, "Tx", t2s.Hash.String(), "MemInput set but input NOT in mempool", i, inp.Input.String())
					}
				} else {
					check_utxo_db = true
				}
			}
			if MPCheckUTXO && check_utxo_db {
				if ok {
					dupa++
					fmt.Println(dupa, "Tx", t2s.Hash.String(), "MemInput NOT set but input IS in mempool", i, inp.Input.String())
				}
				if unsp := common.BlockChain.Unspent.UnspentGet(&inp.Input); unsp == nil {
					dupa++
					fmt.Println(dupa, "Mempool tx", t2s.Hash.String(), "has no valid input in UTXO db:", i)
				}
			}
		}
		if t2s.MemInputs != nil && micnt == 0 {
			dupa++
			fmt.Println(dupa, "Tx", t2s.Hash.String(), "has MemInputs array with all false values")
		}
		if t2s.MemInputCnt != micnt {
			dupa++
			fmt.Println(dupa, "Tx", t2s.Hash.String(), "has incorrect MemInputCnt", t2s.MemInputCnt, micnt)
		}
	}

	for _, so := range SpentOutputs {
		if _, ok := TransactionsToSend[so]; !ok {
			dupa++
			fmt.Println(dupa, "SpentOutput", btc.BIdxString(so), "does not have tx in mempool")
		}
	}
	if spent_cnt != len(SpentOutputs) {
		dupa++
		fmt.Println(dupa, "SpentOutputs length mismatch", spent_cnt, len(SpentOutputs))
	}
	return
}

func checkRejectedTxs() (dupa int) {
	var w4i_cnt int
	var spent_cnt int
	for _, tr := range TransactionsRejected {
		if tr.Tx != nil {
			if tr.Tx.Raw == nil {
				dupa++
				fmt.Println(dupa, "TxR", tr.Id.String(), "has Tx but no Raw")
			}
			if tr.Reason < 200 {
				dupa++
				fmt.Println(dupa, "TxR", tr.Id.String(), "reason", ReasonToString(tr.Reason), "but no data")
			}
		} else {
			if tr.Reason >= 200 {
				dupa++
				fmt.Println(dupa, "TxR", tr.Id.String(), "has data, reason", ReasonToString(tr.Reason))
			}
		}

		if tr.Waiting4 != nil {
			if tr.Reason != TX_REJECTED_NO_TXOU {
				dupa++
				fmt.Println(dupa, "TxR", tr.Id.String(), "has w4i and reason", ReasonToString(tr.Reason))
			}
			if tr.Tx == nil || tr.Tx.Raw == nil {
				dupa++
				fmt.Println(dupa, "TxR", tr.Id.String(), tr.Reason, "has w4i but no tx data")
			} else {
				w4i_cnt++
			}
		} else {
			if tr.Reason == TX_REJECTED_NO_TXOU {
				dupa++
				fmt.Println(dupa, "TxR", tr.Id.String(), "has not w4i but reason", ReasonToString(tr.Reason))
			}
		}
	}
	for _, rec := range WaitingForInputs {
		if len(rec.Ids) == 0 {
			dupa++
			fmt.Println(dupa, "WaitingForInputs", rec.TxID.String(), "has zero records")
		}
		spent_cnt += len(rec.Ids)
	}
	if w4i_cnt != spent_cnt {
		dupa++
		fmt.Println(dupa, "WaitingForInputs count mismatch", w4i_cnt, spent_cnt)
	}
	return
}

func checkSortIndex() (dupa int) {
	seen := make(map[btc.BIDX]int)
	for idx := TRIdxTail; ; idx = TRIdxNext(idx) {
		bidx := TRIdxArray[idx]
		if txr := TransactionsRejected[bidx]; txr != nil {
			if idx, ok := seen[bidx]; ok {
				dupa++
				fmt.Println(dupa, "TxR", txr.Id.String(), ReasonToString(txr.Reason), "from idx", idx,
					"present again in TRIdxArray at", idx, TRIdxHead, TRIdxTail)
			} else {
				seen[bidx] = idx
			}
		} else {
			if !TRIdIsZeroArrayRec(idx) {
				dupa++
				fmt.Println(dupa, "TRIdxArray index", idx, "is not zero", hex.EncodeToString(bidx[:]),
					"but has not txr in the map", TRIdxHead, TRIdxTail)
				break
			}
		}
		if idx == TRIdxHead {
			break
		}
	}
	return dupa
}

func checkRejectedUsedUTXOs() (dupa int) {
	var spent_cnt int
	for utxoidx, lst := range RejectedUsedUTXOs {
		spent_cnt += len(lst)
		for _, bidx := range lst {
			if txr, ok := TransactionsRejected[bidx]; ok && txr.Tx != nil {
				var found bool
				for _, inp := range txr.TxIn {
					if _, ok := RejectedUsedUTXOs[inp.Input.UIdx()]; ok {
						found = true
						break
					}
					if !found {
						dupa++
						fmt.Println(dupa, "Tx", txr.Id.String(), "in RejectedUsedUTXOs but without back reference to RejectedUsedUTXOs")
					}
				}

			} else {
				dupa++
				fmt.Println(dupa, "btc.BIDX", btc.BIdxString(bidx), "present in RejectedUsedUTXOs",
					fmt.Sprintf("%016x", utxoidx), "but not in TransactionsRejected")
				if t2s, ok := TransactionsToSend[bidx]; ok {
					fmt.Println("   It is however in T2S", t2s.Hash.String())
				} else {
					fmt.Println("   Not it is in T2S")
				}
			}
		}
	}
	tot_utxo_used := 0
	for _, tr := range TransactionsRejected {
		if tr.Tx != nil && tr.Tx.Raw != nil {
			tot_utxo_used += len(tr.Tx.TxIn)
		}
	}
	if spent_cnt != tot_utxo_used {
		dupa++
		fmt.Println(dupa, "RejectedUsedUTXOs count mismatch", spent_cnt, tot_utxo_used)

		fmt.Println("Checking which txids are missing...")
		for bidx, txr := range TransactionsRejected {
			if txr.Tx == nil {
				continue
			}
			for _, inp := range txr.TxIn {
				uidx := inp.Input.UIdx()
				if lst, ok := RejectedUsedUTXOs[uidx]; ok {
					var found bool
					for _, bi := range lst {
						if bidx == bi {
							found = true
							break
						}
					}
					if !found {
						fmt.Println(" - Missing on list", inp.Input.String(), "\n  for", txr.Id.String())
					}
				} else {
					fmt.Println(" - Missing record", inp.Input.String(), "\n  for", txr.Id.String())
				}
				RejectedUsedUTXOs[uidx] = append(RejectedUsedUTXOs[uidx], txr.Id.BIdx())
			}
		}
	}
	return
}

func checkFeeList() bool {
	if FeePackagesDirty {
		common.CountSafe("TxPkgsCheckDirty")
		return false
	}

	valid_pkgs := make(map[*OneTxsPackage]bool, len(FeePackages))

	for _, pkg := range FeePackages {
		if valid_pkgs[pkg] {
			println("ERROR: pkg", pkg.String(), "is twice on the list")
			return true
		}
		valid_pkgs[pkg] = true
		if len(pkg.Txs) < 2 {
			println("ERROR: package has only", len(pkg.Txs), "txs")
			return true
		}
		for idx, t := range pkg.Txs {
			if !t.isInMap() {
				println("ERROR: tx in pkg", pkg.String(), "does not point to a valid t2s", idx)
				println("    ...", t.Hash.String())
				return true
			}
			if !slices.Contains(t.inPackages, pkg) {
				println("ERROR: tx", idx, t.Id(), "in pkg", pkg.String(), "does not point back to the package")
				return true
			}
		}
	}

	found_pkgs := make(map[*OneTxsPackage]bool, len(valid_pkgs))
	for _, t2s := range TransactionsToSend {
		if t2s.inPackages != nil {
			for _, pkg := range t2s.inPackages {
				if !valid_pkgs[pkg] {
					println("ERROR: pkg", pkg.String(), "from t2s", t2s.Id(), "is not on the pkg list")
					return true
				}
				if !slices.Contains(pkg.Txs, t2s) {
					println("ERROR: pkg", pkg.String(), "does not have the tx", t2s.Id)
					return true
				}
				found_pkgs[pkg] = true
			}
		}
	}

	if len(found_pkgs) != len(valid_pkgs) {
		println("ERROR: did not find a reference to every pkg in the mempool", len(found_pkgs), (valid_pkgs))
		return true
	}

	common.CountSafe("TxPkgsCheckOK")
	return false
}

func VerifyMempoolSort(txs []*OneTxToSend) bool {
	idxs := make(map[btc.BIDX]int, len(txs))
	for i, t2s := range txs {
		if t2s == nil {
			println("tx at idx", i, len(txs), len(TransactionsToSend), "is nil")
			return true
		}
		idxs[t2s.Hash.BIdx()] = i
	}
	var oks int
	for i, t2s := range txs {
		if t2s.Weight() == 0 {
			println("ERROR: in mempool sorting:", i, "has weight 0", t2s.Hash.String())
			return true
		}
		for _, txin := range t2s.TxIn {
			if idx, ok := idxs[btc.BIdx(txin.Input.Hash[:])]; ok {
				if idx > i {
					println("ERROR: in mempool sorting:", i, "points to", idx, "\n",
						"    ", i, t2s.Hash.String(), "\n",
						" -> ", idx, btc.NewUint256(txin.Input.Hash[:]).String())
					return true
				} else {
					oks++
				}
			}
		}
	}
	//println("mempool sorting OK", oks, len(txs))
	return false
}

// MempoolCheck verifies the Mempool for consistency.
// Usefull for debuggning as normally there should be no consistencies.
// Make sure to call it with TxMutex Locked.
func MempoolCheck() bool {
	var dupa int
	dupa += checkFootprints()
	dupa += checkMempoolTxs()
	dupa += checkRejectedTxs()
	dupa += checkSortIndex()
	dupa += checkRejectedUsedUTXOs()
	if checkFeeList() {
		dupa++
		fmt.Println(dupa, "checkFeeList failed")
	}
	if VerifyMempoolSort(GetSortedMempool()) {
		dupa++
		fmt.Println(dupa, "GetSortedMempool() sorting broken")
	}
	if VerifyMempoolSort(GetSortedMempoolRBF()) {
		dupa++
		fmt.Println(dupa, "GetSortedMempoolRBF() sorting broken")
	}
	if dupa == 0 {
		common.CountSafe("Tx MPCheckOK")
	}
	return dupa != 0
}

/*
var donot bool
var rdbg *bytes.Buffer
var rd1, rd2 *bytes.Buffer

func dumpPkgList(fn string) {
	f, _ := os.Create(fn)
	dumpPkgListHere(f)
	f.Close()
	println("pkg list stored in", fn)
}

func dumpPkgListHere(f io.Writer) {
	for _, pkg := range FeePackages {
		fmt.Fprintln(f, "package", pkg.String(), "with", len(pkg.Txs), "txs:")
		for _, t := range pkg.Txs {
			fmt.Fprintln(f, "   *", t.Hash.String(), "  mic:", t.MemInputCnt, "  inpkgs:", len(t.inPackages))
			for idx, pkg := range t.inPackages {
				fmt.Fprintln(f, "   ", idx, pkg.String())
			}
		}
		fmt.Fprintln(f)
	}
}

func cfl(label string) {
	if donot {
		rdbg = nil
		return
	}
	//common.CountSafe("TxPkgs_CLF")
	if checkFeeList() {
		println("*** fee packages list first noticed broken in", label, "\a")
		dumpPkgList("packages_broken.txt")
		debug.PrintStack()
		donot = true

		if rdbg != nil {
			println(rdbg.String())
		}
		if rd1 != nil {
			os.WriteFile("packages_before1.txt", rd1.Bytes(), 0600)
			println("packages_before1.txt created")
		}
		if rd2 != nil {
			os.WriteFile("packages_before2.txt", rd2.Bytes(), 0600)
			println("packages_before2.txt created")
		}
		os.Exit(1)
	}
}

// make sure to call it with the mutex locked
func checkSortedOK(from string) {
	cfl(from)
	if VerifyMempoolSort(GetSortedMempoolRBF()) {
		println("Sorting fucked in", from)
		println("before it:\n", rdbg.String())
		dumpPkgList("pkglist_broken.txt")
		rdbg = new(bytes.Buffer)
		println("Now retry sort again, this time with FeePackagesDirty")
		FeePackagesDirty = true
		if VerifyMempoolSort(GetSortedMempoolRBF()) {
			println("*** again fucked ***")
		} else {
			println(rdbg.String())
			println("Fixed")
			dumpPkgList("pkglist_fixed.txt")
		}
		os.Exit(1)
	}
}
*/
