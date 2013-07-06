package main

import (
	"net/http"
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
		CFG.ListenTCP = !CFG.ListenTCP
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
}
