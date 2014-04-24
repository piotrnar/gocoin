package webui

import (
	"os"
	"strconv"
	"net/http"
	"io/ioutil"
	"encoding/json"
	"runtime/debug"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/wallet"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/usif"
)

func p_cfg(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	common.LockCfg()
	defer common.UnlockCfg()

	if r.Method=="POST" {
		if len(r.Form["configjson"])>0 {
			e := json.Unmarshal([]byte(r.Form["configjson"][0]), &common.CFG)
			if e == nil {
				common.Reset()
			}
			if len(r.Form["save"])>0 {
				dat, _ := json.Marshal(&common.CFG)
				if dat != nil {
					ioutil.WriteFile(common.ConfigFile, dat, 0660)
				}
			}
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		if len(r.Form["walletdata"])>0 && len(r.Form["walletfname"])>0 {
			fn := r.Form["walletfname"][0]
			if fn=="" {
				fn = wallet.DefaultFileName
			}
			fn = common.GocoinHomeDir + "wallet" + string(os.PathSeparator) + fn
			ioutil.WriteFile(fn, []byte(r.Form["walletdata"][0]), 0660)
			wallet.LoadWallet(fn)
			http.Redirect(w, r, "/wal", http.StatusFound)
			return
		}

		if len(r.Form["shutdown"])>0 {
			usif.Exit_now = true
			w.Write([]byte("Your node should shut down soon"))
			return
		}

		return
	}

	// for any other GET we need a matching session-id
	if !checksid(r) {
		new_session_id(w)
		return
	}

	if len(r.Form["txponoff"])>0 {
		common.CFG.TXPool.Enabled = !common.CFG.TXPool.Enabled
		http.Redirect(w, r, "txs", http.StatusFound)
		return
	}

	if len(r.Form["txronoff"])>0 {
		common.CFG.TXRoute.Enabled = !common.CFG.TXRoute.Enabled
		http.Redirect(w, r, "txs", http.StatusFound)
		return
	}

	if len(r.Form["lonoff"])>0 {
		common.CFG.Net.ListenTCP = !common.CFG.Net.ListenTCP
		http.Redirect(w, r, "net", http.StatusFound)
		return
	}

	if len(r.Form["drop"])>0 {
		network.DropPeer(r.Form["drop"][0])
		http.Redirect(w, r, "net", http.StatusFound)
		return
	}

	if len(r.Form["savecfg"])>0 {
		dat, _ := json.Marshal(&common.CFG)
		if dat != nil {
			ioutil.WriteFile(common.ConfigFile, dat, 0660)
		}
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if len(r.Form["beepblock"])>0 {
		common.CFG.Beeps.NewBlock = !common.CFG.Beeps.NewBlock
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if len(r.Form["freemem"])>0 {
		debug.FreeOSMemory()
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if len(r.Form["mid"])>0 {
		v, e := strconv.ParseUint(r.Form["mid"][0], 10, 32)
		if e == nil && v < uint64(len(common.MinerIds)) {
			common.CFG.Beeps.MinerID = common.MinerIds[v][1]
		} else {
			common.CFG.Beeps.MinerID = ""
		}
		http.Redirect(w, r, "miners", http.StatusFound)
		return
	}

	if len(r.Form["qwalsel"])>0 {
		wallet.LoadWallet(common.GocoinHomeDir + "wallet" + string(os.PathSeparator) + r.Form["qwalsel"][0])
		return
	}

}
