package textui

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/txpool"
	"github.com/piotrnar/gocoin/client/wallet"
	"github.com/piotrnar/gocoin/lib/btc"
)

type OneWalletAddrs struct {
	Idx int
	Key string
	rec *wallet.OneAllAddrBal
}

type SortedWalletAddrs []OneWalletAddrs

var sort_by_cnt bool

func (sk SortedWalletAddrs) Len() int {
	return len(sk)
}

func (sk SortedWalletAddrs) Less(a, b int) bool {
	if sort_by_cnt {
		return sk[a].rec.Count() > sk[b].rec.Count()
	}
	return sk[a].rec.Value > sk[b].rec.Value
}

func (sk SortedWalletAddrs) Swap(a, b int) {
	sk[a], sk[b] = sk[b], sk[a]
}

func max_outs(par string) {
	sort_by_cnt = true
	all_addrs(par)
}

func best_val(par string) {
	sort_by_cnt = false
	all_addrs(par)
}

func all_addrs(par string) {
	var outs, vals, cnts [wallet.IDX_CNT]uint64
	var best SortedWalletAddrs
	var cnt int = 15
	var mode int = wallet.IDX_CNT

	if !common.Get(&common.WalletON) {
		fmt.Println("Wallet functionality is currently disabled.")
		return
	}

	if par != "" {
		prs := strings.SplitN(par, " ", 2)
		if len(prs) > 0 {
			c, e := strconv.ParseUint(prs[0], 10, 32)
			if e != nil {
				// first argument not a number
				mode = -1
				for idx, symb := range wallet.IDX2SYMB {
					//if strings.ToLower(prs[0]) == strings.ToLower(symb) {
					if strings.EqualFold(prs[0], symb) {
						mode = idx
						e = nil
						break
					}
				}
				if mode >= 0 && len(prs) > 1 {
					if c, e = strconv.ParseUint(prs[1], 10, 32); e == nil {
						cnt = int(c)
					}
				}
			} else {
				cnt = int(c)
			}

			if e != nil || mode < 0 || cnt <= 0 {
				fmt.Println("Specify the address type or/and number of top records to display")
				fmt.Print("Valid address types:")
				for _, symb := range wallet.IDX2SYMB {
					fmt.Print(" ", symb)
				}
				fmt.Println()
				return
			}
		}
	}

	var MIN_BTC uint64 = 100e8
	var MIN_OUTS int = 1000

	if mode != wallet.IDX_CNT {
		MIN_BTC = 0
		MIN_OUTS = 0
	}

	wallet.Browse(func(idx int, k string, rec *wallet.OneAllAddrBal) {
		cnts[idx]++
		vals[idx] += rec.Value
		outs[idx] += uint64(rec.Count())
		if sort_by_cnt && rec.Count() >= MIN_OUTS || !sort_by_cnt && rec.Value >= MIN_BTC {
			best = append(best, OneWalletAddrs{Idx: idx, Key: k, rec: rec})
		}
	})
	for idx := range outs {
		fmt.Println(btc.UintToBtc(vals[idx]), "BTC in", outs[idx], "unspent recs from",
			cnts[idx], wallet.IDX2SYMB[idx], "addresses")
	}

	if sort_by_cnt {
		fmt.Println("Top addresses with at least", MIN_OUTS, "unspent outputs:", len(best))
	} else {
		fmt.Println("Top addresses with at least", btc.UintToBtc(MIN_BTC), "BTC:", len(best))
	}

	sort.Sort(best)

	var pkscr_p2sk [23]byte
	var pkscr_p2kh [25]byte
	var ad *btc.BtcAddr

	pkscr_p2sk[0] = 0xa9
	pkscr_p2sk[1] = 20
	pkscr_p2sk[22] = 0x87

	pkscr_p2kh[0] = 0x76
	pkscr_p2kh[1] = 0xa9
	pkscr_p2kh[2] = 20
	pkscr_p2kh[23] = 0x88
	pkscr_p2kh[24] = 0xac

	for i := 0; i < len(best) && i < cnt; i++ {
		switch best[i].Idx {
		case wallet.IDX_P2KH:
			copy(pkscr_p2kh[3:23], best[i].Key)
			ad = btc.NewAddrFromPkScript(pkscr_p2kh[:], common.CFG.Testnet)
		case wallet.IDX_P2SH:
			copy(pkscr_p2sk[2:22], best[i].Key)
			ad = btc.NewAddrFromPkScript(pkscr_p2sk[:], common.CFG.Testnet)
		case wallet.IDX_P2WKH, wallet.IDX_P2WSH, wallet.IDX_P2TAP:
			ad = new(btc.BtcAddr)
			ad.SegwitProg = new(btc.SegwitProg)
			ad.SegwitProg.HRP = btc.GetSegwitHRP(common.CFG.Testnet)
			ad.SegwitProg.Program = []byte(best[i].Key)
			if best[i].Idx != wallet.IDX_P2TAP {
				ad.SegwitProg.Version = 0
			} else {
				ad.SegwitProg.Version = 1
			}
		}
		fmt.Println(i+1, ad.String(), btc.UintToBtc(best[i].rec.Value), "BTC in", best[i].rec.Count(), "inputs")
	}
}

func list_unspent_addr(ad *btc.BtcAddr) {
	var addr_printed bool
	outscr := ad.OutScript()

	unsp := wallet.GetAllUnspent(ad)
	if len(unsp) != 0 {
		var tot uint64
		sort.Sort(unsp)
		for i := range unsp {
			unsp[i].BtcAddr = nil // no need to print the address here
			tot += unsp[i].Value
		}
		fmt.Println(ad.String(), "has", btc.UintToBtc(tot), "BTC in", len(unsp), "records:")
		addr_printed = true
		for i := range unsp {
			fmt.Println(unsp[i].String())
			txpool.TxMutex.Lock()
			bidx, spending := txpool.SpentOutputs[unsp[i].TxPrevOut.UIdx()]
			var t2s *txpool.OneTxToSend
			if spending {
				t2s, spending = txpool.TransactionsToSend[bidx]
			}
			txpool.TxMutex.Unlock()
			if spending {
				fmt.Println("\t- being spent by TxID", t2s.Hash.String())
			}
		}
	}

	txpool.TxMutex.Lock()
	for _, t2s := range txpool.TransactionsToSend {
		for vo, to := range t2s.TxOut {
			if bytes.Equal(to.Pk_script, outscr) {
				if !addr_printed {
					fmt.Println(ad.String(), "has incoming mempool tx(s):")
					addr_printed = true
				}
				fmt.Printf("%15s BTC confirming as %s-%03d\n",
					btc.UintToBtc(to.Value), t2s.Hash.String(), vo)
			}
		}
	}
	txpool.TxMutex.Unlock()

	if !addr_printed {
		fmt.Println(ad.String(), "has no coins")
	}
}

func list_unspent(addr string) {
	if !common.Get(&common.WalletON) {
		fmt.Println("Wallet functionality is currently disabled.")
		return
	}

	// check for raw public key...
	pk, er := hex.DecodeString(addr)
	if er != nil || len(pk) != 33 || pk[0] != 2 && pk[0] != 3 {
		ad, e := btc.NewAddrFromString(addr)
		if e != nil {
			println(e.Error())
			return
		}
		list_unspent_addr(ad)
		return
	}

	// if here, pk contains a valid public key
	ad := btc.NewAddrFromPubkey(pk, btc.AddrVerPubkey(common.Testnet))
	if ad == nil {
		println("Unexpected error returned by NewAddrFromPubkey()")
		return
	}
	hrp := btc.GetSegwitHRP(common.Testnet)
	list_unspent_addr(ad)

	ad.Enc58str = ""
	ad.SegwitProg = &btc.SegwitProg{HRP: hrp, Version: 1, Program: pk[1:]}
	list_unspent_addr(ad)

	ad.Enc58str = ""
	ad.SegwitProg = &btc.SegwitProg{HRP: hrp, Version: 0, Program: ad.Hash160[:]}
	list_unspent_addr(ad)

	h160 := btc.Rimp160AfterSha256(append([]byte{0, 20}, ad.Hash160[:]...))
	ad = btc.NewAddrFromHash160(h160[:], btc.AddrVerScript(common.Testnet))
	list_unspent_addr(ad)
}

func all_val_stats(s string) {
	if !common.Get(&common.WalletON) {
		fmt.Println("Wallet functionality is currently disabled.")
		return
	}

	wallet.PrintStat()
}

func wallet_on_off(s string) {
	if s == "on" {
		select {
		case wallet.OnOff <- true:
		default:
		}
		return
	} else if s == "off" {
		select {
		case wallet.OnOff <- false:
		default:
		}
		return
	}

	if common.Get(&common.WalletON) {
		fmt.Println("Wallet functionality is currently ENABLED. Execute 'wallet off' to disable it.")
		fmt.Println("")
	} else {
		if perc := common.Get(&common.WalletProgress); perc != 0 {
			fmt.Println("Enabling wallet functionality -", (perc-1)/10, "percent complete. Execute 'wallet off' to abort it.")
		} else {
			fmt.Println("Wallet functionality is currently DISABLED. Execute 'wallet on' to enable it.")
		}
	}

	if pend := common.Get(&common.WalletOnIn); pend > 0 {
		fmt.Println("Wallet functionality will auto enable in", pend, "seconds")
	}
}

func save_balances(s string) {
	if s == "force" {
		wallet.LAST_SAVED_FNAME = ""
	}
	sta := time.Now()
	if er := wallet.SaveBalances(); er != nil {
		fmt.Println("Error:", er.Error())
	} else {
		fmt.Println(wallet.LAST_SAVED_FNAME, "saved in", time.Since(sta).String())
	}
}

func init() {
	newUi("richest r", true, best_val, "Show addresses with most BTC: [[type] count]")
	newUi("maxouts o", true, max_outs, "Show addresses with most UTXOs: [[type] count]")
	newUi("balance a", true, list_unspent, "List UTXOs of BTC address or public key: <string>")
	newUi("allbal ab", true, all_val_stats, "Show balances DB statistics")
	newUi("savebal sb", true, save_balances, "Save balances DB to disk: [force]")
	newUi("wallet w", false, wallet_on_off, "Enable or disable wallet functionality: on|off")
}
