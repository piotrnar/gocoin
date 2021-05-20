package script

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/piotrnar/gocoin/lib/btc"
	"io/ioutil"
	"testing"
)

type oneinp struct {
	txid  *btc.Uint256
	vout  int
	pkscr string
	value uint64
}

type testvector struct {
	inps      []oneinp
	tx        string
	ver_flags uint32
	skip      string
}

var last_descr string

func (tv *testvector) String() (s string) {
	s += fmt.Sprintf("Tx with %d inputs:\n", len(tv.inps))
	for i := range tv.inps {
		s += fmt.Sprintf(" %3d) %s-%03d\n", i, tv.inps[i].txid, tv.inps[i].vout)
		s += fmt.Sprintf("      %s\n", tv.inps[i].pkscr)
	}
	s += fmt.Sprintf(" tx_len:%d   flags:0x%x\n", len(tv.tx), tv.ver_flags)
	return
}

func parserec(vv []interface{}) (ret *testvector) {
	ret = new(testvector)
	for i, u := range vv[0].([]interface{}) {
		switch uu := u.(type) {
		case []interface{}:
			txid := btc.NewUint256FromString(uu[0].(string))
			newrec := oneinp{txid: txid, vout: int(uu[1].(float64)), pkscr: uu[2].(string)}
			if len(uu) > 3 {
				newrec.value = uint64(uu[3].(float64))
			}
			ret.inps = append(ret.inps, newrec)
		default:
			fmt.Printf(" - %d is of a type %T\n", i, uu)
		}
	}
	ret.tx = vv[1].(string)
	params := vv[2].(string)
	var e error
	ret.ver_flags, e = decode_flags(params) // deifned in script_test.go
	if e != nil {
		println("skip", params)
		ret.skip = e.Error()
	}
	return
}

// Some tests from the satoshi's json files are not applicable
// for our architectre so lets just fake them.
func skip_broken_tests(tx *btc.Tx) bool {
	// No inputs
	if len(tx.TxIn) == 0 {
		return true
	}

	// Negative output
	for i := range tx.TxOut {
		if tx.TxOut[i].Value > btc.MAX_MONEY {
			return true
		}
	}

	// Duplicate inputs
	if len(tx.TxIn) > 1 {
		for i := 0; i < len(tx.TxIn)-1; i++ {
			for j := i + 1; j < len(tx.TxIn); j++ {
				if tx.TxIn[i].Input == tx.TxIn[j].Input {
					return true
				}
			}
		}
	}

	// Coinbase of w wrong size
	if tx.IsCoinBase() {
		if len(tx.TxIn[0].ScriptSig) < 2 {
			return true
		}
		if len(tx.TxIn[0].ScriptSig) > 100 {
			return true
		}
	}

	return false
}

func execute_test_tx(t *testing.T, tv *testvector) bool {
	if len(tv.inps) == 0 {
		t.Error("Vector has no inputs")
		return false
	}
	rd, er := hex.DecodeString(tv.tx)
	if er != nil {
		t.Error(er.Error())
		return false
	}
	tx, _ := btc.NewTx(rd)
	if tx == nil {
		t.Error("Canot decode tx")
		return false
	}
	tx.Size = uint32(len(rd))
	tx.SetHash(rd)

	if skip_broken_tests(tx) {
		return false
	}

	if !tx.IsCoinBase() {
		for i := range tx.TxIn {
			if tx.TxIn[i].Input.IsNull() {
				return false
			}
		}
	}

	oks := 0
	for i := range tx.TxIn {
		var j int
		for j = range tv.inps {
			if bytes.Equal(tx.TxIn[i].Input.Hash[:], tv.inps[j].txid.Hash[:]) &&
				tx.TxIn[i].Input.Vout == uint32(tv.inps[j].vout) {
				break
			}
		}
		if j >= len(tv.inps) {
			t.Error("Matching input not found")
			continue
		}

		pk, er := btc.DecodeScript(tv.inps[j].pkscr)
		if er != nil {
			t.Error(er.Error())
			continue
		}

		if VerifyTxScript(pk, &SigChecker{Amount: tv.inps[j].value, Idx: i, Tx: tx}, tv.ver_flags) {
			oks++
		}
	}
	return oks == len(tx.TxIn)
}

func TestValidTransactions(t *testing.T) {
	var str interface{}
	dat, er := ioutil.ReadFile("../test/tx_valid.json")
	if er != nil {
		println(er.Error())
		return
	}

	er = json.Unmarshal(dat, &str)
	if er != nil {
		println(er.Error())
		return
	}
	m := str.([]interface{})
	cnt := 0
	for _, v := range m {
		switch vv := v.(type) {
		case []interface{}:
			if len(vv) == 3 {
				cnt++
				tv := parserec(vv)
				if tv.skip != "" {
					//println(tv.skip)
				} else if !execute_test_tx(t, tv) {
					t.Error(cnt, "Failed transaction:", last_descr)
				}
			} else if len(vv) == 1 {
				last_descr = vv[0].(string)
			}
		}
	}
}

func TestInvalidTransactions(t *testing.T) {
	var str interface{}
	dat, er := ioutil.ReadFile("../test/tx_invalid.json")
	if er != nil {
		println(er.Error())
		return
	}

	er = json.Unmarshal(dat, &str)
	if er != nil {
		println(er.Error())
		return
	}
	m := str.([]interface{})
	cnt := 0
	for _, v := range m {
		switch vv := v.(type) {
		case []interface{}:
			if len(vv) == 3 {
				cnt++
				if cnt == 64000 {
					DBG_SCR = true
				}
				tv := parserec(vv)
				if tv.skip != "" {
					//println(tv.skip)
				} else if execute_test_tx(t, tv) {
					t.Error(cnt, "NOT failed transaction:", last_descr)
					return
				}
				last_descr = ""
				if cnt == 64000 {
					return
				}
			} else if len(vv) == 1 {
				if last_descr == "" {
					last_descr = vv[0].(string)
				} else {
					last_descr += "\n" + vv[0].(string)
				}
			}
		}
	}
}

func TestSighash(t *testing.T) {
	var arr [][]interface{}

	dat, er := ioutil.ReadFile("../test/sighash.json")
	if er != nil {
		println(er.Error())
		return
	}

	r := bytes.NewBuffer(dat)
	d := json.NewDecoder(r)
	d.UseNumber()

	er = d.Decode(&arr)
	if er != nil {
		println(er.Error())
		return
	}
	for i := range arr {
		if len(arr[i]) == 5 {
			tmp, _ := hex.DecodeString(arr[i][0].(string))
			tx, _ := btc.NewTx(tmp)
			if tx == nil {
				t.Error("Cannot decode tx from text number", i)
				continue
			}
			tmp, _ = hex.DecodeString(arr[i][1].(string)) // script
			iidx, _ := arr[i][2].(json.Number).Int64()
			htype, _ := arr[i][3].(json.Number).Int64()
			got := tx.SignatureHash(tmp, int(iidx), int32(htype))
			exp := btc.NewUint256FromString(arr[i][4].(string))
			if !bytes.Equal(exp.Hash[:], got) {
				t.Error("SignatureHash mismatch at index", i)
			}
		}
	}
}
