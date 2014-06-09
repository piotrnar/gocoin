package script

import (
	"testing"
	"io/ioutil"
	"encoding/hex"
	"encoding/json"
	"github.com/piotrnar/gocoin/lib/btc"
)

// use some dummy tx
var dummy_tx *btc.Tx

func init() {
	rd, _ := hex.DecodeString("0100000001b14bdcbc3e01bdaad36cc08e81e69c82e1060bc14e518db2b49aa43ad90ba26000000000490047304402203f16c6f40162ab686621ef3000b04e75418a0c0cb2d8aebeac894ae360ac1e780220ddc15ecdfc3507ac48e1681a33eb60996631bf6bf5bc0a0682c4db743ce7ca2b01ffffffff0140420f00000000001976a914660d4ef3a743e3e696ad990364e555c271ad504b88ac00000000")
	dummy_tx, _ := btc.NewTx(rd)
	dummy_tx.Size = uint32(len(rd))
	ha := btc.Sha2Sum(rd)
	dummy_tx.Hash = btc.NewUint256(ha[:])
}


func TestScritpsValid(t *testing.T) {
	dat, er := ioutil.ReadFile("../test/script_valid.json")
	if er != nil {
		t.Error(er.Error())
		return
	}
	var vecs [][]string
	er = json.Unmarshal(dat, &vecs)
	if er != nil {
		t.Error(er.Error())
		return
	}

	tot := 0
	for i := range vecs {
		if len(vecs[i])>=2 {
			tot++

			s1, e := btc.DecodeScript(vecs[i][0])
			if e!=nil {
				t.Error(tot, "error A in", vecs[i][0], "->", vecs[i][1])
				return
			}
			s2, e := btc.DecodeScript(vecs[i][1])
			if e!=nil {
				t.Error(tot, "error B in", vecs[i][0], "->", vecs[i][1])
				return
			}

			res := VerifyTxScript(s1, s2, 0, dummy_tx, true)
			if !res {
				t.Error(tot, "VerifyTxScript failed in", vecs[i][0], "->", vecs[i][1])
				return
			}
		}
	}
}


func TestScritpsInvalid(t *testing.T) {
	var vecs [][]string

	DBG_ERR = false
	dat, er := ioutil.ReadFile("../test/script_invalid.json")
	if er != nil {
		t.Error(er.Error())
		return
	}
	er = json.Unmarshal(dat, &vecs)
	if er != nil {
		t.Error(er.Error())
		return
	}

	tot := 0
	for i := range vecs {
		if len(vecs[i])>=2 {
			tot++

			s1, e := btc.DecodeScript(vecs[i][0])
			if e!=nil {
				t.Error(tot, "error A in", vecs[i][0], "->", vecs[i][1])
				return
			}
			s2, e := btc.DecodeScript(vecs[i][1])
			if e!=nil {
				t.Error(tot, "error B in", vecs[i][0], "->", vecs[i][1])
				return
			}

			res := VerifyTxScript(s1, s2, 0, dummy_tx, true)
			if res {
				t.Error(tot, "VerifyTxScript NOT failed in", vecs[i][0], "->", vecs[i][1])
				return
			}
		}
	}
}
