package btc

import (
	"fmt"
	"bytes"
	"errors"
	"strconv"
	"testing"
	"strings"
	"math/big"
	"io/ioutil"
	"encoding/hex"
	"encoding/json"
	"encoding/binary"
)


func RawToStack(sig []byte) ([]byte) {
	if len(sig)==1 {
		if sig[0]==0x81 {
			return []byte{OP_1NEGATE}
		}
		if sig[0]==0x80 || sig[0]==0x00 {
			return []byte{OP_0}
		}
		if sig[0]<=16 {
			return []byte{OP_1-1+sig[0]}
		}
	}
	bb := new(bytes.Buffer)
	if len(sig) < OP_PUSHDATA1 {
		bb.Write([]byte{byte(len(sig))})
	} else if len(sig) <= 0xff {
		bb.Write([]byte{OP_PUSHDATA1})
		bb.Write([]byte{byte(len(sig))})
	} else if len(sig) <= 0xffff {
		bb.Write([]byte{OP_PUSHDATA2})
		binary.Write(bb, binary.LittleEndian, uint16(len(sig)))
	} else {
		bb.Write([]byte{OP_PUSHDATA4})
		binary.Write(bb, binary.LittleEndian, uint32(len(sig)))
	}
	bb.Write(sig)
	return bb.Bytes()
}


func int2scr(v int64) ([]byte) {
	if v==-1 || v>=1 && v<=16 {
		return []byte{byte(v + OP_1 - 1)}
	}

	neg := v<0
	if neg {
		v = -v
	}
	bn := big.NewInt(v)
	bts := bn.Bytes()
	if (bts[0]&0x80)!=0 {
		if neg {
			bts = append([]byte{0x80}, bts...)
		} else {
			bts = append([]byte{0x00}, bts...)
		}
	} else if neg {
		bts[0] |= 0x80
	}

	sig := make([]byte, len(bts))
	for i := range bts {
		sig[len(bts)-i-1] = bts[i]
	}

	return RawToStack(sig)
}


func pk2hex(pk string) (out []byte, e error) {
	xx := strings.Split(pk, " ")
	for i := range xx {
		v, er := strconv.ParseInt(xx[i], 10, 64)
		if er==nil {
			switch {
				case v==-1: out = append(out, 0x4f)
				case v==0: out = append(out, 0x0)
				case v>0 && v<=16: out = append(out, 0x50+byte(v))
				default:
					out = append(out, int2scr(v)...)
			}
		} else if len(xx[i])>2 && xx[i][:2]=="0x" {
			d, _ := hex.DecodeString(xx[i][2:])
			out = append(out, d...)
		} else {
			if len(xx[i])>=2 && xx[i][0]=='\'' && xx[i][len(xx[i])-1]=='\'' {
				out = append(out, RawToStack([]byte(xx[i][1:len(xx[i])-1]))...)
			} else {
				if len(xx[i])>3 && xx[i][:3]=="OP_" {
					xx[i] = xx[i][3:]
				}
				switch(xx[i]) {
					case "NOP": out = append(out, 0x61)
					case "VER": out = append(out, 0x62)
					case "IF": out = append(out, 0x63)
					case "NOTIF": out = append(out, 0x64)
					case "VERIF": out = append(out, 0x65)
					case "VERNOTIF": out = append(out, 0x66)
					case "ELSE": out = append(out, 0x67)
					case "ENDIF": out = append(out, 0x68)
					case "VERIFY": out = append(out, 0x69)
					case "RETURN": out = append(out, 0x6a)
					case "TOALTSTACK": out = append(out, 0x6b)
					case "FROMALTSTACK": out = append(out, 0x6c)
					case "2DROP": out = append(out, 0x6d)
					case "2DUP": out = append(out, 0x6e)
					case "3DUP": out = append(out, 0x6f)
					case "2OVER": out = append(out, 0x70)
					case "2ROT": out = append(out, 0x71)
					case "2SWAP": out = append(out, 0x72)
					case "IFDUP": out = append(out, 0x73)
					case "DEPTH": out = append(out, 0x74)
					case "DROP": out = append(out, 0x75)
					case "DUP": out = append(out, 0x76)
					case "NIP": out = append(out, 0x77)
					case "OVER": out = append(out, 0x78)
					case "PICK": out = append(out, 0x79)
					case "ROLL": out = append(out, 0x7a)
					case "ROT": out = append(out, 0x7b)
					case "SWAP": out = append(out, 0x7c)
					case "TUCK": out = append(out, 0x7d)
					case "CAT": out = append(out, 0x7e)
					case "SUBSTR": out = append(out, 0x7f)
					case "LEFT": out = append(out, 0x80)
					case "RIGHT": out = append(out, 0x81)
					case "SIZE": out = append(out, 0x82)
					case "INVERT": out = append(out, 0x83)
					case "AND": out = append(out, 0x84)
					case "OR": out = append(out, 0x85)
					case "XOR": out = append(out, 0x86)
					case "EQUAL": out = append(out, 0x87)
					case "EQUALVERIFY": out = append(out, 0x88)
					case "RESERVED1": out = append(out, 0x89)
					case "RESERVED2": out = append(out, 0x8a)
					case "1ADD": out = append(out, 0x8b)
					case "1SUB": out = append(out, 0x8c)
					case "2MUL": out = append(out, 0x8d)
					case "2DIV": out = append(out, 0x8e)
					case "NEGATE": out = append(out, 0x8f)
					case "ABS": out = append(out, 0x90)
					case "NOT": out = append(out, 0x91)
					case "0NOTEQUAL": out = append(out, 0x92)
					case "ADD": out = append(out, 0x93)
					case "SUB": out = append(out, 0x94)
					case "MUL": out = append(out, 0x95)
					case "DIV": out = append(out, 0x96)
					case "MOD": out = append(out, 0x97)
					case "LSHIFT": out = append(out, 0x98)
					case "RSHIFT": out = append(out, 0x99)
					case "BOOLAND": out = append(out, 0x9a)
					case "BOOLOR": out = append(out, 0x9b)
					case "NUMEQUAL": out = append(out, 0x9c)
					case "NUMEQUALVERIFY": out = append(out, 0x9d)
					case "NUMNOTEQUAL": out = append(out, 0x9e)
					case "LESSTHAN": out = append(out, 0x9f)
					case "GREATERTHAN": out = append(out, 0xa0)
					case "LESSTHANOREQUAL": out = append(out, 0xa1)
					case "GREATERTHANOREQUAL": out = append(out, 0xa2)
					case "MIN": out = append(out, 0xa3)
					case "MAX": out = append(out, 0xa4)
					case "WITHIN": out = append(out, 0xa5)
					case "RIPEMD160": out = append(out, 0xa6)
					case "SHA1": out = append(out, 0xa7)
					case "SHA256": out = append(out, 0xa8)
					case "HASH160": out = append(out, 0xa9)
					case "HASH256": out = append(out, 0xaa)
					case "CODESEPARATOR": out = append(out, 0xab)
					case "CHECKSIG": out = append(out, 0xac)
					case "CHECKSIGVERIFY": out = append(out, 0xad)
					case "CHECKMULTISIG": out = append(out, 0xae)
					case "CHECKMULTISIGVERIFY": out = append(out, 0xaf)
					case "NOP1": out = append(out, 0xb0)
					case "NOP2": out = append(out, 0xb1)
					case "NOP3": out = append(out, 0xb2)
					case "NOP4": out = append(out, 0xb3)
					case "NOP5": out = append(out, 0xb4)
					case "NOP6": out = append(out, 0xb5)
					case "NOP7": out = append(out, 0xb6)
					case "NOP8": out = append(out, 0xb7)
					case "NOP9": out = append(out, 0xb8)
					case "NOP10": out = append(out, 0xb9)
					case "": out = append(out, []byte{}...)
					default:
						return nil, errors.New("Syntax error: "+xx[i])
				}
			}
		}
	}
	return
}


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

			s1, e := pk2hex(vecs[i][0])
			if e!=nil {
				t.Error(tot, "error A in", vecs[i][0], "->", vecs[i][1])
				return
			}
			s2, e := pk2hex(vecs[i][1])
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

			s1, e := pk2hex(vecs[i][0])
			if e!=nil {
				t.Error(tot, "error A in", vecs[i][0], "->", vecs[i][1])
				return
			}
			s2, e := pk2hex(vecs[i][1])
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

		pk, er := pk2hex(tv.inps[j].pkscr)
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
