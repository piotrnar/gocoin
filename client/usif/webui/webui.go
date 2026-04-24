package webui

import (
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/piotrnar/gocoin"
	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/client/usif"
)

var start_time time.Time

func load_template(fn string) string {
	dat, er := os.ReadFile("www/templ/" + fn)
	if er != nil {
		return er.Error() + "\n"
	}
	return string(dat)
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
	s = strings.Replace(s, "/*_WALLET_ON_*/", fmt.Sprint("var wallet_on = ", common.Get(&common.WalletON)), 1)
	s = strings.Replace(s, "/*_CHAIN_IN_SYNC_*/", fmt.Sprint("var chain_in_sync = ", common.BlockChainSynchronized.Load()), 1)

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
	s = strings.Replace(s, "<!--LOAD_TIME-->", time.Since(start_time).String(), 1)
	w.Write([]byte(s))
}

func p_wallet_is_off(w http.ResponseWriter, r *http.Request) {
	s := load_template("wallet_off.html")
	write_html_head(w, r)
	w.Write([]byte(s))
	write_html_tail(w)
}

func p_general(w http.ResponseWriter, r *http.Request) {
	var page string
	if r.URL.Path == "/" {
		http.Redirect(w, r, "/home", http.StatusFound)
		return
		// home
	} else {
		ss := strings.Split(r.URL.Path, "/")
		if len(ss) != 2 {
			w.WriteHeader(http.StatusNotFound)
			w.Write([]byte("Not found " + r.URL.Path))
			return
		}
		page = ss[1]
	}
	if page == "send" || page == "wallet" {
		if !common.Get(&common.WalletON) {
			p_wallet_is_off(w, r)
			return
		}
	}
	if dat, er := os.ReadFile("www/" + page + ".html"); er == nil {
		if page == "txs" {
			txs_page_modify(r, &dat)
		}
		write_html_head(w, r)
		w.Write(dat)
		write_html_tail(w)

	} else {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(er.Error()))
	}
}

func p_authkey(w http.ResponseWriter, r *http.Request) {
	w.Header()["Content-Type"] = []string{"text/plain"}
	w.Write([]byte(common.PublicKey))
}

func ServerThread() {
	fmt.Println("Starting WebUI at", common.CFG.WebUI.Interface)

	http.Handle("/static/", http.FileServer(http.Dir("www")))

	http.HandleFunc("/", p_general)
	http.HandleFunc("/cfg", p_cfg)

	http.HandleFunc("/balance.json", json_balance)
	http.HandleFunc("/payment.zip", dl_payment)
	http.HandleFunc("/balance.zip", dl_balance)

	http.HandleFunc("/txs2s.xml", xml_txs2s)
	http.HandleFunc("/txsre.xml", xml_txsre)
	http.HandleFunc("/txw4i.xml", xml_txw4i)
	http.HandleFunc("/raw_tx", raw_tx)

	http.HandleFunc("/status.json", json_status)
	http.HandleFunc("/counts.json", json_counts)
	http.HandleFunc("/system.json", json_system)
	http.HandleFunc("/bwidth.json", json_bwidth)
	http.HandleFunc("/txstat.json", json_txstat)
	http.HandleFunc("/netcon.json", json_netcon)
	http.HandleFunc("/blocks.json", json_blocks)
	http.HandleFunc("/peerst.json", json_peerst)
	http.HandleFunc("/bwchar.json", json_bwchar)
	http.HandleFunc("/mpfees.json", json_mpfees)
	http.HandleFunc("/blkver.json", json_blkver)
	http.HandleFunc("/miners.json", json_miners)
	http.HandleFunc("/blfees.json", json_blfees)
	http.HandleFunc("/walsta.json", json_wallet_status)

	http.HandleFunc("/mempool_fees.txt", txt_mempool_fees)
	http.HandleFunc("/authkey.txt", p_authkey)

	go start_ssl_server()
	http.ListenAndServe(common.CFG.WebUI.Interface, usif.TrackingMiddleware(http.DefaultServeMux))
}

type null_logger struct {
}

func (nl null_logger) Write(p []byte) (n int, err error) {
	return len(p), nil
}

type certReloader struct {
	certPath    string
	keyPath     string
	mu          sync.Mutex // serializes reloads
	cert        atomic.Pointer[tls.Certificate]
	lastAttempt time.Time     // last time we tried to reload
	retryAfter  time.Duration // minimum gap between reload attempts
}

func newCertReloader(certPath, keyPath string, retryAfter time.Duration) (*certReloader, error) {
	cr := &certReloader{certPath: certPath, keyPath: keyPath, retryAfter: retryAfter}
	if err := cr.reload(); err != nil {
		return nil, err
	}
	return cr, nil
}

func (cr *certReloader) reload() error {
	cr.mu.Lock()
	defer cr.mu.Unlock()

	// Someone else may have reloaded while we were waiting for the lock.
	if existing := cr.cert.Load(); existing != nil && existing.Leaf != nil &&
		time.Now().Before(existing.Leaf.NotAfter) {
		return nil
	}

	// Don't hammer the disk if the file isn't being updated.
	if !cr.lastAttempt.IsZero() && time.Since(cr.lastAttempt) < cr.retryAfter {
		return nil
	}
	cr.lastAttempt = time.Now()

	cert, err := tls.LoadX509KeyPair(cr.certPath, cr.keyPath)
	if err != nil {
		return err
	}
	// Parse the leaf so we can check NotAfter later without re-parsing.
	if cert.Leaf == nil && len(cert.Certificate) > 0 {
		leaf, err := x509.ParseCertificate(cert.Certificate[0])
		if err != nil {
			return err
		}
		cert.Leaf = leaf
	}
	cr.cert.Store(&cert)
	println("SSL certificate loaded, expires:", cert.Leaf.NotAfter.String())
	return nil
}

func (cr *certReloader) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cert := cr.cert.Load()
	if cert != nil && cert.Leaf != nil && time.Now().Before(cert.Leaf.NotAfter) {
		return cert, nil
	}
	// Expired - try to reload (may be a no-op if we tried recently).
	if err := cr.reload(); err != nil {
		// Reload failed. Fall back to the old cert so the handshake can
		// still proceed; the client will see the expired cert and decide.
		println("cert reload failed:", err.Error())
		return cert, nil
	}
	return cr.cert.Load(), nil
}

func start_ssl_server() {
	// try to start SSL server...
	dat, err := os.ReadFile("ssl_cert/ca.crt")
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

	cr, err := newCertReloader("ssl_cert/server.crt", "ssl_cert/server.key", 5*time.Minute)
	if err != nil {
		println("cert load error:", err.Error())
		return
	}

	server := &http.Server{
		Addr:    ssl_serv_addr,
		Handler: usif.TrackingMiddleware(http.DefaultServeMux),
		TLSConfig: &tls.Config{
			ClientAuth:     tls.RequireAndVerifyClientCert,
			GetCertificate: cr.GetCertificate,
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
	err = server.ListenAndServeTLS("", "")
	if err != nil {
		println(err.Error())
	}
}
