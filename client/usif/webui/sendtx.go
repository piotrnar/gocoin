package webui

import (
	"fmt"
	"html"
	"bytes"
	"strings"
	"strconv"
	"net/http"
	"archive/zip"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/wallet"
)


const (
	AvgSignatureSize = 73
	AvgPublicKeySize = 34 /*Assumine compressed key*/
)


func dl_payment(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	var err string

	if len(r.Form["outcnt"])==1 {
		var thisbal btc.AllUnspentTx
		var pay_cmd string
		var totalinput, spentsofar uint64
		var change_addr *btc.BtcAddr
		var multisig_input []*wallet.MultisigAddr

		addrs_to_msign := make(map[string]bool)

		tx := new(btc.Tx)
		tx.Version = 1
		tx.Lock_time = 0

		outcnt, _ := strconv.ParseUint(r.Form["outcnt"][0], 10, 32)

		wallet.LockBal()
		for i:=1; i<=int(outcnt); i++ {
			is := fmt.Sprint(i)
			if len(r.Form["txout"+is])==1 && r.Form["txout"+is][0]=="on" {
				hash := btc.NewUint256FromString(r.Form["txid"+is][0])
				if hash!=nil {
					vout, er := strconv.ParseUint(r.Form["txvout"+is][0], 10, 32)
					if er==nil {
						var po = btc.TxPrevOut{Hash:hash.Hash, Vout:uint32(vout)}
						for j := range wallet.MyBalance {
							if wallet.MyBalance[j].TxPrevOut==po {
								thisbal = append(thisbal, wallet.MyBalance[j])

								// Add the input to our tx
								tin := new(btc.TxIn)
								tin.Input = wallet.MyBalance[j].TxPrevOut
								tin.Sequence = 0xffffffff
								tx.TxIn = append(tx.TxIn, tin)

								// Add new multisig address description
								_, msi := wallet.IsMultisig(wallet.MyBalance[j].BtcAddr)
								multisig_input = append(multisig_input, msi)
								if msi != nil {
									for ai := range msi.ListOfAddres {
										addrs_to_msign[msi.ListOfAddres[ai]] = true
									}
								}

								// Add the value to total input value
								totalinput += wallet.MyBalance[j].Value

								// If no change specified, use the first input addr as it
								if change_addr == nil {
									change_addr = wallet.MyBalance[j].BtcAddr
								}
							}
						}
					}
				}
			}
		}
		wallet.UnlockBal()

		for i:=1; ; i++ {
			adridx := fmt.Sprint("adr", i)
			btcidx := fmt.Sprint("btc", i)

			if len(r.Form[adridx])!=1 || len(r.Form[btcidx])!=1 {
				break
			}

			if len(r.Form[adridx][0])>1 {
				addr, er := btc.NewAddrFromString(r.Form[adridx][0])
				if er == nil {
					am, er := btc.StringToSatoshis(r.Form[btcidx][0])
					if er==nil && am>0 {
						if pay_cmd=="" {
							pay_cmd = "wallet -useallinputs -send "
						} else {
							pay_cmd += ","
						}
						pay_cmd += addr.Enc58str + "=" + btc.UintToBtc(am)

						tout := new(btc.TxOut)
						tout.Value = am
						tout.Pk_script = addr.OutScript()
						tx.TxOut = append(tx.TxOut, tout)

						spentsofar += am
					} else {
						err = "Incorrect amount (" + r.Form[btcidx][0] + ") for Output #" + fmt.Sprint(i)
						goto error
					}
				} else {
					err = "Incorrect address (" + r.Form[adridx][0] + ") for Output #" + fmt.Sprint(i)
					goto error
				}
			}
		}

		if pay_cmd=="" {
			err = "No inputs selected"
			goto error
		}

		am, er := btc.StringToSatoshis(r.Form["txfee"][0])
		if er != nil {
			err = "Incorrect fee value: " + r.Form["txfee"][0]
			goto error
		}

		pay_cmd += " -fee " + r.Form["txfee"][0]
		spentsofar += am

		if len(r.Form["change"][0])>1 {
			addr, er := btc.NewAddrFromString(r.Form["change"][0])
			if er != nil {
				err = "Incorrect change address: " + r.Form["change"][0]
				goto error
			}
			change_addr = addr
		}
		pay_cmd += " -change " + change_addr.String()

		if totalinput > spentsofar {
			// Add change output
			tout := new(btc.TxOut)
			tout.Value = totalinput - spentsofar
			tout.Pk_script = change_addr.OutScript()
			tx.TxOut = append(tx.TxOut, tout)
		}

		buf := new(bytes.Buffer)
		zi := zip.NewWriter(buf)

		was_tx := make(map [[32]byte] bool, len(thisbal))
		for i := range thisbal {
			if was_tx[thisbal[i].TxPrevOut.Hash] {
				continue
			}
			was_tx[thisbal[i].TxPrevOut.Hash] = true
			txid := btc.NewUint256(thisbal[i].TxPrevOut.Hash[:])
			fz, _ := zi.Create("balance/" + txid.String() + ".tx")
			wallet.GetRawTransaction(thisbal[i].MinedAt, txid, fz)
		}

		fz, _ := zi.Create("balance/unspent.txt")
		for i := range thisbal {
			fmt.Fprintf(fz, "%s # %.8f BTC @ %s, %d confs\n", thisbal[i].TxPrevOut.String(),
				float64(thisbal[i].Value)/1e8, thisbal[i].BtcAddr.StringLab(),
				1+common.Last.Block.Height-thisbal[i].MinedAt)
		}

		if len(addrs_to_msign) > 0 {
			// Multisig (or mixed) transaction ...
			for i := range multisig_input {
				if multisig_input[i] == nil {
					continue
				}
				d, er := hex.DecodeString(multisig_input[i].RedeemScript)
				if er != nil {
					println("ERROR parsing hex RedeemScript:", er.Error())
					continue
				}
				ms, er := btc.NewMultiSigFromP2SH(d)
				if er != nil {
					println("ERROR parsing bin RedeemScript:", er.Error())
					continue
				}
				tx.TxIn[i].ScriptSig = ms.Bytes()
			}
			fz, _ = zi.Create("multi2sign.txt")
			fz.Write([]byte(hex.EncodeToString(tx.Serialize())))

			fz, _ = zi.Create("multi_" + common.CFG.PayCommandName)
			for k, _ := range addrs_to_msign {
				fmt.Fprintln(fz, "wallet -msign", k, " -raw ...")
			}
		} else {
			// Non-multisig transaction ...
			fz, _ = zi.Create("tx2sign.txt")
			fz.Write([]byte(hex.EncodeToString(tx.Serialize())))

			if pay_cmd!="" {
				fz, _ = zi.Create(common.CFG.PayCommandName)
				fz.Write([]byte(pay_cmd))
			}
		}

		zi.Close()
		w.Header()["Content-Type"] = []string{"application/zip"}
		w.Write(buf.Bytes())
		return
	} else {
		err = "Bad request"
	}
error:
	s := load_template("send_error.html")
	write_html_head(w, r)
	s = strings.Replace(s, "<!--ERROR_MSG-->", err, 1)
	w.Write([]byte(s))
	write_html_tail(w)
}


func p_snd(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	s := load_template("send.html")

	wallet.LockBal()
	if wallet.MyWallet!=nil && len(wallet.MyBalance)>0 {
		wal := load_template("send_wal.html")
		row_tmp := load_template("send_wal_row.html")
		wal = strings.Replace(wal, "{TOTAL_BTC}", fmt.Sprintf("%.8f", float64(wallet.LastBalance)/1e8), 1)
		wal = strings.Replace(wal, "{UNSPENT_OUTS}", fmt.Sprint(len(wallet.MyBalance)), -1)
		for i := range wallet.MyBalance {
			row := row_tmp
			row = strings.Replace(row, "{WALLET_FILE}", html.EscapeString(wallet.MyBalance[i].BtcAddr.Extra.Wallet), 1)
			lab := wallet.MyBalance[i].BtcAddr.Extra.Label
			if wallet.MyBalance[i].BtcAddr.Extra.Virgin {
				lab += " ***"
			}

			var estimated_sig_size uint
			ms, msr := wallet.IsMultisig(wallet.MyBalance[i].BtcAddr)
			if ms {
				if msr != nil {
					estimated_sig_size = msr.KeysRequired*AvgSignatureSize + msr.KeysProvided*AvgPublicKeySize
				} else {
					estimated_sig_size = 2*AvgSignatureSize + 3*AvgPublicKeySize
				}
			} else {
				estimated_sig_size = AvgSignatureSize + AvgPublicKeySize
			}


			row = strings.Replace(row, "{ADDR_LABEL}", html.EscapeString(lab), 1)
			row = strings.Replace(row, "{ROW_NUMBER}", fmt.Sprint(i+1), -1)
			row = strings.Replace(row, "{MINED_IN}", fmt.Sprint(wallet.MyBalance[i].MinedAt), 1)
			row = strings.Replace(row, "{TX_ID}", btc.NewUint256(wallet.MyBalance[i].TxPrevOut.Hash[:]).String(), -1)
			row = strings.Replace(row, "{TX_VOUT}", fmt.Sprint(wallet.MyBalance[i].TxPrevOut.Vout), -1)
			row = strings.Replace(row, "{TX_SIGSIZ}", fmt.Sprint(estimated_sig_size), -1)
			row = strings.Replace(row, "{BTC_AMOUNT}", fmt.Sprintf("%.8f", float64(wallet.MyBalance[i].Value)/1e8), 1)
			row = strings.Replace(row, "{OUT_VALUE}", fmt.Sprint(wallet.MyBalance[i].Value), 1)
			row = strings.Replace(row, "{BTC_ADDR}", wallet.MyBalance[i].BtcAddr.String(), 1)
			wal = templ_add(wal, "<!--UTXOROW-->", row)
		}

		// Own wallet
		for i := range wallet.MyWallet.Addrs {
			row := "wallet.push({'addr':'" + wallet.MyWallet.Addrs[i].Enc58str + "', " +
				"'label':'" + wallet.MyWallet.Addrs[i].Extra.Label + "', " +
				"'wallet':'" + wallet.MyWallet.Addrs[i].Extra.Wallet + "', " +
				"'virgin':" + fmt.Sprint(wallet.MyWallet.Addrs[i].Extra.Virgin) + "})\n"
			wal = templ_add(wal, "/*WALLET_ENTRY_JS*/", row)
		}

		wal = strings.Replace(wal, "/*WALLET_ENTRY_JS*/", "const ADDR_LIST_SIZE = " + fmt.Sprint(common.CFG.WebUI.AddrListLen), 1)

		s = strings.Replace(s, "<!--WALLET-->", wal, 1)
	} else {
		if wallet.MyWallet==nil {
			s = strings.Replace(s, "<!--WALLET-->", "You have no wallet", 1)
		} else {
			s = strings.Replace(s, "<!--WALLET-->", "Your current wallet is empty", 1)
		}
	}
	wallet.UnlockBal()

	write_html_head(w, r)
	w.Write([]byte(s))
	write_html_tail(w)
}
