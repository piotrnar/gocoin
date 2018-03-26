package btc

import (
	"encoding/hex"
	"io/ioutil"
	"net/http"
	"os"
	"testing"
)

const block_hash = "0000000000000000000884ad62c7036a7e2022bca3f0bd68628414150e8a0ea6"

var _block_filename = ""

func block_filename() string {
	if _block_filename == "" {
		_block_filename = os.TempDir() + string(os.PathSeparator) + block_hash
	}
	return _block_filename
}

// Download block from blockchain.info and store it in the TEMP folder
func fetch_block(b *testing.B) {
	url := "https://blockchain.info/block/" + block_hash + "?format=hex"
	r, er := http.Get(url)
	if er == nil {
		if r.StatusCode == 200 {
			rawhex, er := ioutil.ReadAll(r.Body)
			r.Body.Close()
			if er == nil {
				raw, er := hex.DecodeString(string(rawhex))
				if er == nil {
					er = ioutil.WriteFile(block_filename(), raw, 0600)
				}
			}
		} else {
			b.Fatal("Unexpected HTTP Status code", r.StatusCode, url)
		}
	} else {
		b.Fatal(er.Error())
	}
	return
}

func BenchmarkBuildTxList(b *testing.B) {
	raw, e := ioutil.ReadFile(block_filename())
	if e != nil {
		fetch_block(b)
		if raw, e = ioutil.ReadFile(block_filename()); e != nil {
			b.Fatal(e.Error())
		}
	}
	b.SetBytes(int64(len(raw)))
	bl, e := NewBlock(raw)
	if e != nil {
		b.Fatal(e.Error())
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bl.TxCount = 0
		bl.BuildTxList()
	}
}

func BenchmarkCalcMerkle(b *testing.B) {
	raw, e := ioutil.ReadFile(block_filename())
	if e != nil {
		fetch_block(b)
		if raw, e = ioutil.ReadFile(block_filename()); e != nil {
			b.Fatal(e.Error())
		}
	}
	bl, e := NewBlock(raw)
	if e != nil {
		b.Fatal(e.Error())
	}
	bl.BuildTxList()
	mtr := make([][32]byte, len(bl.Txs), 3*len(bl.Txs)) // make the buffer 3 times longer as we use append() inside CalcMerkle
	for i, tx := range bl.Txs {
		mtr[i] = tx.Hash.Hash
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		CalcMerkle(mtr)
	}
}
