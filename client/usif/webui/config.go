package webui

import (
	"fmt"
	"time"
	"strconv"
	"net/http"
	"io/ioutil"
	"encoding/json"
	"github.com/piotrnar/gocoin/lib/others/sys"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/usif"
	"github.com/piotrnar/gocoin/lib/others/peersdb"
)

func p_cfg(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	if common.CFG.WebUI.ServerMode {
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
				common.SaveConfig()
			}
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		if len(r.Form["friends_file"]) > 0 {
			ioutil.WriteFile(common.GocoinHomeDir + "friends.txt", []byte(r.Form["friends_file"][0]), 0600)
			network.Mutex_net.Lock()
			network.NextConnectFriends = time.Now()
			network.Mutex_net.Unlock()
			http.Redirect(w, r, "/net", http.StatusFound)
			return
		}

		if len(r.Form["shutdown"])>0 {
			usif.Exit_now.Set()
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
		common.ListenTCP = common.CFG.Net.ListenTCP
		http.Redirect(w, r, "net", http.StatusFound)
		return
	}

	if len(r.Form["drop"])>0 {
		if conid, e := strconv.ParseUint(r.Form["drop"][0], 10, 32); e==nil {
			network.DropPeer(uint32(conid))
		}
		http.Redirect(w, r, "net", http.StatusFound)
		return
	}

	if len(r.Form["getmp"])>0 {
		if conid, e := strconv.ParseUint(r.Form["getmp"][0], 10, 32); e==nil {
			network.GetMP(uint32(conid))
		}
		return
	}

	if len(r.Form["conn"]) > 0 {
		ad, er := peersdb.NewAddrFromString(r.Form["conn"][0], false)
		if er != nil {
			w.Write([]byte(er.Error()))
			return
		}
		w.Write([]byte(fmt.Sprint("Connecting to ", ad.Ip())))
		ad.Manual = true
		network.DoNetwork(ad)
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

	if len(r.Form["freemem"])>0 {
		sys.FreeMem()
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
}
