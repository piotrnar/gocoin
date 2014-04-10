package webui

import (
	"os"
	"fmt"
	"sort"
	"strings"
	"net/http"
	"io/ioutil"
	"crypto/rand"
	"encoding/hex"
	"path/filepath"
	"github.com/piotrnar/gocoin/btc"
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


func write_html_head(w http.ResponseWriter, r *http.Request) {
	sessid := sid(r)
	if sessid=="" {
		var sid [16]byte
		rand.Read(sid[:])
		sessid = hex.EncodeToString(sid[:])
		http.SetCookie(w, &http.Cookie{Name:"sid", Value:sessid})
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
	s = strings.Replace(s, "{VERSION}", btc.SourcesTag, 1)
	s = strings.Replace(s, "{SESSION_ID}", sessid, 1)
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

func p_counts(w http.ResponseWriter, r *http.Request) {
	write_html_head(w, r)
	common.CounterMutex.Lock()
	ck := make([]string, 0)
	for k, _ := range common.Counter {
		ck = append(ck, k)
	}
	sort.Strings(ck)
	fmt.Fprint(w, "<table class=\"mono\"><tr>")
	fmt.Fprint(w, "<td valign=\"top\"><table class=\"bord\"><tr><th colspan=\"2\">Generic Counters")
	prv_ := ""
	for i := range ck {
		if ck[i][4]=='_' {
			if ck[i][:4]!=prv_ {
				prv_ = ck[i][:4]
				fmt.Fprint(w, "</table><td valign=\"top\"><table class=\"bord\"><tr><th colspan=\"2\">")
				switch prv_ {
					case "rbts": fmt.Fprintln(w, "Received bytes")
					case "rcvd": fmt.Fprintln(w, "Received msgs")
					case "sbts": fmt.Fprintln(w, "Sent bytes")
					case "sent": fmt.Fprintln(w, "Sent msgs")
					case "hbts": fmt.Fprintln(w, "Hold bytes")
					case "hold": fmt.Fprintln(w, "Hold msgs")
					default: fmt.Fprintln(w, prv_)
				}
			}
			fmt.Fprintf(w, "<tr><td>%s</td><td>%d</td></tr>\n", ck[i][5:], common.Counter[ck[i]])
		} else {
			fmt.Fprintf(w, "<tr><td>%s</td><td>%d</td></tr>\n", ck[i], common.Counter[ck[i]])
		}
	}
	fmt.Fprint(w, "</table></table>")
	common.CounterMutex.Unlock()
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

	http.HandleFunc("/", p_home)

	http.ListenAndServe(iface, nil)
}
