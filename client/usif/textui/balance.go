package textui

import (
	"os"
	"fmt"
	"sort"
	"bytes"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/qdb"
	"github.com/piotrnar/gocoin/others/sys"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/wallet"
)

func list_unspent(addr string) {
	fmt.Println("Checking unspent coins for addr", addr)
	var ad *btc.BtcAddr
	var e error
	ad, e = btc.NewAddrFromString(addr)
	if e != nil {
		println(e.Error())
		return
	}
	sa := ad.StealthAddr
	exp_scr := ad.OutScript()
	var walk btc.FunctionWalkUnspent
	var unsp btc.AllUnspentTx

	if sa==nil {
		walk = func(db *qdb.DB, k qdb.KeyType, rec *btc.OneWalkRecord) (uint32) {
			if bytes.Equal(rec.Script(), exp_scr) {
				unsp = append(unsp, rec.ToUnspent(ad))
			}
			return 0
		}
	} else {
		wallet.FetchStealthKeys()
		d := wallet.FindStealthSecret(sa)
		if d==nil {
			fmt.Println("No matching secret found in your wallet/stealth folder")
			return
		}
		walk = func(db *qdb.DB, k qdb.KeyType, rec *btc.OneWalkRecord) (uint32) {
			if !rec.IsStealthIdx() {
				return 0
			}
			fl, uo := wallet.CheckStealthRec(db, k, rec, ad, d, true)
			if uo!=nil {
				unsp = append(unsp, uo)
			}
			return fl
		}
	}
	common.BlockChain.Unspent.BrowseUTXO(false, walk)

	sort.Sort(unsp)
	var sum uint64
	for i := range unsp {
		if len(unsp)<200 {
			fmt.Println(unsp[i].String())
		}
		sum += unsp[i].Value
	}
	fmt.Printf("Total %.8f unspent BTC in %d outputs at address %s\n",
		float64(sum)/1e8, len(unsp), ad.String());
}

func load_wallet(fn string) {
	if fn=="." {
		fmt.Println("Default wallet from", common.GocoinHomeDir+"wallet/DEFAULT")
		wallet.LoadWallet(common.GocoinHomeDir+"wallet/DEFAULT")
	} else if fn != "" {
		fmt.Println("Switching to wallet from", fn)
		wallet.LoadWallet(fn)
	}

	if wallet.MyWallet==nil {
		fmt.Println("No wallet loaded")
		return
	}

	if fn == "-" {
		fmt.Println("Reloading wallet from", wallet.MyWallet.FileName)
		wallet.LoadWallet(wallet.MyWallet.FileName)
		fmt.Println("Dumping current wallet from", wallet.MyWallet.FileName)
	}

	for i := range wallet.MyWallet.Addrs {
		fmt.Println(" ", wallet.MyWallet.Addrs[i].String(), wallet.MyWallet.Addrs[i].Label())
	}
}


func show_balance(p string) {
	if p=="sum" {
		fmt.Print(wallet.DumpBalance(wallet.MyBalance, nil, false, true))
		return
	}
	if p!="" {
		fmt.Println("Using wallet from file", p, "...")
		wallet.LoadWallet(p)
	}

	if wallet.MyWallet==nil {
		println("You have no loaded wallet")
		return
	}

	if len(wallet.MyWallet.Addrs)==0 {
		println("Your loaded wallet has no addresses")
		return
	}

	fmt.Print(wallet.UpdateBalanceFolder())
	fmt.Println("Your balance data has been saved to the 'balance/' folder.")
	fmt.Println("You nend to move this folder to your wallet PC, to spend the coins.")
}


func show_balance_stats(p string) {
	println("CachedAddrs count:", len(wallet.CachedAddrs))
	println("CacheUnspentIdx count:", len(wallet.CacheUnspentIdx))
	println("CacheUnspent count:", len(wallet.CacheUnspent))
	println("StealthAddrs count:", len(wallet.StealthAdCache))
	println("StealthSecrets:", len(wallet.StealthSecrets))
	if p!="" {
		wallet.BalanceMutex.Lock()
		for i := range wallet.CacheUnspent {
			if len(wallet.CacheUnspent[i].AllUnspentTx)==0 {
				fmt.Printf("%5d) %s: empty\n", i, wallet.CacheUnspent[i].BtcAddr.String())
			} else {
				var val uint64
				for j:=range wallet.CacheUnspent[i].AllUnspentTx {
					val += wallet.CacheUnspent[i].AllUnspentTx[j].Value
				}
				fmt.Printf("%5d) %s: %s BTC in %d\n", i, wallet.CacheUnspent[i].BtcAddr.String(),
					btc.UintToBtc(val), len(wallet.CacheUnspent[i].AllUnspentTx))
			}
		}
		wallet.BalanceMutex.Unlock()
	}
}


func do_scan_stealth(p string, ignore_prefix bool) {
	ad, _ := btc.NewAddrFromString(p)
	if ad==nil {
		fmt.Println("Specify base58 encoded bitcoin address")
		return
	}

	sa := ad.StealthAddr
	if sa==nil {
		fmt.Println("Specify base58 encoded stealth address")
		return
	}
	if sa.Version!=btc.StealthAddressVersion(common.CFG.Testnet) {
		fmt.Println("Incorrect version of the stealth address")
		return
	}
	if len(sa.SpendKeys)!=1 {
		fmt.Println("Currently only single spend keys are supported. This address has", len(sa.SpendKeys))
		return
	}

	//fmt.Println("scankey", hex.EncodeToString(sa.ScanKey[:]))
	if ignore_prefix {
		sa.Prefix = []byte{0}
		fmt.Println("Ignoring Prefix inside the address")
	} else if len(sa.Prefix)==0 {
		fmt.Println("Prefix not present in the address")
	} else {
		fmt.Println("Prefix", sa.Prefix[0], hex.EncodeToString(sa.Prefix[1:]))
	}

	wallet.FetchStealthKeys()
	d := wallet.FindStealthSecret(sa)
	if d==nil {
		fmt.Println("No matching secret found in your wallet/stealth folder")
		return
	}
	defer sys.ClearBuffer(d)

	var unsp btc.AllUnspentTx

	common.BlockChain.Unspent.BrowseUTXO(true, func(db *qdb.DB, k qdb.KeyType, rec *btc.OneWalkRecord) (uint32) {
		if !rec.IsStealthIdx() {
			return 0
		}
		fl, uo := wallet.CheckStealthRec(db, k, rec, ad, d, true)
		if uo!=nil {
			unsp = append(unsp, uo)
		}
		return fl
	})

	sort.Sort(unsp)
	os.RemoveAll("balance")
	os.MkdirAll("balance/", 0770)
	utxt, _ := os.Create("balance/unspent.txt")
	fmt.Print(wallet.DumpBalance(unsp, utxt, true, false))
}


func scan_stealth(p string) {
	do_scan_stealth(p, false)
}

func scan_all_stealth(p string) {
	do_scan_stealth(p, true)
}

func init() {
	newUi("balance bal", true, show_balance, "Show & save balance of currently loaded or a specified wallet")
	newUi("balstat", true, show_balance_stats, "Show balance cache statistics")
	newUi("scan", true, scan_stealth, "Get balance of a stealth address")
	newUi("scan0", true, scan_all_stealth, "Get balance of a stealth address. Ignore the prefix")
	newUi("unspent u", true, list_unspent, "Shows unpent outputs for a given address")
	newUi("wallet wal", true, load_wallet, "Load wallet from given file (or re-load the last one) and display its addrs")
}
