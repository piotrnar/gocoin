package main

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/piotrnar/gocoin"
	"github.com/piotrnar/gocoin/lib/btc"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
)

const MAX_UNSPENT_AT_ONCE = 20

var (
	proxy string
	ltc   bool
	tbtc  bool
)

type prvout struct {
	Address string
	Unspent []struct {
		Tx            string
		N             uint32
		Amount        string
		Confirmations uint
	}
}

type singleadd struct {
	Status  string
	Data    prvout
	Code    int
	Message string
}

type mulitiadd struct {
	Status  string
	Data    []prvout
	Code    int
	Message string
}

func print_help() {
	fmt.Println()
	fmt.Println("Specify at lest one parameter on the command line.")
	fmt.Println("  Name of one text file containing litecoin addresses,")
	fmt.Println("... or space separteted litecoin addresses themselves.")
	fmt.Println()
	fmt.Println("Add -ltc at the command line, to fetch Litecoin balance.")
	fmt.Println()
	fmt.Println("To use Tor, setup environment variable TOR=host:port")
	fmt.Println("The host:port should point to your Tor's SOCKS proxy.")
}

func dials5(tcp, dest string) (conn net.Conn, err error) {
	//println("Tor'ing to", dest, "via", proxy)
	var buf [10]byte
	var host, ps string
	var port uint64

	conn, err = net.Dial(tcp, proxy)
	if err != nil {
		return
	}

	_, err = conn.Write([]byte{5, 1, 0})
	if err != nil {
		return
	}

	_, err = io.ReadFull(conn, buf[:2])
	if err != nil {
		return
	}

	if buf[0] != 5 {
		err = errors.New("We only support SOCKS5 proxy.")
	} else if buf[1] != 0 {
		err = errors.New("SOCKS proxy connection refused.")
		return
	}

	host, ps, err = net.SplitHostPort(dest)
	if err != nil {
		return
	}

	port, err = strconv.ParseUint(ps, 10, 16)
	if err != nil {
		return
	}

	req := make([]byte, 5+len(host)+2)
	copy(req[:4], []byte{5, 1, 0, 3})
	req[4] = byte(len(host))
	copy(req[5:], []byte(host))
	binary.BigEndian.PutUint16(req[len(req)-2:], uint16(port))
	_, err = conn.Write(req)
	if err != nil {
		return
	}

	_, err = io.ReadFull(conn, buf[:])
	if err != nil {
		return
	}

	if buf[1] != 0 {
		err = errors.New("SOCKS proxy connection terminated.")
	}

	return
}

func splitHostPort(addr string) (host string, port uint16, err error) {
	host, portStr, err := net.SplitHostPort(addr)
	portInt, err := strconv.ParseUint(portStr, 10, 16)
	port = uint16(portInt)
	return
}

// Download raw transaction from blockr.io
func get_raw_tx(txid string) (raw []byte) {
	url := base_url() + "tx/raw/" + txid
	r, er := http.Get(url)
	if er == nil && r.StatusCode == 200 {
		defer r.Body.Close()
		c, _ := ioutil.ReadAll(r.Body)
		var txx struct {
			Status string
			Data   struct {
				Tx struct {
					Hex string
				}
			}
		}
		er = json.Unmarshal(c[:], &txx)
		if er == nil {
			raw, _ = hex.DecodeString(txx.Data.Tx.Hex)
		} else {
			println("er", er.Error())
		}
	}
	return
}

func get_unspent(addrs []*btc.BtcAddr) []prvout {
	if len(addrs) == 0 {
		panic("no addresses to fetch")
	}

	if len(addrs) > MAX_UNSPENT_AT_ONCE {
		panic("too many addresses")
	}

	url := base_url() + "address/unspent/" + addrs[0].String()
	for i := 1; i < len(addrs); i++ {
		url += "," + addrs[i].String()
	}

	r, er := http.Get(url)
	if er == nil {
		if r.StatusCode != 200 {
			println("get_unspent: StatusCode", r.StatusCode)
			return nil
		}

		c, _ := ioutil.ReadAll(r.Body)
		r.Body.Close()

		if len(addrs) == 1 {
			var s singleadd
			er = json.Unmarshal(c, &s)
			if er == nil {
				return []prvout{s.Data}
			}
		} else {
			var r mulitiadd
			er = json.Unmarshal(c, &r)
			if er == nil {
				return r.Data
			}
		}
	}

	if er != nil {
		println("get_unspent:", er.Error())
	}
	return nil
}

func base_url() string {
	if tbtc {
		return "http://tbtc.blockr.io/api/v1/"
	}
	if ltc {
		return "http://ltc.blockr.io/api/v1/"
	}
	return "http://btc.blockr.io/api/v1/"
}

func curr_unit() string {
	if ltc {
		return "LTC"
	} else {
		return "BTC"
	}
}

func load_wallet(fn string) (addrs []*btc.BtcAddr) {
	f, e := os.Open(fn)
	if e != nil {
		println(e.Error())
		return
	}
	defer f.Close()
	rd := bufio.NewReader(f)
	linenr := 0
	for {
		var l string
		l, e = rd.ReadString('\n')
		l = strings.Trim(l, " \t\r\n")
		linenr++
		if len(l) > 0 {
			if l[0] == '@' {
				fmt.Println("netsted wallet in line", linenr, "- ignore it")
			} else if l[0] != '#' {
				ls := strings.SplitN(l, " ", 2)
				if len(ls) > 0 {
					a, e := btc.NewAddrFromString(ls[0])
					if e != nil {
						println(fmt.Sprint(fn, ":", linenr), e.Error())
					} else {
						addrs = append(addrs, a)
					}
				}
			}
		}
		if e != nil {
			break
		}
	}
	return
}

func main() {
	fmt.Println("Gocoin BalIO version", gocoin.Version)

	if len(os.Args) < 2 {
		print_help()
		return
	}

	proxy = os.Getenv("TOR")
	if proxy != "" {
		fmt.Println("Using Tor at", proxy)
		http.DefaultClient.Transport = &http.Transport{Dial: dials5}
	} else {
		fmt.Println("WARNING: not using Tor (setup TOR variable, if you want)")
	}

	var addrs []*btc.BtcAddr

	var argz []string
	for i := 1; i < len(os.Args); i++ {
		if os.Args[i] == "-ltc" {
			ltc = true
		} else if os.Args[i] == "-t" {
			tbtc = true
		} else {
			argz = append(argz, os.Args[i])
		}
	}

	if len(argz) == 1 {
		fi, er := os.Stat(argz[0])
		if er == nil && fi.Size() > 10 && !fi.IsDir() {
			addrs = load_wallet(argz[0])
			if addrs != nil {
				fmt.Println("Found", len(addrs), "address(es) in", argz[0])
			}
		}
	}

	if len(addrs) == 0 {
		for i := range argz {
			a, e := btc.NewAddrFromString(argz[i])
			if e != nil {
				println(argz[i], ": ", e.Error())
				return
			} else {
				addrs = append(addrs, a)
			}
		}
	}

	if len(addrs) == 0 {
		print_help()
		return
	}

	for i := range addrs {
		switch addrs[i].Version {
		case 48:
			ltc = true
		case 111:
			tbtc = true
		}
	}

	if tbtc && ltc {
		println("Litecoin's testnet is not suppported")
		return
	}

	url := base_url() + "address/unspent/"
	for i := range addrs {
		if i > 0 {
			url += ","
		}
		url += addrs[i].String()
	}

	if len(addrs) == 0 {
		println("No addresses to fetch balance for")
		return
	}

	var sum, outcnt uint64

	os.RemoveAll("balance/")
	os.Mkdir("balance/", 0700)
	unsp, _ := os.Create("balance/unspent.txt")
	for off := 0; off < len(addrs); off += MAX_UNSPENT_AT_ONCE {
		var r []prvout
		if off+MAX_UNSPENT_AT_ONCE < len(addrs) {
			r = get_unspent(addrs[off : off+MAX_UNSPENT_AT_ONCE])
		} else {
			r = get_unspent(addrs[off:])
		}
		if r == nil {
			return
		}
		for i := range r {
			for j := range r[i].Unspent {
				txraw := get_raw_tx(r[i].Unspent[j].Tx)
				if len(txraw) > 0 {
					ioutil.WriteFile("balance/"+r[i].Unspent[j].Tx+".tx", txraw, 0666)
				} else {
					println("ERROR: cannot fetch raw tx data for", r[i].Unspent[j].Tx)
					os.Exit(1)
				}

				val, _ := btc.StringToSatoshis(r[i].Unspent[j].Amount)
				sum += val
				outcnt++
				fmt.Fprintf(unsp, "%s-%03d # %s @ %s, %d confs", r[i].Unspent[j].Tx,
					r[i].Unspent[j].N, r[i].Unspent[j].Amount, r[i].Address, r[i].Unspent[j].Confirmations)
				fmt.Fprintln(unsp)
			}
		}
	}
	unsp.Close()
	if outcnt > 0 {
		fmt.Printf("Total %.8f %s in %d unspent outputs.\n", float64(sum)/1e8, curr_unit(), outcnt)
		fmt.Println("The data has been stored in 'balance' folder.")
		fmt.Println("Use it with the wallet app to spend any of it.")
	} else {
		fmt.Println("No coins found on the given address(es).")
	}
}
