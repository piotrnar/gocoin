package script

import (
	//"fmt"
	"errors"
	"testing"
	"strings"
	"io/ioutil"
	"encoding/json"
	"github.com/piotrnar/gocoin/lib/btc"
)



func TestScritpsValid(t *testing.T) {
	DBG_ERR = false

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

			flags, e := decode_flags(vecs[i][2])
			if e != nil {
				//fmt.Println("InvalidScript", tot, e.Error())
				continue
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

			flags, e := decode_flags(vecs[i][2])
			if e != nil {
				//fmt.Println("InvalidScript", tot, e.Error())
				continue
			}

			res := VerifyTxScript(s1, s2, 0, mk_out_tx(s1, s2), flags)
			if res {
				t.Error(tot, "VerifyTxScript NOT failed in", vecs[i][0], "->", vecs[i][1], "/", vecs[i][2], "/", vecs[i][3])
				return
			}
		}
	}
}


func decode_flags(s string) (fl uint32, e error) {
	ss := strings.Split(s, ",")
	for i := range ss {
		switch ss[i] {
			case "": // ignore
			case "NONE": // ignore
				break
			case "P2SH":
				fl |= VER_P2SH
			case "DERSIG":
				fl |= VER_DERSIG
			default:
				e = errors.New("Unsupported flag "+ss[i])
				return
		}
	}
	return
}


func mk_out_tx(sig_scr, pk_scr []byte) (output_tx *btc.Tx) {
	// We build input_tx only to calculate it's hash for output_tx
	input_tx := new(btc.Tx)
	input_tx.Version = 1
	input_tx.TxIn = []*btc.TxIn{ &btc.TxIn{Input:btc.TxPrevOut{Vout:0xffffffff},
		ScriptSig:[]byte{0,0}, Sequence:0xffffffff} }
	input_tx.TxOut = []*btc.TxOut{ &btc.TxOut{Pk_script:pk_scr} }
	// Lock_time = 0

	output_tx = new(btc.Tx)
	output_tx.Version = 1
	output_tx.TxIn = []*btc.TxIn{ &btc.TxIn{Input:btc.TxPrevOut{Hash:btc.Sha2Sum(input_tx.Serialize()), Vout:0},
		ScriptSig:sig_scr, Sequence:0xffffffff} }
	output_tx.TxOut = []*btc.TxOut{ &btc.TxOut{} }
	// Lock_time = 0

	return
}