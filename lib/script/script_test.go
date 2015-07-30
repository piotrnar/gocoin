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


func mk_out_tx(s1, s2 []byte) (output_tx *btc.Tx) {
	input_tx := new(btc.Tx)
	input_tx.Version = 1
	input_tx.TxIn = []*btc.TxIn{ &btc.TxIn{Input:btc.TxPrevOut{Vout:0xffffffff}, ScriptSig:[]byte{0,0}, Sequence:0xffffffff} }

	input_tx.TxOut = make([]*btc.TxOut, 1)
	input_tx.TxOut[0] = &btc.TxOut{}
	input_tx.Lock_time = 0

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