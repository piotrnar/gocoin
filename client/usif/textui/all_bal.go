package textui

import (
	"fmt"
	"sort"
	"strconv"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/wallet"
)


type OneWalletAddrs struct {
	P2SH bool
	Key [20]byte
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
	var ptkh_outs, ptkh_vals, ptsh_outs, ptsh_vals uint64
	var best SortedWalletAddrs
	var cnt int = 15

	if par!="" {
		if c, e := strconv.ParseUint(par, 10, 32); e==nil {
			cnt = int(c)
		}
	}

	for k, rec := range wallet.AllBalancesP2SH {
		ptsh_vals += rec.Value
		ptsh_outs += uint64(rec.Count())
		if sort_by_cnt && rec.Count()>=1000 || !sort_by_cnt && rec.Value>=1000e8 {
			best = append(best, OneWalletAddrs{P2SH:true, Key:k, rec:rec})
		}
	}

	for k, rec := range wallet.AllBalancesP2KH {
		ptkh_vals += rec.Value
		ptkh_outs += uint64(rec.Count())
		if sort_by_cnt && rec.Count()>=1000 || !sort_by_cnt && rec.Value>=1000e8 {
			best = append(best, OneWalletAddrs{Key:k, rec:rec})
		}
	}

	fmt.Println(btc.UintToBtc(ptkh_vals), "BTC in", ptkh_outs, "unspent recs from", len(wallet.AllBalancesP2KH), "P2KH addresses")
	fmt.Println(btc.UintToBtc(ptsh_vals), "BTC in", ptsh_outs, "unspent recs from", len(wallet.AllBalancesP2SH), "P2SH addresses")

	if sort_by_cnt {
		fmt.Println("Addrs with at least 1000 inps:", len(best))
	} else {
		fmt.Println("Addrs with at least 1000 BTC:", len(best))
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

	for i:=0; i<len(best) && i<cnt; i++ {
		if best[i].P2SH {
			copy(pkscr_p2sk[2:22], best[i].Key[:])
			ad = btc.NewAddrFromPkScript(pkscr_p2sk[:], common.CFG.Testnet)
		} else {
			copy(pkscr_p2kh[3:23], best[i].Key[:])
			ad = btc.NewAddrFromPkScript(pkscr_p2kh[:], common.CFG.Testnet)
		}
		fmt.Println(i+1, ad.String(), btc.UintToBtc(best[i].rec.Value), "BTC in", best[i].rec.Count(), "inputs")
	}
}

func list_unspent(addr string) {
	fmt.Println("Checking unspent coins for addr", addr)

	ad, e := btc.NewAddrFromString(addr)
	if e != nil {
		println(e.Error())
		return
	}

	unsp := wallet.GetAllUnspent(ad)
	if len(unsp)==0 {
		fmt.Println(ad.String(), "has no coins")
	} else {
		var tot uint64
		sort.Sort(unsp)
		for i := range unsp {
			unsp[i].BtcAddr = nil // no need to print the address here
			tot += unsp[i].Value
		}
		fmt.Println(ad.String(), "has", btc.UintToBtc(tot), "BTC in", len(unsp), "records:")
		for i := range unsp {
			fmt.Println(unsp[i].String())
		}
	}
}

func init() {
	newUi("richest r", true, best_val, "Show addresses with most coins")
	newUi("maxouts o", true, max_outs, "Show addresses with highest number of outputs")
	newUi("balance a", true, list_unspent, "List balance of given bitcoin address")
}
