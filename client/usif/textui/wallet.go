package textui

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/wallet"
	"github.com/piotrnar/gocoin/lib/btc"
)

type OneWalletAddrs struct {
	Typ int // 0-p2kh, 1-p2sh, 2-segwit_prog, 3-taproot
	Key []byte
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

func new_slice(in []byte) (kk []byte) {
	kk = make([]byte, len(in))
	copy(kk, in)
	return
}

func all_addrs(par string) {
	var ptkh_outs, ptkh_vals, ptsh_outs, ptsh_vals uint64
	var ptwkh_outs, ptwkh_vals, ptwsh_outs, ptwsh_vals uint64
	var ptap_outs, ptap_vals uint64
	var best SortedWalletAddrs
	var cnt int = 15
	var mode int

	if !common.GetBool(&common.WalletON) {
		fmt.Println("Wallet functionality is currently disabled.")
		return
	}

	if par != "" {
		prs := strings.SplitN(par, " ", 2)
		if len(prs) > 0 {
			if c, e := strconv.ParseUint(prs[0], 10, 32); e == nil {
				if c > 4 {
					cnt = int(c)
				} else {
					mode = int(c + 1)
					fmt.Println("Counting only addr type", ([]string{"P2KH", "P2SH", "P2WKH", "P2WSH", "P2TAP"})[int(c)])
					if len(prs) > 1 {
						if c, e := strconv.ParseUint(prs[1], 10, 32); e == nil {
							cnt = int(c)
						}
					}
				}
			} else {
				fmt.Println("Specify the address type or/and number of top records to display")
				fmt.Println("Valid address types: 0-P2K, 1-P2SH, 2-P2WKH, 3-P2WSH, 4-P2TAP")
				return
			}
		}
	}

	var MIN_BTC uint64 = 100e8
	var MIN_OUTS int = 1000

	if mode != 0 {
		MIN_BTC = 0
		MIN_OUTS = 0
	}

	if mode == 0 || mode == 1 {
		for k, rec := range wallet.AllBalancesP2KH {
			ptkh_vals += rec.Value
			ptkh_outs += uint64(rec.Count())
			if sort_by_cnt && rec.Count() >= MIN_OUTS || !sort_by_cnt && rec.Value >= MIN_BTC {
				best = append(best, OneWalletAddrs{Typ: 0, Key: new_slice(k[:]), rec: rec})
			}
		}
		fmt.Println(btc.UintToBtc(ptkh_vals), "BTC in", ptkh_outs, "unspent recs from", len(wallet.AllBalancesP2KH), "P2KH addresses")
	}

	if mode == 0 || mode == 2 {
		for k, rec := range wallet.AllBalancesP2SH {
			ptsh_vals += rec.Value
			ptsh_outs += uint64(rec.Count())
			if sort_by_cnt && rec.Count() >= MIN_OUTS || !sort_by_cnt && rec.Value >= MIN_BTC {
				best = append(best, OneWalletAddrs{Typ: 1, Key: new_slice(k[:]), rec: rec})
			}
		}
		fmt.Println(btc.UintToBtc(ptsh_vals), "BTC in", ptsh_outs, "unspent recs from", len(wallet.AllBalancesP2SH), "P2SH addresses")
	}

	if mode == 0 || mode == 3 {
		for k, rec := range wallet.AllBalancesP2WKH {
			ptwkh_vals += rec.Value
			ptwkh_outs += uint64(rec.Count())
			if sort_by_cnt && rec.Count() >= MIN_OUTS || !sort_by_cnt && rec.Value >= MIN_BTC {
				best = append(best, OneWalletAddrs{Typ: 2, Key: new_slice(k[:]), rec: rec})
			}
		}
		fmt.Println(btc.UintToBtc(ptwkh_vals), "BTC in", ptwkh_outs, "unspent recs from", len(wallet.AllBalancesP2WKH), "P2WKH addresses")
	}

	if mode == 0 || mode == 4 {
		for k, rec := range wallet.AllBalancesP2WSH {
			ptwsh_vals += rec.Value
			ptwsh_outs += uint64(rec.Count())
			if sort_by_cnt && rec.Count() >= MIN_OUTS || !sort_by_cnt && rec.Value >= MIN_BTC {
				best = append(best, OneWalletAddrs{Typ: 2, Key: new_slice(k[:]), rec: rec})
			}
		}
		fmt.Println(btc.UintToBtc(ptwsh_vals), "BTC in", ptwsh_outs, "unspent recs from", len(wallet.AllBalancesP2WSH), "P2WSH addresses")
	}

	if mode == 0 || mode == 5 {
		for k, rec := range wallet.AllBalancesP2TAP {
			ptap_vals += rec.Value
			ptap_outs += uint64(rec.Count())
			if sort_by_cnt && rec.Count() >= MIN_OUTS || !sort_by_cnt && rec.Value >= MIN_BTC {
				best = append(best, OneWalletAddrs{Typ: 3, Key: new_slice(k[:]), rec: rec})
			}
		}
		fmt.Println(btc.UintToBtc(ptap_vals), "BTC in", ptap_outs, "unspent recs from", len(wallet.AllBalancesP2TAP), "P2TAP addresses")
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
		switch best[i].Typ {
		case 0:
			copy(pkscr_p2kh[3:23], best[i].Key)
			ad = btc.NewAddrFromPkScript(pkscr_p2kh[:], common.CFG.Testnet)
		case 1:
			copy(pkscr_p2sk[2:22], best[i].Key)
			ad = btc.NewAddrFromPkScript(pkscr_p2sk[:], common.CFG.Testnet)
		case 2, 3:
			ad = new(btc.BtcAddr)
			ad.SegwitProg = new(btc.SegwitProg)
			ad.SegwitProg.HRP = btc.GetSegwitHRP(common.CFG.Testnet)
			ad.SegwitProg.Program = best[i].Key
			ad.SegwitProg.Version = best[i].Typ - 2
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
			network.TxMutex.Lock()
			bidx, spending := network.SpentOutputs[unsp[i].TxPrevOut.UIdx()]
			var t2s *network.OneTxToSend
			if spending {
				t2s, spending = network.TransactionsToSend[bidx]
			}
			network.TxMutex.Unlock()
			if spending {
				fmt.Println("\t- being spent by TxID", t2s.Hash.String())
			}
		}
	}

	network.TxMutex.Lock()
	for _, t2s := range network.TransactionsToSend {
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
	network.TxMutex.Unlock()

	if !addr_printed {
		fmt.Println(ad.String(), "has no coins")
	}
}

func list_unspent(addr string) {
	if !common.GetBool(&common.WalletON) {
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
	if !common.GetBool(&common.WalletON) {
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

	if common.GetBool(&common.WalletON) {
		fmt.Println("Wallet functionality is currently ENABLED. Execute 'wallet off' to disable it.")
		fmt.Println("")
	} else {
		if perc := common.GetUint32(&common.WalletProgress); perc != 0 {
			fmt.Println("Enabling wallet functionality -", (perc-1)/10, "percent complete. Execute 'wallet off' to abort it.")
		} else {
			fmt.Println("Wallet functionality is currently DISABLED. Execute 'wallet on' to enable it.")
		}
	}

	if pend := common.GetUint32(&common.WalletOnIn); pend > 0 {
		fmt.Println("Wallet functionality will auto enable in", pend, "seconds")
	}
}

func init() {
	newUi("richest r", true, best_val, "Show addresses with most coins [0,1,2,3 or count]")
	newUi("maxouts o", true, max_outs, "Show addresses with highest number of outputs [0,1,2,3 or count]")
	newUi("balance a", true, list_unspent, "List balance of given bitcoin address (or raw public key)")
	newUi("allbal ab", true, all_val_stats, "Show Allbalance statistics")
	newUi("wallet w", false, wallet_on_off, "Enable (on) or disable (off) wallet functionality")
}
