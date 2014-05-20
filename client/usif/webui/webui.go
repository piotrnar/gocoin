package webui

import (
	"os"
	"fmt"
	"strings"
	"net/http"
	"io/ioutil"
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"github.com/piotrnar/gocoin/lib/others/ver"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/wallet"
)

var webuimenu = [][2]string {
	{"/", "Home"},
	{"/wal", "Wallet"},
	{"/snd", "MakeTx"},
	{"/net", "Network"},
	{"/txs", "Transactions"},
	{"/blocks", "Blocks"},
	{"/miners", "Miners"},
	{"/counts", "Counters"},
}

const htmlhead = `<script type="text/javascript" src="webui/gocoin.js"></script>
<link rel="stylesheet" href="webui/gocoin.css" type="text/css"></head><body>
<table align="center" width="990" cellpadding="0" cellspacing="0"><tr><td>
`

func ipchecker(r *http.Request) bool {
	if common.NetworkClosed {
		return false
	}
	var a,b,c,d uint32
	n, _ := fmt.Sscanf(r.RemoteAddr, "%d.%d.%d.%d", &a, &b, &c, &d)
	if n!=4 {
		return false
	}
	addr := (a<<24) | (b<<16) | (c<<8) | d
	for i := range common.WebUIAllowed {
		if (addr&common.WebUIAllowed[i].Mask)==common.WebUIAllowed[i].Addr {
			r.ParseForm()
			return true
		}
	}
	println("ipchecker:", r.RemoteAddr, "is blocked")
	return false
}


func load_template(fn string) string {
	dat, _ := ioutil.ReadFile("www/templates/"+fn)
	return string(dat)
}


func templ_add(tmpl string, id string, val string) string {
	return strings.Replace(tmpl, id, val+id, 1)
}


func p_webui(w http.ResponseWriter, r *http.Request) {
	pth := strings.SplitN(r.URL.Path[1:], "/", 3)
	if len(pth)==2 {
		dat, _ := ioutil.ReadFile("www/resources/"+pth[1])
		if len(dat)>0 {
			switch filepath.Ext(r.URL.Path) {
				case ".js": w.Header()["Content-Type"] = []string{"text/javascript"}
				case ".css": w.Header()["Content-Type"] = []string{"text/css"}
			}
			w.Write(dat)
		}
	}
}


func sid(r *http.Request) string {
	c, _ := r.Cookie("sid")
	if c != nil {
		return c.Value
	}
	return ""
}


func checksid(r *http.Request) bool {
	if len(r.Form["sid"])==0 {
		return false
	}
	if len(r.Form["sid"][0])<16 {
		return false
	}
	return r.Form["sid"][0]==sid(r)
}


func new_session_id(w http.ResponseWriter) (sessid string) {
	var sid [16]byte
	rand.Read(sid[:])
	sessid = hex.EncodeToString(sid[:])
	http.SetCookie(w, &http.Cookie{Name:"sid", Value:sessid})
	return
}


func write_html_head(w http.ResponseWriter, r *http.Request) {
	sessid := sid(r)
	if sessid=="" {
		sessid = new_session_id(w)
	}

	// Quick switch wallet
	if checksid(r) && len(r.Form["qwalsel"])>0 {
		wallet.LoadWallet(common.GocoinHomeDir + "wallet" + string(os.PathSeparator) + r.Form["qwalsel"][0])
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
		return
	}

	// If currently selected wallet is address book and we are not on the wallet page - switch to default
	if r.URL.Path!="/wal" && wallet.MyWallet!=nil &&
		strings.HasSuffix(wallet.MyWallet.FileName, string(os.PathSeparator) + wallet.AddrBookFileName) {
		wallet.LoadWallet(common.GocoinHomeDir + "wallet" + string(os.PathSeparator) + wallet.DefaultFileName)
		http.Redirect(w, r, r.URL.Path, http.StatusFound)
		return
	}

	s := load_template("page_head.html")
	s = strings.Replace(s, "{VERSION}", ver.SourcesTag, 1)
	s = strings.Replace(s, "{SESSION_ID}", sessid, 1)
	if r.URL.Path!="/" {
		s = strings.Replace(s, "{HELPURL}", "help#" + r.URL.Path[1:], 1)
	} else {
		s = strings.Replace(s, "{HELPURL}", "help", 1)
	}
	if common.Testnet {
		s = strings.Replace(s, "{TESTNET}", "Testnet ", 1)
	} else {
		s = strings.Replace(s, "{TESTNET}", "", 1)
	}
	for i := range webuimenu {
		var x string
		if i>0 && i<len(webuimenu)-1 {
			x = " | "
		}
		x += "<a "
		if r.URL.Path==webuimenu[i][0] {
			x += "class=\"menuat\" "
		}
		x += "href=\""+webuimenu[i][0]+"\">"+webuimenu[i][1]+"</a>"
		if i==len(webuimenu)-1 {
			s = strings.Replace(s, "{MENU_LEFT}", "", 1)
			s = strings.Replace(s, "{MENU_RIGHT}", x, 1)
		} else {
			s = strings.Replace(s, "{MENU_LEFT}", x+"{MENU_LEFT}", 1)
		}
	}

	// Quick wallet switch
	fis, er := ioutil.ReadDir(common.GocoinHomeDir+"wallet/")
	if er == nil {
		for i := range fis {
			if !fis[i].IsDir() && fis[i].Size()>1 && fis[i].Name()[0]!='.' && fis[i].Name()!=wallet.AddrBookFileName {
				var ow string
				if wallet.MyWallet!=nil && strings.HasSuffix(wallet.MyWallet.FileName,
					string(os.PathSeparator) + fis[i].Name()) {
					ow = "<option selected>"+fis[i].Name()+"</option>"
				} else {
					ow = "<option>"+fis[i].Name()+"</option>"
				}
				s = templ_add(s, "<!--QUICK_WALLET_SELECT-->", ow)
			}
		}
	}

	w.Write([]byte(s))
}

func write_html_tail(w http.ResponseWriter) {
	dat, _ := ioutil.ReadFile("webht/page_tail.html")
	w.Write(dat)
}

func p_help(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	fname := "help.html"
	if len(r.Form["topic"])>0 && len(r.Form["topic"][0])==4 {
		for i:=0; i<4; i++ {
			if r.Form["topic"][0][i]<'a' || r.Form["topic"][0][i]>'z' {
				goto broken_topic  // we only accept 4 locase characters
			}
		}
		fname = "help_" + r.Form["topic"][0] + ".html"
	}
broken_topic:

	page := load_template(fname)
	write_html_head(w, r)
	w.Write([]byte(page))
	write_html_tail(w)
}


func ServerThread(iface string) {
	http.HandleFunc("/webui/", p_webui)
	http.HandleFunc("/wal", p_wal)
	http.HandleFunc("/snd", p_snd)
	http.HandleFunc("/net", p_net)
	http.HandleFunc("/txs", p_txs)
	http.HandleFunc("/blocks", p_blocks)
	http.HandleFunc("/miners", p_miners)
	http.HandleFunc("/counts", p_counts)
	http.HandleFunc("/cfg", p_cfg)
	http.HandleFunc("/help", p_help)

	http.HandleFunc("/txs2s.xml", xml_txs2s)
	http.HandleFunc("/txsre.xml", xml_txsre)
	http.HandleFunc("/txw4i.xml", xml_txw4i)
	http.HandleFunc("/raw_tx", raw_tx)
	http.HandleFunc("/balance.xml", xml_balance)
	http.HandleFunc("/raw_balance", raw_balance)
	http.HandleFunc("/raw_net", raw_net)
	http.HandleFunc("/balance.zip", dl_balance)
	http.HandleFunc("/payment.zip", dl_payment)
	http.HandleFunc("/addrs.xml", xml_addrs)
	http.HandleFunc("/wallets.xml", xml_wallets)

	http.HandleFunc("/", p_home)

	http.ListenAndServe(iface, nil)
}
