package webui

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io/ioutil"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/usif"
	"github.com/piotrnar/gocoin/client/wallet"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/utxo"
)

const (
	AT_P2PKH       = "P2PKH"
	AT_P2SH        = "P2SH"
	AT_P2WSH       = "P2WSH"
	AT_P2SH_P2WPKH = "P2SH-P2WPKH"
	AT_P2WPKH      = "P2WPKH"
	AT_P2TAP       = "P2TAP"
)

func p_wal(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	if !common.Get(&common.WalletON) {
		p_wallet_is_off(w, r)
		return
	}

	var str string
	common.Last.Mutex.Lock()
	if common.BlockChain.Consensus.Enforce_SEGWIT != 0 &&
		common.Last.Block.Height >= common.BlockChain.Consensus.Enforce_SEGWIT {
		str = "var segwit_active=true"
	} else {
		str = "var segwit_active=false"
	}
	common.Last.Mutex.Unlock()
	page := load_template("wallet.html")
	page = strings.Replace(page, "/*WALLET_JS_VARS*/", str, 1)
	write_html_head(w, r)
	w.Write([]byte(page))
	write_html_tail(w)
}

func getaddrtype(aa *btc.BtcAddr) string {
	if aa.SegwitProg != nil {
		if aa.SegwitProg.Version == 0 && len(aa.SegwitProg.Program) == 20 {
			return AT_P2WPKH
		}
		if aa.SegwitProg.Version == 1 && len(aa.SegwitProg.Program) == 32 {
			return AT_P2TAP
		}
	}
	if aa.Version == btc.AddrVerPubkey(common.Testnet) {
		return AT_P2PKH
	}
	if aa.Version == btc.AddrVerScript(common.Testnet) {
		return "P2SH"
	}
	return "unknown"
}

func json_balance(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) || !common.Get(&common.WalletON) {
		return
	}

	if r.Method != "POST" {
		return
	}

	summary := len(r.Form["summary"]) > 0
	mempool := len(r.Form["mempool"]) > 0
	getrawtx := len(r.Form["rawtx"]) > 0

	inp, er := ioutil.ReadAll(r.Body)
	if er != nil {
		println(er.Error())
		return
	}

	var addrs []string
	er = json.Unmarshal(inp, &addrs)
	if er != nil {
		println(er.Error())
		return
	}

	type OneOut struct {
		TxId     string
		Vout     uint32
		Value    uint64
		Height   uint32
		Coinbase bool
		Message  string
		Addr     string
		AddrType string
		Spending bool   // if true the spending tx is in the mempool
		RawTx    string `json:",omitempty"`
	}

	type OneOuts struct {
		Value            uint64
		OutCnt           int
		SegWitCnt        int
		SegWitAddr       string
		SegWitNativeCnt  int
		SegWitNativeAddr string
		SegWitTapCnt     int
		SegWitTapAddr    string
		Outs             []OneOut

		PendingCnt   int
		PendingValue uint64
		PendingOuts  []OneOut

		SpendingValue uint64
		SpendingCnt   uint64
	}

	out := make(map[string]*OneOuts)

	lck := new(usif.OneLock)
	lck.In.Add(1)
	lck.Out.Add(1)
	usif.LocksChan <- lck
	lck.In.Wait()

	var addr_map map[string]string

	if mempool {
		// make addrs -> idx
		addr_map = make(map[string]string, 2*len(addrs))
	}

	for _, a := range addrs {
		var aa, ab *btc.BtcAddr
		var e error
		var pubkey []byte
		var as string

		if len(a) == 66 && a[0] == '0' && (a[1] == '2' || a[1] == '3') {
			pubkey, e = hex.DecodeString(a) // raw public key
			if e != nil || len(pubkey) != 33 {
				continue
			}
			if aa = btc.NewAddrFromPubkey(pubkey, btc.AddrVerPubkey(common.Testnet)); aa == nil {
				continue
			}
		} else {
			// bitcoin address (of some sort)
			if aa, e = btc.NewAddrFromString(a); e != nil {
				continue
			}
		}

		unsp := wallet.GetAllUnspent(aa)
		newrec := new(OneOuts)
		if len(unsp) > 0 {
			newrec.OutCnt = len(unsp)
			as = aa.String()
			for _, u := range unsp {
				newrec.Value += u.Value
				network.TxMutex.Lock()
				_, spending := network.SpentOutputs[u.TxPrevOut.UIdx()]
				network.TxMutex.Unlock()
				if spending {
					newrec.SpendingValue += u.Value
					newrec.SpendingCnt++
				}
				if !summary {
					txid := btc.NewUint256(u.TxPrevOut.Hash[:])
					var rawtx string
					if getrawtx {
						dat, er := common.GetRawTx(uint32(u.MinedAt), txid)
						if er == nil {
							rawtx = hex.EncodeToString(dat)
						}
					}
					newrec.Outs = append(newrec.Outs, OneOut{
						TxId: btc.NewUint256(u.TxPrevOut.Hash[:]).String(), Vout: u.Vout,
						Value: u.Value, Height: u.MinedAt, Coinbase: u.Coinbase,
						Message: html.EscapeString(string(u.Message)), Addr: as, Spending: spending,
						RawTx: rawtx, AddrType: getaddrtype(aa)})
				}
			}
		}

		out[a] = newrec

		if mempool {
			addr_map[string(aa.OutScript())] = a
		}

		/* For P2KH addr, we also check its segwit's P2SH-P2WPKH and Native P2WPKH */
		if aa.SegwitProg == nil && aa.Version == btc.AddrVerPubkey(common.Testnet) {
			p2kh := aa.Hash160

			// P2SH SegWit if applicable
			h160 := btc.Rimp160AfterSha256(append([]byte{0, 20}, p2kh[:]...))
			ab = btc.NewAddrFromHash160(h160[:], btc.AddrVerScript(common.Testnet))
			as = ab.String()
			newrec.SegWitAddr = as
			unsp = wallet.GetAllUnspent(ab)
			if len(unsp) > 0 {
				newrec.OutCnt += len(unsp)
				newrec.SegWitCnt = len(unsp)
				for _, u := range unsp {
					newrec.Value += u.Value
					network.TxMutex.Lock()
					_, spending := network.SpentOutputs[u.TxPrevOut.UIdx()]
					network.TxMutex.Unlock()
					if spending {
						newrec.SpendingValue += u.Value
						newrec.SpendingCnt++
					}
					if !summary {
						txid := btc.NewUint256(u.TxPrevOut.Hash[:])
						var rawtx string
						if getrawtx {
							dat, er := common.GetRawTx(uint32(u.MinedAt), txid)
							if er == nil {
								rawtx = hex.EncodeToString(dat)
							}
						}
						newrec.Outs = append(newrec.Outs, OneOut{
							TxId: txid.String(), Vout: u.Vout,
							Value: u.Value, Height: u.MinedAt, Coinbase: u.Coinbase,
							Message: html.EscapeString(string(u.Message)), Addr: as,
							Spending: spending, RawTx: rawtx, AddrType: AT_P2SH_P2WPKH})
					}
				}
			}
			if mempool {
				addr_map[string(ab.OutScript())] = a
			}

			// Native SegWit if applicable
			ab = btc.NewAddrFromPkScript(append([]byte{0, 20}, p2kh[:]...), common.Testnet)
			as = ab.String()
			newrec.SegWitNativeAddr = as
			unsp = wallet.GetAllUnspent(ab)
			if len(unsp) > 0 {
				newrec.OutCnt += len(unsp)
				newrec.SegWitNativeCnt = len(unsp)
				for _, u := range unsp {
					newrec.Value += u.Value
					network.TxMutex.Lock()
					_, spending := network.SpentOutputs[u.TxPrevOut.UIdx()]
					network.TxMutex.Unlock()
					if spending {
						newrec.SpendingValue += u.Value
						newrec.SpendingCnt++
					}
					if !summary {
						txid := btc.NewUint256(u.TxPrevOut.Hash[:])
						var rawtx string
						if getrawtx {
							dat, er := common.GetRawTx(uint32(u.MinedAt), txid)
							if er == nil {
								rawtx = hex.EncodeToString(dat)
							}
						}
						newrec.Outs = append(newrec.Outs, OneOut{
							TxId: txid.String(), Vout: u.Vout,
							Value: u.Value, Height: u.MinedAt, Coinbase: u.Coinbase,
							Message: html.EscapeString(string(u.Message)), Addr: as,
							Spending: spending, RawTx: rawtx, AddrType: AT_P2WPKH})
					}
				}
			}
			if mempool {
				addr_map[string(ab.OutScript())] = a
			}

			// Also Check PAY2TAP, if pubkey mode...
			if pubkey != nil {
				if ab = btc.NewAddrFromPubkey(pubkey, btc.AddrVerPubkey(common.Testnet)); ab == nil {
					continue
				}
				ab.SegwitProg = &btc.SegwitProg{HRP: btc.GetSegwitHRP(common.Testnet), Version: 1, Program: pubkey[1:]}
				as = ab.String()
				newrec.SegWitTapAddr = as
				unsp = wallet.GetAllUnspent(ab)
				if len(unsp) > 0 {
					newrec.OutCnt += len(unsp)
					newrec.SegWitTapCnt = len(unsp)
					for _, u := range unsp {
						newrec.Value += u.Value
						network.TxMutex.Lock()
						_, spending := network.SpentOutputs[u.TxPrevOut.UIdx()]
						network.TxMutex.Unlock()
						if spending {
							newrec.SpendingValue += u.Value
							newrec.SpendingCnt++
						}
						if !summary {
							txid := btc.NewUint256(u.TxPrevOut.Hash[:])
							var rawtx string
							if getrawtx {
								dat, er := common.GetRawTx(uint32(u.MinedAt), txid)
								if er == nil {
									rawtx = hex.EncodeToString(dat)
								}
							}
							newrec.Outs = append(newrec.Outs, OneOut{
								TxId: txid.String(), Vout: u.Vout,
								Value: u.Value, Height: u.MinedAt, Coinbase: u.Coinbase,
								Message: html.EscapeString(string(u.Message)), Addr: as,
								Spending: spending, RawTx: rawtx, AddrType: AT_P2TAP})
						}
					}
				}
				if mempool {
					addr_map[string(ab.OutScript())] = a
				}
			}
		}
	}

	// check memory pool
	if mempool {
		network.TxMutex.Lock()
		for _, t2s := range network.TransactionsToSend {
			for vo, to := range t2s.TxOut {
				if a, ok := addr_map[string(to.Pk_script)]; ok {
					var tpo btc.TxPrevOut
					tpo.Hash = t2s.Hash.Hash
					tpo.Vout = uint32(vo)
					newrec := out[a]
					newrec.PendingValue += to.Value
					newrec.PendingCnt++
					_, spending := network.SpentOutputs[tpo.UIdx()]
					if spending {
						newrec.SpendingValue += to.Value
						newrec.SpendingCnt++
					}
					if !summary {
						po := &btc.TxPrevOut{Hash: t2s.Hash.Hash, Vout: uint32(vo)}
						_, spending := network.SpentOutputs[po.UIdx()]
						newrec.PendingOuts = append(newrec.PendingOuts, OneOut{
							TxId: t2s.Hash.String(), Vout: uint32(vo),
							Value: to.Value, Spending: spending})
					}
				}
			}
		}
		network.TxMutex.Unlock()
	}

	lck.Out.Done()

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}

func dl_balance(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) || !common.Get(&common.WalletON) {
		return
	}

	if r.Method != "POST" {
		return
	}

	var addrs []string
	var labels []string

	if len(r.Form["addrcnt"]) != 1 {
		println("no addrcnt")
		return
	}
	addrcnt, _ := strconv.ParseUint(r.Form["addrcnt"][0], 10, 32)

	for i := 0; i < int(addrcnt); i++ {
		is := fmt.Sprint(i)
		if len(r.Form["addr"+is]) == 1 {
			addrs = append(addrs, r.Form["addr"+is][0])
			if len(r.Form["label"+is]) == 1 {
				labels = append(labels, r.Form["label"+is][0])
			} else {
				labels = append(labels, "")
			}
		}
	}

	var thisbal utxo.AllUnspentTx

	lck := new(usif.OneLock)
	lck.In.Add(1)
	lck.Out.Add(1)
	usif.LocksChan <- lck
	lck.In.Wait()

	for idx, a := range addrs {
		var pubkey []byte
		var e error
		var aa *btc.BtcAddr
		if len(a) == 66 && a[0] == '0' && (a[1] == '2' || a[1] == '3') {
			pubkey, e = hex.DecodeString(a) // raw public key
			if e != nil || len(pubkey) != 33 {
				continue
			}
			if aa = btc.NewAddrFromPubkey(pubkey, btc.AddrVerPubkey(common.Testnet)); aa == nil {
				continue
			}
		} else {
			// bitcoin address (of some sort)
			if aa, e = btc.NewAddrFromString(a); e != nil {
				continue
			}
		}

		aa.Extra.Label = labels[idx]
		newrecs := wallet.GetAllUnspent(aa)
		if len(newrecs) > 0 {
			thisbal = append(thisbal, newrecs...)
		}

		/* Segwit P2WPKH: */
		if aa.SegwitProg == nil && aa.Version == btc.AddrVerPubkey(common.Testnet) {
			var ab *btc.BtcAddr
			p2kh := aa.Hash160

			// P2SH SegWit if applicable
			h160 := btc.Rimp160AfterSha256(append([]byte{0, 20}, p2kh[:]...))
			ab = btc.NewAddrFromHash160(h160[:], btc.AddrVerScript(common.Testnet))
			newrecs = wallet.GetAllUnspent(ab)
			if len(newrecs) > 0 {
				thisbal = append(thisbal, newrecs...)
			}

			// Native SegWit if applicable
			ab = btc.NewAddrFromPkScript(append([]byte{0, 20}, p2kh[:]...), common.Testnet)
			newrecs = wallet.GetAllUnspent(ab)
			if len(newrecs) > 0 {
				thisbal = append(thisbal, newrecs...)
			}

			// Also Check PAY2TAP, if pubkey mode...
			if pubkey != nil {
				aa.SegwitProg = &btc.SegwitProg{HRP: btc.GetSegwitHRP(common.Testnet), Version: 1, Program: pubkey[1:]}
				newrecs = wallet.GetAllUnspent(aa)
				if len(newrecs) > 0 {
					thisbal = append(thisbal, newrecs...)
				}
			}
		}
	}
	lck.Out.Done()

	buf := new(bytes.Buffer)
	zi := zip.NewWriter(buf)
	was_tx := make(map[[32]byte]bool)

	sort.Sort(thisbal)
	for i := range thisbal {
		if was_tx[thisbal[i].TxPrevOut.Hash] {
			continue
		}
		was_tx[thisbal[i].TxPrevOut.Hash] = true
		txid := btc.NewUint256(thisbal[i].TxPrevOut.Hash[:])
		fz, _ := zi.Create("balance/" + txid.String() + ".tx")
		if dat, er := common.GetRawTx(thisbal[i].MinedAt, txid); er == nil {
			fz.Write(dat)
		} else {
			println(er.Error())
		}
	}

	fz, _ := zi.Create("balance/unspent.txt")
	for i := range thisbal {
		fmt.Fprintln(fz, thisbal[i].UnspentTextLine())
	}

	zi.Close()
	w.Header()["Content-Type"] = []string{"application/zip"}
	w.Write(buf.Bytes())

}

func json_wallet_status(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	var out struct {
		WalletON       bool
		WalletProgress uint32
		WalletOnIn     uint32
	}
	common.LockCfg()
	out.WalletON = common.WalletON
	out.WalletProgress = common.WalletProgress
	out.WalletOnIn = common.WalletOnIn
	common.UnlockCfg()

	bx, er := json.Marshal(out)
	if er == nil {
		w.Header()["Content-Type"] = []string{"application/json"}
		w.Write(bx)
	} else {
		println(er.Error())
	}
}
