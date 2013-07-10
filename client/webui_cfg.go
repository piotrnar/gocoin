package main

import (
	"io/ioutil"
	"net/http"
	"encoding/json"
)

func p_cfg(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	if len(r.Form["txponoff"])>0 {
		CFG.TXPool.Enabled = !CFG.TXPool.Enabled
		http.Redirect(w, r, "txs", http.StatusFound)
		return
	}

	if len(r.Form["txronoff"])>0 {
		CFG.TXRoute.Enabled = !CFG.TXRoute.Enabled
		http.Redirect(w, r, "txs", http.StatusFound)
		return
	}

	if len(r.Form["lonoff"])>0 {
		CFG.Net.ListenTCP = !CFG.Net.ListenTCP
		http.Redirect(w, r, "net", http.StatusFound)
		return
	}

	if len(r.Form["drop"])>0 {
		net_drop(r.Form["drop"][0])
		http.Redirect(w, r, "net", http.StatusFound)
		return
	}

	if len(r.Form["freemem"])>0 {
		show_mem("free")
		http.Redirect(w, r, "/", http.StatusFound)
	}

	if len(r.Form["configjson"])>0 {
		e := json.Unmarshal([]byte(r.Form["configjson"][0]), &CFG)
		if e == nil {
			resetcfg()
		}
		if len(r.Form["save"])>0 {
			dat, _ := json.Marshal(&CFG)
			if dat != nil {
				ioutil.WriteFile(ConfigFile, dat, 0660)
			}
		}
		http.Redirect(w, r, "/", http.StatusFound)
	}

	if r.Method=="POST" && len(r.Form["shutdown"])>0 {
		exit_now = true
		w.Write([]byte("Your node should shut down soon"))
	}

	if len(r.Form["mid"])>0 {
		set_miner(r.Form["mid"][0])
		http.Redirect(w, r, "miners", http.StatusFound)
	}
}
