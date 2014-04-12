package btc

import (
	"fmt"
	"bytes"
	"testing"
	"io/ioutil"
	"encoding/hex"
	"encoding/json"
)


func TestScritps(t *testing.T) {
	// use some dummy tx
	rd, _ := hex.DecodeString("0100000001b14bdcbc3e01bdaad36cc08e81e69c82e1060bc14e518db2b49aa43ad90ba26000000000490047304402203f16c6f40162ab686621ef3000b04e75418a0c0cb2d8aebeac894ae360ac1e780220ddc15ecdfc3507ac48e1681a33eb60996631bf6bf5bc0a0682c4db743ce7ca2b01ffffffff0140420f00000000001976a914660d4ef3a743e3e696ad990364e555c271ad504b88ac00000000")
	tx, _ := NewTx(rd)
	tx.Size = uint32(len(rd))
	ha := Sha2Sum(rd)
	tx.Hash = NewUint256(ha[:])

	dat, er := ioutil.ReadFile("test/script_valid.json")
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

			s1, e := DecodeScript(vecs[i][0])
			if e!=nil {
				t.Error(tot, "error A in", vecs[i][0], "->", vecs[i][1])
				return
			}
			s2, e := DecodeScript(vecs[i][1])
			if e!=nil {
				t.Error(tot, "error B in", vecs[i][0], "->", vecs[i][1])
				return
			}

			res := VerifyTxScript(s1, s2, 0, tx, true)
			if !res {
				t.Error(tot, "VerifyTxScript failed in", vecs[i][0], "->", vecs[i][1])
				return
			}
		}
	}
	//t.Info(tot, "valid vectors processed")

	dat, er = ioutil.ReadFile("test/script_invalid.json")
	if er != nil {
		t.Error(er.Error())
		return
	}
	er = json.Unmarshal(dat, &vecs)
	if er != nil {
		t.Error(er.Error())
		return
	}

	//DbgSwitch(DBG_SCRIPT, true)
	tot = 0
	for i := range vecs {
		if len(vecs[i])>=2 {
			tot++

			s1, e := DecodeScript(vecs[i][0])
			if e!=nil {
				t.Error(tot, "error A in", vecs[i][0], "->", vecs[i][1])
				return
			}
			s2, e := DecodeScript(vecs[i][1])
			if e!=nil {
				t.Error(tot, "error B in", vecs[i][0], "->", vecs[i][1])
				return
			}

			res := VerifyTxScript(s1, s2, 0, tx, true)
			if res {
				t.Error(tot, "VerifyTxScript NOT failed in", vecs[i][0], "->", vecs[i][1])
				return
			}
		}
	}
	//println(tot, "invalid vectors processed")
}



type oneinp struct {
	txid *Uint256
	vout int
	pkscr string
}

type testvector struct {
	inps []oneinp
	tx string
	p2sh bool
}

var last_descr string

func (tv *testvector) String() (s string) {
	s += fmt.Sprintf("Tx with %d inputs:\n", len(tv.inps))
	for i := range tv.inps {
		s += fmt.Sprintf(" %3d) %s-%03d\n", i, tv.inps[i].txid, tv.inps[i].vout)
		s += fmt.Sprintf("      %s\n", tv.inps[i].pkscr)
	}
	s += fmt.Sprintf(" tx_len:%d   p2sh:%t\n", len(tv.tx), tv.p2sh)
	return
}


func parserec(vv []interface{}) (ret *testvector) {
	ret = new(testvector)
	for i, u := range vv[0].([]interface{}) {
		switch uu := u.(type) {
			case []interface{}:
				txid := NewUint256FromString(uu[0].(string))
				ret.inps = append(ret.inps, oneinp{txid:txid,
					vout:int(uu[1].(float64)), pkscr:uu[2].(string)})
			default:
				fmt.Printf(" - %d is of a type %T\n", i, uu)
		}
	}
	ret.tx = vv[1].(string)
	ret.p2sh = vv[2].(bool)
	return
}



// Some tests from the satoshi's json files are not applicable
// ... for our architectre so lets just fake them.
func skip_broken_tests(tx *Tx) bool {
	// No inputs
	if len(tx.TxIn)==0 {
		return true
	}

	// Negative output
	for i := range tx.TxOut {
		if tx.TxOut[i].Value > MAX_MONEY {
			return true
		}
	}

	// Duplicate inputs
	if len(tx.TxIn) > 1 {
		for i:=0; i<len(tx.TxIn)-1; i++ {
			for j:=i+1; j<len(tx.TxIn); j++ {
				if tx.TxIn[i].Input == tx.TxIn[j].Input {
					return true
				}
			}
		}
	}

	// Coinbase of w wrong size
	if tx.IsCoinBase() {
		if len(tx.TxIn[0].ScriptSig)<2 {
			return true
		}
		if len(tx.TxIn[0].ScriptSig)>100 {
			return true
		}
	}

	return false
}


func execute_test_tx(t *testing.T, tv *testvector) bool {
	if len(tv.inps)==0 {
		t.Error("Vector has no inputs")
		return false
	}
	rd, er := hex.DecodeString(tv.tx)
	if er != nil {
		t.Error(er.Error())
		return false
	}
	tx, _ := NewTx(rd)
	if tx==nil {
		t.Error("Canot decode tx")
		return false
	}
	tx.Size = uint32(len(rd))
	ha := Sha2Sum(rd)
	tx.Hash = NewUint256(ha[:])

	if skip_broken_tests(tx) {
		return false
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
		if j>=len(tv.inps) {
			t.Error("Matching input not found")
			continue
		}

		pk, er := DecodeScript(tv.inps[j].pkscr)
		if er!=nil {
			t.Error(er.Error())
			continue
		}
		var ss []byte
		if tv.inps[j].vout>=0 {
			ss = tx.TxIn[i].ScriptSig
		}
		if VerifyTxScript(ss, pk, i, tx, tv.p2sh) {
			oks++
		}
	}
	return oks==len(tx.TxIn)
}


func TestValidTransactions(t *testing.T) {
	var str interface{}
	dat, er := ioutil.ReadFile("test/tx_valid.json")
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
	for _, v := range m {
		switch vv := v.(type) {
			case []interface{}:
				if len(vv)==3 {
					tv := parserec(vv)
					if !execute_test_tx(t, tv) {
						t.Error("Failed transaction:", last_descr)
					}
				} else if len(vv)==1 {
					last_descr = vv[0].(string)
				}
		}
	}
}


func TestInvalidTransactions(t *testing.T) {
	var str interface{}
	dat, er := ioutil.ReadFile("test/tx_invalid.json")
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
				if len(vv)==3 {
					cnt++
					tv := parserec(vv)
					if execute_test_tx(t, tv) {
						t.Error(cnt, "NOT failed transaction:", last_descr)
						return
					}
					last_descr = ""
				} else if len(vv)==1 {
					if last_descr=="" {
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

	dat, er := ioutil.ReadFile("test/sighash.json")
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
		if len(arr[i])==5 {
			tmp, _ := hex.DecodeString(arr[i][0].(string))
			tx, _ := NewTx(tmp)
			if tx == nil {
				t.Error("Cannot decode tx from text number", i)
				continue
			}
			tmp, _ = hex.DecodeString(arr[i][1].(string)) // script
			iidx, _ := arr[i][2].(json.Number).Int64()
			htype, _ := arr[i][3].(json.Number).Int64()
			got := tx.SignatureHash(tmp, int(iidx), int32(htype))
			exp := NewUint256FromString(arr[i][4].(string))
			if !bytes.Equal(exp.Hash[:], got) {
				t.Error("SignatureHash mismatch at index", i)
			}
		}
	}
}
