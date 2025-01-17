package utils

import (
	"fmt"
	"io"
	"net/http"

	"github.com/piotrnar/gocoin/lib/btc"
)

// GetBlockFromBlockstream downloads a raw block from blockstream
func GetBlockFromBlockstream(hash *btc.Uint256, api_url string) (raw []byte) {
	url := api_url + hash.String() + "/raw"
	r, er := http.Get(url)
	if er == nil {
		if r.StatusCode == 200 {
			raw, _ = io.ReadAll(r.Body)
			r.Body.Close()
		} else {
			fmt.Println("blockstream block StatusCode=", r.StatusCode)
		}
	}
	if er != nil {
		fmt.Println("blockstream block:", er.Error())
	}
	return
}

func IsBlockOK(raw []byte, hash *btc.Uint256) (bl *btc.Block) {
	var er error
	bl, er = btc.NewBlock(raw)
	if er != nil {
		return
	}
	if !bl.Hash.Equal(hash) {
		return nil
	}
	er = bl.BuildTxList()
	if er != nil {
		return nil
	}
	if !bl.MerkleRootMatch() {
		return nil
	}
	return
}

// GetBlockFromWeb downloads a raw block from a web server (try one after another).
func GetBlockFromWeb(hash *btc.Uint256) (bl *btc.Block) {
	var raw []byte

	raw = GetBlockFromBlockstream(hash, "https://blockstream.info/api/block/")
	if bl = IsBlockOK(raw, hash); bl != nil {
		if Verbose {
			println("GetTxFromBlockstream - OK")
		}
		return
	}
	if Verbose {
		println("GetTxFromBlockstream error")
	}

	raw = GetBlockFromBlockstream(hash, "https://mempool.space/api/block/")
	if bl = IsBlockOK(raw, hash); bl != nil {
		if Verbose {
			println("GetBlockFromMempoolSpace - OK")
		}
		return
	}
	if Verbose {
		println("GetBlockFromMempoolSpace error")
	}

	return
}
