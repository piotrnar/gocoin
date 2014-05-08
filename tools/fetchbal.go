package main

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/piotrnar/gocoin/btc"
	"github.com/piotrnar/gocoin/client/wallet"
	"github.com/piotrnar/gocoin/others/utils"
	"github.com/piotrnar/gocoin/others/ver"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
)

var proxy string

type restype struct {
	Unspent_outputs []struct {
		Tx_hash       string
		Tx_index      uint64
		Tx_output_n   uint64
		Script        string
		Value         uint64
		Value_hex     string
		Confirmations uint64
	}
}

func print_help() {
	fmt.Println("Specify at lest one parameter on the command line.")
	fmt.Println("  Name of one text file containing bitcoin addresses,")
	fmt.Println("... or space separteted bitcoin addresses themselves.")
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

func main() {
	fmt.Println("Gocoin FetchBalance version", ver.SourcesTag)

	proxy = os.Getenv("TOR")
	if proxy != "" {
		fmt.Println("Using Tor at", proxy)
		http.DefaultClient.Transport = &http.Transport{Dial: dials5}
	} else {
		fmt.Println("WARNING: not using Tor (setup TOR variable, if you want)")
	}

	if len(os.Args) < 2 {
		print_help()
		return
	}

	var addrs []*btc.BtcAddr

	if len(os.Args) == 2 {
		fi, er := os.Stat(os.Args[1])
		if er == nil && fi.Size() > 10 && !fi.IsDir() {
			wal := wallet.NewWallet(os.Args[1])
			if wal != nil {
				fmt.Println("Found", len(wal.Addrs), "address(es) in", wal.FileName)
				addrs = wal.Addrs
			}
		}
	}

	if len(addrs) == 0 {
		for i := 1; i < len(os.Args); i++ {
			a, e := btc.NewAddrFromString(os.Args[i])
			if e != nil {
				println(os.Args[i], ": ", e.Error())
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

	url := "http://blockchain.info/unspent?active="
	for i := range addrs {
		if i > 0 {
			url += "|"
		}
		url += addrs[i].String()
	}

	var sum, outcnt uint64
	r, er := http.Get(url)
	//println(url)
	if er == nil && r.StatusCode == 200 {
		defer r.Body.Close()
		c, _ := ioutil.ReadAll(r.Body)
		var r restype
		er = json.Unmarshal(c[:], &r)
		if er == nil {
			os.RemoveAll("balance/")
			os.Mkdir("balance/", 0700)
			unsp, _ := os.Create("balance/unspent.txt")
			for i := 0; i < len(r.Unspent_outputs); i++ {
				pkscr, _ := hex.DecodeString(r.Unspent_outputs[i].Script)
				b58adr := "???"
				if pkscr != nil {
					ba := btc.NewAddrFromPkScript(pkscr, false)
					if ba != nil {
						b58adr = ba.String()
					}
				}
				txidlsb, _ := hex.DecodeString(r.Unspent_outputs[i].Tx_hash)
				if txidlsb != nil {
					txid := btc.NewUint256(txidlsb)
					rawtx := utils.GetTxFromWeb(txid)
					if rawtx != nil {
						ioutil.WriteFile("balance/"+txid.String()+".tx", rawtx, 0666)
						fmt.Fprintf(unsp, "%s-%03d # %.8f @ %s, %d confs\n",
							txid.String(), r.Unspent_outputs[i].Tx_output_n,
							float64(r.Unspent_outputs[i].Value)/1e8,
							b58adr, r.Unspent_outputs[i].Confirmations)
						sum += r.Unspent_outputs[i].Value
						outcnt++
					} else {
						fmt.Printf(" - cannot fetch %s-%03d\n", txid.String(), r.Unspent_outputs[i].Tx_output_n)
					}
				}
			}
			unsp.Close()
			if outcnt > 0 {
				fmt.Printf("Total %.8f BTC in %d unspent outputs.\n", float64(sum)/1e8, outcnt)
				fmt.Println("The data has been stored in 'balance' folder.")
				fmt.Println("Use it with the wallet app to spend any of it.")
			} else {
				fmt.Println("The fateched balance is empty.")
			}
		} else {
			fmt.Println("Unspent json.Unmarshal", er.Error())
		}
	} else {
		if er != nil {
			fmt.Println("Unspent ", er.Error())
		} else {
			fmt.Println("Unspent HTTP StatusCode", r.StatusCode)
		}
	}
}
