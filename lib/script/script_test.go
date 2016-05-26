package script

import (
	//"os"
	//"fmt"
	"errors"
	"testing"
	"strings"
	"io/ioutil"
	"encoding/json"
	"github.com/piotrnar/gocoin/lib/btc"
)


func TestScritps(t *testing.T) {
	var vecs [][]string

	DBG_ERR = false
	dat, er := ioutil.ReadFile("../test/script_tests.json")
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
				println("unknonw flag", tot, e.Error())
				continue
			}

			exp_res := vecs[i][3]=="OK"

			res := VerifyTxScript(s2, 0, mk_out_tx(s1, s2), flags)

			if res!=exp_res {
				t.Error(tot, "TestScritps failed in", vecs[i][0], "->", vecs[i][1], "/", vecs[i][2], res, vecs[i][3])
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
			case "STRICTENC":
				fl |= VER_STRICTENC
			case "DERSIG":
				fl |= VER_DERSIG
			case "LOW_S":
				fl |= VER_LOW_S
			case "NULLDUMMY":
				fl |= VER_NULLDUMMY
			case "SIGPUSHONLY":
				fl |= VER_SIGPUSHONLY
			case "MINIMALDATA":
				fl |= VER_MINDATA
			case "DISCOURAGE_UPGRADABLE_NOPS":
				fl |= VER_BLOCK_OPS
			case "CLEANSTACK":
				fl |= VER_CLEANSTACK
			case "CHECKLOCKTIMEVERIFY":
				fl |= VER_CLTV
			case "CHECKSEQUENCEVERIFY":
				fl |= VER_CSV
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

	output_tx.Hash = btc.NewSha2Hash(output_tx.Serialize())

	return
}