package main

import (
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/client/wallet"
	"github.com/piotrnar/gocoin/lib"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"strconv"
)

var proxy string

type prvout struct {
	Address string
	Unspent []struct {
		Tx string
		N uint32
		Amount string
	}
}

type singleadd struct {
	Status string
	Data prvout
	Code int
	Message string
}

type mulitiadd struct {
	Status string
	Data []prvout
	Code int
	Message string
}

func print_help() {
	fmt.Println("Specify at lest one parameter on the command line.")
	fmt.Println("  Name of one text file containing litecoin addresses,")
	fmt.Println("... or space separteted litecoin addresses themselves.")
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
func GetTxFromBlockrIo(txid string) (raw []byte) {
	url := "http://ltc.blockr.io/api/v1/tx/raw/" + txid
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

func main() {
	fmt.Println("Gocoin LTCbal version", lib.Version)

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

	url := "http://ltc.blockr.io/api/v1/address/unspent/"
	for i := range addrs {
		if i > 0 {
			url += ","
		}
		url += addrs[i].String()
	}

	var sum, outcnt uint64
	r, er := http.Get(url)
	//println(url)
	if er == nil && r.StatusCode == 200 {
		defer r.Body.Close()
		c, _ := ioutil.ReadAll(r.Body)
		var r mulitiadd

		if len(addrs)==1 {
			var s singleadd
			er = json.Unmarshal(c[:], &s)
			if er == nil {
				r.Status = s.Status
				r.Data = []prvout{s.Data}
				r.Code = s.Code
				r.Message = s.Message
			}
		} else {
			er = json.Unmarshal(c[:], &r)
		}
		if er == nil {
			os.RemoveAll("balance/")
			os.Mkdir("balance/", 0700)

			println(r.Status, r.Code, r.Message, len(r.Data))

			unsp, _ := os.Create("balance/unspent.txt")
			for i :=  range r.Data {
				for j := range r.Data[i].Unspent {
					txraw := GetTxFromBlockrIo(r.Data[i].Unspent[j].Tx)
					if len(txraw)>0 {
						ioutil.WriteFile("balance/"+r.Data[i].Unspent[j].Tx+".tx", txraw, 0666)
					} else {
						println("ERROR: cannot fetch raw tx data for", r.Data[i].Unspent[j].Tx)
						os.Exit(1)
					}
					println(r.Data[i].Unspent[j].Tx, len(txraw))

					val, _ := btc.StringToSatoshis(r.Data[i].Unspent[j].Amount)
					sum += val
					outcnt++
					fmt.Fprintf(unsp, "%s-%03d # %s @ %s", r.Data[i].Unspent[j].Tx, r.Data[i].Unspent[j].N,
						r.Data[i].Unspent[j].Amount, r.Data[i].Address)
					fmt.Fprintln(unsp)
				}
			}
			unsp.Close()
			if outcnt > 0 {
				fmt.Printf("Total %.8f LTC in %d unspent outputs.\n", float64(sum)/1e8, outcnt)
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
