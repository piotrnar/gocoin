package webui

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/usif"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/utxo"
	"nhooyr.io/websocket/wsjson"

	rmtcmn "github.com/piotrnar/gocoin/remote-wallet/common"
	rmtsrv "github.com/piotrnar/gocoin/remote-wallet/server"
)

const (
	AvgSignatureSize = 73
	AvgPublicKeySize = 34 /*Assumine compressed key*/
    FormParseMemBufSize = 1000000
)

type MultisigAddr struct {
	MultiAddress               string
	ScriptPubKey               string
	KeysRequired, KeysProvided uint
	RedeemScript               string
	ListOfAddres               []string
}

func make_tx(form url.Values, tx *btc.Tx) (err string, thisbal utxo.AllUnspentTx, pay_cmd string){
    var totalinput, spentsofar uint64
    var change_addr *btc.BtcAddr

    tx.Version = 1
    tx.Lock_time = 0

    seq, er := strconv.ParseInt(form["tx_seq"][0], 10, 64)
    if er != nil || seq < -1000000 || seq > 0xffffffff {
        err = "Incorrect Sequence value: " + form["tx_seq"][0]
        return err, nil, ""
    }

    outcnt, _ := strconv.ParseUint(form["outcnt"][0], 10, 32)

    lck := new(usif.OneLock)
    lck.In.Add(1)
    lck.Out.Add(1)
    usif.LocksChan <- lck
    lck.In.Wait()
    defer lck.Out.Done()

    for i := 1; i <= int(outcnt); i++ {
        is := fmt.Sprint(i)
        if len(form["txout"+is]) == 1 && form["txout"+is][0] == "on" {
            hash := btc.NewUint256FromString(form["txid"+is][0])
            if hash != nil {
                vout, er := strconv.ParseUint(form["txvout"+is][0], 10, 32)
                if er == nil {
                    var po = btc.TxPrevOut{Hash: hash.Hash, Vout: uint32(vout)}
                    if res := common.BlockChain.Unspent.UnspentGet(&po); res != nil {
                        addr := btc.NewAddrFromPkScript(res.Pk_script, common.Testnet)

                        unsp := &utxo.OneUnspentTx{TxPrevOut: po, Value: res.Value,
                            MinedAt: res.BlockHeight, Coinbase: res.WasCoinbase, BtcAddr: addr}

                        thisbal = append(thisbal, unsp)

                        // Add the input to our tx
                        tin := new(btc.TxIn)
                        tin.Input = po
                        tin.Sequence = uint32(seq)
                        tx.TxIn = append(tx.TxIn, tin)

                        // Add the value to total input value
                        totalinput += res.Value

                        // If no change specified, use the first input addr as it
                        if change_addr == nil {
                            change_addr = addr
                        }
                    }
                }
            }
        }
    }

    if change_addr == nil {
        // There werte no inputs
        return
    }

    for i := 1; ; i++ {
        adridx := fmt.Sprint("adr", i)
        btcidx := fmt.Sprint("btc", i)

        if len(form[adridx]) != 1 || len(form[btcidx]) != 1 {
            break
        }

        if len(form[adridx][0]) > 1 {
            addr, er := btc.NewAddrFromString(form[adridx][0])
            if er == nil {
                am, er := btc.StringToSatoshis(form[btcidx][0])
                if er == nil && am > 0 {
                    if pay_cmd == "" {
                        pay_cmd = "wallet -a=false -useallinputs -send "
                    } else {
                        pay_cmd += ","
                    }
                    pay_cmd += addr.String() + "=" + btc.UintToBtc(am)

                    outs, er := btc.NewSpendOutputs(addr, am, common.CFG.Testnet)
                    if er != nil {
                        err = er.Error()
                        return err, nil, ""
                    }
                    tx.TxOut = append(tx.TxOut, outs...)

                    spentsofar += am
                } else {
                    err = "Incorrect amount (" + form[btcidx][0] + ") for Output #" + fmt.Sprint(i)
                    return err, nil, ""
                }
            } else {
                err = "Incorrect address (" + form[adridx][0] + ") for Output #" + fmt.Sprint(i)
                return err, nil, ""
            }
        }
    }

    if pay_cmd == "" {
        err = "No inputs selected"
        return err, nil, ""
    }

    pay_cmd += fmt.Sprint(" -seq ", seq)

    am, er := btc.StringToSatoshis(form["txfee"][0])
    if er != nil {
        err = "Incorrect fee value: " + form["txfee"][0]
        return err, nil, ""
    }

    pay_cmd += " -fee " + form["txfee"][0]
    spentsofar += am

    if len(form["change"][0]) > 1 {
        addr, er := btc.NewAddrFromString(form["change"][0])
        if er != nil {
            err = "Incorrect change address: " + form["change"][0]
            return err, nil, ""
        }
        change_addr = addr
    }
    pay_cmd += " -change " + change_addr.String()

    if totalinput > spentsofar {
        // Add change output
        outs, er := btc.NewSpendOutputs(change_addr, totalinput-spentsofar, common.CFG.Testnet)
        if er != nil {
            err = er.Error()
            return err, nil, ""
        }
        tx.TxOut = append(tx.TxOut, outs...)
    }
    return "", thisbal, pay_cmd
}

func sign_transaction(wrs *rmtsrv.WebsocketServer)func (w http.ResponseWriter, r *http.Request) {
    return func(w http.ResponseWriter, r *http.Request) {
        fmt.Println("Received sign transaction request here")
        r.ParseMultipartForm(FormParseMemBufSize)

        if !ipchecker(r) || !common.GetBool(&common.WalletON)  {
            return
        }

        var err string
        if(wrs.Conn == nil){
                err = "Remote wallet is not connected"
                goto error
        }

        if len(r.Form["outcnt"])==1 {
            tx := new(btc.Tx)
            er, thisbal, pay_cmd := make_tx(r.Form, tx)
            if err != "" {
                err = er
                goto error
            }

            st := rmtcmn.SignTransactionRequestPayload{}

            was_tx := make(map [[32]byte] bool, len(thisbal))
            for i := range thisbal {
                if was_tx[thisbal[i].TxPrevOut.Hash] {
                    continue
                }
                was_tx[thisbal[i].TxPrevOut.Hash] = true
                txid := btc.NewUint256(thisbal[i].TxPrevOut.Hash[:])
                if dat, er := common.GetRawTx(thisbal[i].MinedAt, txid); er == nil {
                    st.BalanceFileName = txid.String() + ".tx"
                    st.BalanceFileContents = hex.EncodeToString(dat)
                } else {
                    println(er.Error())
                }
            }

            b := bytes.NewBuffer(make([]byte, 0))
            for i := range thisbal {
                fmt.Fprintln(b, thisbal[i].UnspentTextLine())
            }
            st.Unspent = string(b.Bytes())

            if pay_cmd!="" {
                st.PayCmd = pay_cmd
            }

            // Non-multisig transaction ...
            st.Tx2Sign = fmt.Sprintf("%x", tx.Serialize())
            msg := rmtcmn.Msg{Type: rmtcmn.SignTransaction, Payload: st}
            
            error := wsjson.Write(context.Background(), wrs.Conn, msg)
            if error != nil {
                fmt.Println(error)
                err = error.Error()
                goto error
            }
            return
        } else {
            err = "Bad request"
            goto error
            }
    error:
        w.WriteHeader(http.StatusInternalServerError)
        w.Write([]byte(err))
        return 
    }
}

func dl_payment(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) || !common.GetBool(&common.WalletON) {
		return
	}

	var err string

	if len(r.Form["outcnt"]) == 1 {
        tx := new(btc.Tx)
        err, thisbal, pay_cmd := make_tx(r.Form, tx)
        if err != "" {
            goto error
        }

        buf := new(bytes.Buffer)
		zi := zip.NewWriter(buf)
		was_tx := make(map[[32]byte]bool, len(thisbal))
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
		if pay_cmd != "" {
			fz, _ = zi.Create(common.CFG.WebUI.PayCmdName)
			fz.Write([]byte(pay_cmd))
		}

		// Non-multisig transaction ...
		fz, _ = zi.Create("tx2sign.txt")
		fz.Write([]byte(hex.EncodeToString(tx.Serialize())))

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

	if !common.GetBool(&common.WalletON) {
		p_wallet_is_off(w, r)
		return
	}

	s := load_template("send.html")

	write_html_head(w, r)
	w.Write([]byte(s))
	write_html_tail(w)
}
