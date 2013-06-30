package main

import (
	"fmt"
	"sort"
	"strings"
	"net/http"
	"io/ioutil"
	"path/filepath"
	"github.com/piotrnar/gocoin/btc"
)

var webuimenu = [][2]string {
	{"/", "Home"},
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

func load_template(fn string) string {
	dat, _ := ioutil.ReadFile("webht/"+fn)
	return string(dat)
}


func p_webui(w http.ResponseWriter, r *http.Request) {
	if len(strings.SplitN(r.URL.Path[1:], "/", 3))==2 {
		dat, _ := ioutil.ReadFile(r.URL.Path[1:])
		if len(dat)>0 {
			switch filepath.Ext(r.URL.Path) {
				case ".js": w.Header()["Content-Type"] = []string{"text/javascript"}
				case ".css": w.Header()["Content-Type"] = []string{"text/css"}
			}
			w.Write(dat)
		}
	}
}

func write_html_head(w http.ResponseWriter, r *http.Request) {
	s := load_template("page_head.html")
	s = strings.Replace(s, "{VERSION}", btc.SourcesTag, 1)
	if CFG.Testnet {
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
	w.Write([]byte(s))
}

func write_html_tail(w http.ResponseWriter) {
	dat, _ := ioutil.ReadFile("webht/page_tail.html")
	w.Write(dat)
}

func p_counts(w http.ResponseWriter, r *http.Request) {
	write_html_head(w, r)
	counter_mutex.Lock()
	ck := make([]string, 0)
	for k, _ := range Counter {
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
					case "rcvd": fmt.Fprintln(w, "Received messages")
					case "sbts": fmt.Fprintln(w, "Sent bytes")
					case "sent": fmt.Fprintln(w, "Sent messages")
					default: fmt.Fprintln(w, prv_)
				}
			}
			fmt.Fprintf(w, "<tr><td>%s</td><td>%d</td></tr>\n", ck[i][5:], Counter[ck[i]])
		} else {
			fmt.Fprintf(w, "<tr><td>%s</td><td>%d</td></tr>\n", ck[i], Counter[ck[i]])
		}
	}
	fmt.Fprint(w, "</table></table>")
	counter_mutex.Unlock()
	write_html_tail(w)
}

func webserver() {
	http.HandleFunc("/webui/", p_webui)
	http.HandleFunc("/net", p_net)
	http.HandleFunc("/txs", p_txs)
	http.HandleFunc("/blocks", p_blocks)
	http.HandleFunc("/miners", p_miners)
	http.HandleFunc("/counts", p_counts)

	http.HandleFunc("/txs2s.xml", xmp_txs2s)
	http.HandleFunc("/txsre.xml", xml_txsre)
	http.HandleFunc("/txw4i.xml", xml_txw4i)
	http.HandleFunc("/balance.xml", xml_balance)
	http.HandleFunc("/raw_balance", raw_balance)
	http.HandleFunc("/raw_net", raw_net)
	http.HandleFunc("/balance.zip", dl_balance)

	http.HandleFunc("/", p_home)

	http.ListenAndServe(CFG.WebUI, nil)
}
