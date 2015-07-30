package script

import (
	"testing"
	"strings"
	"io/ioutil"
	"encoding/json"
	"github.com/piotrnar/gocoin/lib/btc"
)

var NotSupported = []string {"DISCOURAGE_UPGRADABLE_NOPS", "MINIMALDATA", /*"DERSIG", */
"LOW_S", "STRICTENC", "NULLDUMMY", "SIGPUSHONLY", "CLEANSTACK"}

// use some dummy tx
var input_tx *btc.Tx

func init() {
	input_tx = new(btc.Tx)
	input_tx.Version = 1
	input_tx.TxIn = make([]*btc.TxIn, 1)
	input_tx.TxIn[0] = &btc.TxIn{}
	input_tx.TxIn[0].Input.Vout = 0xffffffff
	input_tx.TxIn[0].ScriptSig = []byte{0,0}
	input_tx.TxIn[0].Sequence = 0xffffffff
	input_tx.TxOut = make([]*btc.TxOut, 1)
	input_tx.TxOut[0] = &btc.TxOut{}
	input_tx.Lock_time = 0
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
		if len(vecs[i])>=3 {
			ok := true
			for k := range NotSupported {
				if strings.Contains(vecs[i][2], NotSupported[k]) {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}

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

			var flags uint32
			if strings.Contains(vecs[i][2], "P2SH") {
				flags |= VER_P2SH
			}
			if strings.Contains(vecs[i][2], "DERSIG") {
				flags |= VER_DERSIG
			}

			res := VerifyTxScript(s1, s2, 0, mk_out_tx(s1, s2), flags)
			if !res {
				t.Error(tot, "VerifyTxScript failed in", vecs[i][0], "->", vecs[i][1], "/", vecs[i][2])
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
		if len(vecs[i])>=3 {
			ok := true
			for k := range NotSupported {
				if strings.Contains(vecs[i][2], NotSupported[k]) {
					ok = false
					break
				}
			}
			if !ok {
				continue
			}

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

			var flags uint32
			if strings.Contains(vecs[i][2], "P2SH") {
				flags |= VER_P2SH
			}
			if strings.Contains(vecs[i][2], "DERSIG") {
				flags |= VER_DERSIG
			}

			res := VerifyTxScript(s1, s2, 0, mk_out_tx(s1, s2), flags)
			if res {
				t.Error(tot, "VerifyTxScript NOT failed in", vecs[i][0], "->", vecs[i][1], "/", vecs[i][2], "/", vecs[i][3])
				return
			}
		}
	}
}

func mk_out_tx(s1, s2 []byte) (output_tx *btc.Tx) {
	input_tx.TxOut[0].Pk_script = s2
	rd := input_tx.Serialize()
	input_tx.Size = uint32(len(rd))
	ha := btc.Sha2Sum(rd)
	input_tx.Hash = btc.NewUint256(ha[:])

	output_tx = new(btc.Tx)
	output_tx.Version = 1
	output_tx.TxIn = make([]*btc.TxIn, 1)
	output_tx.TxIn[0] = &btc.TxIn{}
	output_tx.TxIn[0].Input.Hash = input_tx.Hash.Hash
	output_tx.TxIn[0].Input.Vout = 0
	output_tx.TxIn[0].ScriptSig = s1
	output_tx.TxIn[0].Sequence = 0xffffffff
	output_tx.TxOut = make([]*btc.TxOut, 1)
	output_tx.TxOut[0] = &btc.TxOut{}
	output_tx.Lock_time = 0
	rd = output_tx.Serialize()
	output_tx.Size = uint32(len(rd))
	ha = btc.Sha2Sum(rd)
	output_tx.Hash = btc.NewUint256(ha[:])
	return
}