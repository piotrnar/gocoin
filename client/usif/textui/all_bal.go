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
		return len(sk[a].rec.Unsp) > len(sk[b].rec.Unsp)
	}
	return sk[a].rec.Value&^wallet.VALUE_P2SH_BIT > sk[b].rec.Value&^wallet.VALUE_P2SH_BIT
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
	var tot_val, tot_inps, ptsh, ptsh_outs, ptsh_vals uint64
	var best SortedWalletAddrs
	var cnt int = 15

	if par!="" {
		if c, e := strconv.ParseUint(par, 10, 32); e==nil {
			cnt = int(c)
		}
	}

	wallet.BalanceMutex.Lock()
	defer wallet.BalanceMutex.Unlock()

	if len(wallet.AllBalances)==0 {
		fmt.Println("No balance data")
		return
	}

	for k, rec := range wallet.AllBalances {
		val := rec.Value&^wallet.VALUE_P2SH_BIT
		tot_val += val
		tot_inps += uint64(len(rec.Unsp))
		if (rec.Value&wallet.VALUE_P2SH_BIT)!=0 {
			ptsh++
			ptsh_outs += uint64(len(rec.Unsp))
			ptsh_vals += val
		}
		if sort_by_cnt {
			if len(rec.Unsp)>=1000 {
				best = append(best, OneWalletAddrs{Key:k, rec:rec})
			}
		} else {
			if val>=1000e8 {
				best = append(best, OneWalletAddrs{Key:k, rec:rec})
			}
		}
	}
	fmt.Println(btc.UintToBtc(tot_val), "BTC in", tot_inps, "unspent records from", len(wallet.AllBalances), "addresses")
	fmt.Println(btc.UintToBtc(ptsh_vals), "BTC in", ptsh_outs, "records from", ptsh, "P2SH addresses")

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
		if (best[i].rec.Value&wallet.VALUE_P2SH_BIT)!=0 {
			copy(pkscr_p2sk[2:22], best[i].Key[:])
			ad = btc.NewAddrFromPkScript(pkscr_p2sk[:], common.CFG.Testnet)
		} else {
			copy(pkscr_p2kh[3:23], best[i].Key[:])
			ad = btc.NewAddrFromPkScript(pkscr_p2kh[:], common.CFG.Testnet)
		}
		fmt.Println(i+1, ad.String(), btc.UintToBtc(best[i].rec.Value&^wallet.VALUE_P2SH_BIT),
			"BTC in", len(best[i].rec.Unsp), "inputs")
	}
}


func init() {
	newUi("richest r", true, best_val, "Show the richest addresses")
	newUi("maxouts o", true, max_outs, "Show addresses with bniggest number of outputs")
}
