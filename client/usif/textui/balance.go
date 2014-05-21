package textui

import (
	"os"
	"fmt"
	"sort"
	"bytes"
	"strconv"
	"io/ioutil"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/qdb"
	"github.com/piotrnar/gocoin/lib/chain"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/client/usif"
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
	var walk chain.FunctionWalkUnspent
	var unsp chain.AllUnspentTx

	if sa==nil {
		walk = func(db *qdb.DB, k qdb.KeyType, rec *chain.OneWalkRecord) (uint32) {
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
		walk = func(db *qdb.DB, k qdb.KeyType, rec *chain.OneWalkRecord) (uint32) {
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
	} else if fn == "-" {
		fmt.Println("Reloading wallet from", wallet.MyWallet.FileName)
		wallet.LoadWallet(wallet.MyWallet.FileName)
		fmt.Println("Dumping current wallet from", wallet.MyWallet.FileName)
	} else if fn != "" {
		fmt.Println("Switching to wallet from", fn)
		wallet.LoadWallet(fn)
	}

	if wallet.MyWallet==nil {
		fmt.Println("No wallet loaded")
		return
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
	if sa.Version!=btc.StealthAddressVersion(common.Testnet) {
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

	var unsp chain.AllUnspentTx

	common.BlockChain.Unspent.BrowseUTXO(true, func(db *qdb.DB, k qdb.KeyType, rec *chain.OneWalkRecord) (uint32) {
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

func arm_stealth(p string) {
	var buf, b2 [256]byte

	create := p!=""

	fmt.Print("Enter seed password of the stealth key (empty line to abort) : ")
	le := sys.ReadPassword(buf[:])
	if le<=0 {
		fmt.Println("Aborted")
		return
	}
	if create {
		fmt.Print("Re-enter the seed password : ")
		l := sys.ReadPassword(b2[:])
		if l!=le && !bytes.Equal(buf[:le], b2[:l]) {
			sys.ClearBuffer(buf[:le])
			sys.ClearBuffer(b2[:l])
			fmt.Println("The passwords you entered do not match")
			return
		}
	}

	nw := make([]byte, 32)
	btc.ShaHash(buf[:le], nw)  // seed
	sys.ClearBuffer(buf[:le])
	btc.ShaHash(nw, nw)        // 1st key
	wallet.ArmedStealthSecrets = append(wallet.ArmedStealthSecrets, nw)
	if create {
		fmt.Println("You have created a new stealth scan-key. Make sure to not forget this password!")
		pk := btc.PublicFromPrivate(nw, true)
		fmt.Println("Public hexdump:", hex.EncodeToString(pk))
		fmt.Println(" Go to your wallet machine and execute:")
		fmt.Println("   wallet -scankey", hex.EncodeToString(pk), "-prefix 0")
		fmt.Println("   (change the prefix to a different value if you want)")
	}
	fmt.Println("Stealth key number", len(wallet.ArmedStealthSecrets)-1, "has been stored in memory")
	fmt.Println("Reloading the current wallet...")
	usif.ExecUiReq(&usif.OneUiReq{Handler:func(p string) {
		wallet.LoadWallet(wallet.MyWallet.FileName)
	}})
	show_prompt = false
}

func listarmkeys(p string) {
	if p!="seed" {
		if len(wallet.StealthSecrets)>0 {
			fmt.Println("Persistent secret scan keys:")
			for i := range wallet.StealthSecrets {
				pk := btc.PublicFromPrivate(wallet.StealthSecrets[i], true)
				fmt.Print(" #", i, "  ", hex.EncodeToString(pk))
				if p=="addr" {
					fmt.Print("  ", btc.NewAddrFromPubkey(pk, btc.AddrVerPubkey(common.Testnet)).String())
				}
				fmt.Println()
			}
		} else {
			fmt.Println("You have no persistent secret scan keys")
		}
	}
	if p!="file" {
		if len(wallet.ArmedStealthSecrets)>0 {
			fmt.Println("Volatile secret scan keys:")
			for i := range wallet.ArmedStealthSecrets {
				pk := btc.PublicFromPrivate(wallet.ArmedStealthSecrets[i], true)
				fmt.Print(" #", i, "  ", hex.EncodeToString(pk))
				if p=="addr" {
					fmt.Print("  ", btc.NewAddrFromPubkey(pk, btc.AddrVerPubkey(common.Testnet)).String())
				}
				if p=="save" {
					fn := common.GocoinHomeDir + "wallet/stealth/" + hex.EncodeToString(pk)
					if fi, er := os.Stat(fn); er==nil && fi.Size()>=32 {
						fmt.Print("  already on disk")
					} else {
						ioutil.WriteFile(fn, wallet.ArmedStealthSecrets[i], 0600)
						fmt.Print("  saved")
					}
					sys.ClearBuffer(wallet.ArmedStealthSecrets[i])
				}
				fmt.Println()
			}
		} else {
			fmt.Println("You have no volatile secret scan keys")
		}
	}
	if p=="save" {
		wallet.ArmedStealthSecrets = nil
		wallet.FetchStealthKeys()
	}
}

func unarm_stealth(p string) {
	if len(wallet.ArmedStealthSecrets)==0 {
		fmt.Println("You have no armed seed keys")
		listarmkeys("")
		return
	}
	if (p=="*" || p=="all") {
		for i:=range wallet.ArmedStealthSecrets {
			sys.ClearBuffer(wallet.ArmedStealthSecrets[i])
		}
		wallet.ArmedStealthSecrets = nil
		fmt.Println("Removed all armed stealth keys")
		return
	}
	v, e := strconv.ParseUint(p, 10, 32)
	if e != nil {
		println(e.Error())
		fmt.Println("Specify a valid armed seed key index. Type 'armed seed' to list them.")
		return
	}
	if v >= uint64(len(wallet.ArmedStealthSecrets)) {
		fmt.Println("Specify a valid armed seed key index. Type 'armed seed' to list them.")
		return
	}
	sys.ClearBuffer(wallet.ArmedStealthSecrets[v])
	wallet.ArmedStealthSecrets = append(wallet.ArmedStealthSecrets[:v],
		wallet.ArmedStealthSecrets[v+1:len(wallet.ArmedStealthSecrets)]...)
	fmt.Println("Removed armed stealth key number", v)
}


func init() {
	newUi("arm", false, arm_stealth, "Arm the client with a private stealth secret. Add switch -c when creating a new key")
	newUi("armed", false, listarmkeys, "Show currently armed private stealth keys. Optional param: seed, file, addr, save")
	newUi("unarm ua", false, unarm_stealth, "Purge an armed private stealth secret from memory. Specify number or * for all")
	newUi("balance bal", true, show_balance, "Show & save balance of currently loaded or a specified wallet")
	newUi("balstat", true, show_balance_stats, "Show balance cache statistics")
	newUi("scan", true, scan_stealth, "Get balance of a stealth address")
	newUi("scan0", true, scan_all_stealth, "Get balance of a stealth address. Ignore the prefix")
	newUi("unspent u", true, list_unspent, "Shows unpent outputs for a given address")
	newUi("wallet wal", true, load_wallet, "Load wallet from given file (or re-load the last one) and display its addrs")
}
