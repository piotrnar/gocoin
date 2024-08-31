package webui

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/piotrnar/gocoin"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/usif"
)

var start_time time.Time

func ipchecker(r *http.Request) bool {
	if common.NetworkClosed.Get() || usif.Exit_now.Get() {
		return false
	}

	if r.TLS != nil {
		r.ParseForm()
		return true
	}

	var a, b, c, d uint32
	n, _ := fmt.Sscanf(r.RemoteAddr, "%d.%d.%d.%d", &a, &b, &c, &d)
	if n != 4 {
		return false
	}
	addr := (a << 24) | (b << 16) | (c << 8) | d
	common.LockCfg()
	for i := range common.WebUIAllowed {
		if (addr & common.WebUIAllowed[i].Mask) == common.WebUIAllowed[i].Addr {
			common.UnlockCfg()
			r.ParseForm()
			return true
		}
	}
	common.UnlockCfg()
	println("ipchecker:", r.RemoteAddr, "is blocked")
	return false
}

func load_template(fn string) string {
	dat, er := ioutil.ReadFile("www/templates/" + fn)
	if er != nil {
		return er.Error() + "\n"
	}
	return string(dat)
}

func templ_add(tmpl string, id string, val string) string {
	return strings.Replace(tmpl, id, val+id, 1)
}

func p_webui(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	pth := strings.SplitN(r.URL.Path[1:], "/", 3)
	if len(pth) == 2 {
		dat, _ := ioutil.ReadFile("www/resources/" + pth[1])
		if len(dat) > 0 {
			switch filepath.Ext(r.URL.Path) {
			case ".js":
				w.Header()["Content-Type"] = []string{"text/javascript"}
			case ".css":
				w.Header()["Content-Type"] = []string{"text/css"}
			}
			w.Write(dat)
		} else {
			http.NotFound(w, r)
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
	if len(r.Form["sid"]) == 0 {
		return false
	}
	if len(r.Form["sid"][0]) < 16 {
		return false
	}
	return r.Form["sid"][0] == sid(r)
}

func new_session_id(w http.ResponseWriter) (sessid string) {
	var sid [16]byte
	rand.Read(sid[:])
	sessid = hex.EncodeToString(sid[:])
	http.SetCookie(w, &http.Cookie{Name: "sid", Value: sessid})
	return
}

func write_html_head(w http.ResponseWriter, r *http.Request) {
	start_time = time.Now()

	sessid := sid(r)
	if sessid == "" {
		sessid = new_session_id(w)
	}

	s := load_template("page_head.html")
	s = strings.Replace(s, "{PAGE_TITLE}", common.CFG.WebUI.Title, 1)
	s = strings.Replace(s, "/*_SESSION_ID_*/", "var sid = '"+sessid+"'", 1)
	s = strings.Replace(s, "/*_AVERAGE_FEE_SPB_*/", fmt.Sprint("var avg_fee_spb = ", usif.GetAverageFee()), 1)
	s = strings.Replace(s, "/*_SERVER_MODE_*/", fmt.Sprint("var server_mode = ", common.CFG.WebUI.ServerMode), 1)
	s = strings.Replace(s, "/*_TIME_NOW_*/", fmt.Sprint("= ", time.Now().Unix()), 1)
	s = strings.Replace(s, "/*_WALLET_ON_*/", fmt.Sprint("var wallet_on = ", common.GetBool(&common.WalletON)), 1)
	s = strings.Replace(s, "/*_CHAIN_IN_SYNC_*/", fmt.Sprint("var chain_in_sync = ", common.GetBool(&common.BlockChainSynchronized)), 1)

	if r.URL.Path != "/" {
		s = strings.Replace(s, "{HELPURL}", "help#"+r.URL.Path[1:], 1)
	} else {
		s = strings.Replace(s, "{HELPURL}", "help", 1)
	}
	s = strings.Replace(s, "{VERSION}", gocoin.Version, 1)
	if common.Testnet {
		s = strings.Replace(s, "{TESTNET}", " Testnet ", 1)
	} else {
		s = strings.Replace(s, "{TESTNET}", "", 1)
	}

	w.Write([]byte(s))
}

func write_html_tail(w http.ResponseWriter) {
	s := load_template("page_tail.html")
	s = strings.Replace(s, "<!--LOAD_TIME-->", time.Now().Sub(start_time).String(), 1)
	w.Write([]byte(s))
}

func p_help(w http.ResponseWriter, r *http.Request) {
	if !ipchecker(r) {
		return
	}

	fname := "help.html"
	if len(r.Form["topic"]) > 0 && len(r.Form["topic"][0]) == 4 {
		for i := 0; i < 4; i++ {
			if r.Form["topic"][0][i] < 'a' || r.Form["topic"][0][i] > 'z' {
				goto broken_topic // we only accept 4 locase characters
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

func p_wallet_is_off(w http.ResponseWriter, r *http.Request) {
	s := load_template("wallet_off.html")
	write_html_head(w, r)
	w.Write([]byte(s))
	write_html_tail(w)
}

func ServerThread() {
	fmt.Println("Starting WebUI at", common.CFG.WebUI.Interface)

	http.HandleFunc("/webui/", p_webui)

	http.HandleFunc("/wal", p_wal)
	http.HandleFunc("/snd", p_snd)
	http.HandleFunc("/balance.json", json_balance)
	http.HandleFunc("/payment.zip", dl_payment)
	http.HandleFunc("/balance.zip", dl_balance)

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

	http.HandleFunc("/", p_home)
	http.HandleFunc("/status.json", json_status)
	http.HandleFunc("/counts.json", json_counts)
	http.HandleFunc("/system.json", json_system)
	http.HandleFunc("/bwidth.json", json_bwidth)
	http.HandleFunc("/txstat.json", json_txstat)
	http.HandleFunc("/netcon.json", json_netcon)
	http.HandleFunc("/blocks.json", json_blocks)
	http.HandleFunc("/peerst.json", json_peerst)
	http.HandleFunc("/bwchar.json", json_bwchar)
	http.HandleFunc("/mempool_stats.json", json_mempool_stats)
	http.HandleFunc("/mempool_fees.json", json_mempool_fees)
	http.HandleFunc("/blkver.json", json_blkver)
	http.HandleFunc("/miners.json", json_miners)
	http.HandleFunc("/blfees.json", json_blfees)
	http.HandleFunc("/walsta.json", json_wallet_status)

	http.HandleFunc("/mempool_fees.txt", txt_mempool_fees)

	go start_ssl_server()
	http.ListenAndServe(common.CFG.WebUI.Interface, nil)
}

type null_logger struct {
}

func (nl null_logger) Write(p []byte) (n int, err error) {
	return len(p), nil
}

func start_ssl_server() {
	// try to start SSL server...
	dat, err := ioutil.ReadFile("ssl_cert/ca.crt")
	if err != nil {
		println("ssl_cert/ca.crt not found")
		// no "ca.crt" file - do not start SSL server
		return
	}

	port := common.CFG.WebUI.SSLPort
	if port == 0 {
		if common.Testnet {
			if common.DefaultTcpPort == 48333 {
				port = 44433
			} else {
				port = 14433
			}
		} else {
			port = 4433
		}
	}
	ssl_serv_addr := fmt.Sprint(":", port)

	server := &http.Server{
		Addr: ssl_serv_addr,
		TLSConfig: &tls.Config{
			ClientAuth: tls.RequireAndVerifyClientCert,
		},
		ErrorLog: log.New(new(null_logger), "", 0),
	}
	server.TLSConfig.ClientCAs = x509.NewCertPool()
	ok := server.TLSConfig.ClientCAs.AppendCertsFromPEM(dat)
	if !ok {
		println("AppendCertsFromPEM error")
		return
	}

	println("Starting SSL server at", ssl_serv_addr, "...")
	err = server.ListenAndServeTLS("ssl_cert/server.crt", "ssl_cert/server.key")
	if err != nil {
		println(err.Error())
	}
}
