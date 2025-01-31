package webui

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/network"
	"github.com/piotrnar/gocoin/client/peersdb"
	"github.com/piotrnar/gocoin/client/usif"
	"github.com/piotrnar/gocoin/client/wallet"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/sys"
)

func p_cfg(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	if common.CFG.WebUI.ServerMode {
		return
	}

	if r.Method == "POST" {
		if len(r.Form["configjson"]) > 0 {
			common.LockCfg()
			e := json.Unmarshal([]byte(r.Form["configjson"][0]), &common.CFG)
			if e == nil {
				common.Reset()
			}
			if len(r.Form["save"]) > 0 {
				common.SaveConfig()
			}
			common.UnlockCfg()
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}

		if len(r.Form["friends_file"]) > 0 {
			ioutil.WriteFile(common.GocoinHomeDir+"friends.txt", []byte(r.Form["friends_file"][0]), 0600)
			network.Mutex_net.Lock()
			network.NextConnectFriends = time.Now()
			network.Mutex_net.Unlock()
			http.Redirect(w, r, "/net", http.StatusFound)
			return
		}

		if len(r.Form["shutdown"]) > 0 {
			usif.Exit_now.Set()
			w.Write([]byte("Your node should shut down soon"))
			return
		}

		if len(r.Form["wallet"]) > 0 {
			if r.Form["wallet"][0] == "on" {
				wallet.OnOff <- true
			} else if r.Form["wallet"][0] == "off" {
				wallet.OnOff <- false
			}
			if len(r.Form["page"]) > 0 {
				http.Redirect(w, r, r.Form["page"][0], http.StatusFound)
			} else {
				http.Redirect(w, r, "/wal", http.StatusFound)
			}
			return
		}

		return
	}

	// for any other GET we need a matching session-id
	if !checksid(r) {
		new_session_id(w)
		return
	}

	if len(r.Form["getmp"]) > 0 {
		if conid, e := strconv.ParseUint(r.Form["getmp"][0], 10, 32); e == nil {
			network.GetMP(uint32(conid))
		}
		return
	}

	if len(r.Form["freemem"]) > 0 {
		sys.FreeMem()
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if len(r.Form["drop"]) > 0 {
		if conid, e := strconv.ParseUint(r.Form["drop"][0], 10, 32); e == nil {
			network.DropPeer(uint32(conid))
		}
		http.Redirect(w, r, "net", http.StatusFound)
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

	if len(r.Form["unban"]) > 0 {
		w.Write([]byte(usif.UnbanPeer(r.Form["unban"][0])))
		return
	}

	if len(r.Form["getconfig"]) > 0 {
		if !common.CFG.WebUI.ServerMode {
			common.LockCfg()
			dat, _ := json.MarshalIndent(&common.CFG, "", "    ")
			common.UnlockCfg()
			w.Write(dat)
		}
		return
	}

	if len(r.Form["getfriends"]) > 0 {
		if !common.CFG.WebUI.ServerMode {
			d, _ := os.ReadFile(common.GocoinHomeDir + "friends.txt")
			w.Write(d)
		}
		return
	}

	if len(r.Form["getauthkey"]) > 0 {
		w.Write([]byte(common.PublicKey))
		return
	}

	// All the functions below modify the config file
	common.LockCfg()
	defer common.UnlockCfg()

	if len(r.Form["txponoff"]) > 0 {
		common.CFG.TXPool.Enabled = !common.CFG.TXPool.Enabled
		w.Write([]byte(fmt.Sprint(common.CFG.TXPool.Enabled)))
		return
	}

	if len(r.Form["txronoff"]) > 0 {
		common.CFG.TXRoute.Enabled = !common.CFG.TXRoute.Enabled
		w.Write([]byte(fmt.Sprint(common.CFG.TXRoute.Enabled)))
		return
	}

	if len(r.Form["lonoff"]) > 0 {
		common.CFG.Net.ListenTCP = !common.CFG.Net.ListenTCP
		common.ListenTCP = common.CFG.Net.ListenTCP
		w.Write([]byte(fmt.Sprint(common.ListenTCP)))
		return
	}

	if len(r.Form["savecfg"]) > 0 {
		dat, _ := json.MarshalIndent(&common.CFG, "", "    ")
		if dat != nil {
			os.WriteFile(common.ConfigFile, dat, 0660)
		}
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}

	if len(r.Form["trusthash"]) > 0 {
		if btc.NewUint256FromString(r.Form["trusthash"][0]) != nil {
			common.CFG.LastTrustedBlock = r.Form["trusthash"][0]
			common.ApplyLastTrustedBlock()
		}
		w.Write([]byte(common.CFG.LastTrustedBlock))
		return
	}
}
