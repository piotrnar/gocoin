package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/piotrnar/gocoin/lib/btc"
)

func getfees(hash *btc.Uint256) (res []uint64, er error) {
	var r *http.Response
	var start_index int
	var url string
	var c []byte
	var result []struct {
		TxID   string `json:"txid"`
		Fee    uint64 `json:"fee"`
		Size   uint64 `json:"size"`
		Weight uint64 `json:"weight"`
	}

	for {
		url = fmt.Sprint("https://mempool.space/api/block/"+hash.String()+"/txs/", start_index)
		//println(url)
		if r, er = http.Get(url); er != nil {
			println(er.Error())
			return
		}
		if r.StatusCode != 200 {
			er = errors.New(fmt.Sprint("HTTP StatusCode ", r.StatusCode))
			return
		}

		c, er = io.ReadAll(r.Body)
		r.Body.Close()
		if er != nil {
			return
		}
		println("got", len(c), "bytes")

		//os.WriteFile("resp.json", c, 0600)

		if er = json.Unmarshal(c, &result); er != nil {
			return
		}

		println("arrlen", len(result), "at index", start_index, len(res))
		if len(result) == 0 {
			return
		}

		for _, r := range result {
			res = append(res, r.Fee)
			//res = append(res, 1000*r.Fee/r.Size)
		}
		if len(result) != 25 {
			return
		}
		start_index += 25
	}
}

func main() {
	if len(os.Args) < 2 {
		println("Specify the block hash")
		return
	}
	block_hash := btc.NewUint256FromString(os.Args[1])
	if block_hash == nil {
		println("Incorrent block hash")
		return
	}

	fname := "../block_" + block_hash.String()[56:] + "_fees.go"
	f, er := os.Create(fname)
	if er != nil {
		println(er.Error())
		return
	}
	defer f.Close()
	wr := bufio.NewWriter(f)

	block_fees, er := getfees(block_hash)
	if er != nil && len(block_fees) == 0 {
		println(er.Error())
		return
	}

	fmt.Fprintln(wr, "package main")
	fmt.Fprintln(wr, "var block_hash = \""+block_hash.String()+"\"")
	fmt.Fprintln(wr, "var block_fees = []uint64{")

	for i, f := range block_fees {
		fmt.Fprint(wr, " ", f, ",")
		if (i & 7) == 7 {
			fmt.Fprintln(wr)
		}
	}
	fmt.Fprintln(wr, "\n}")

	wr.Flush()

}
