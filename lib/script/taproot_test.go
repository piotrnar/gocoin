package script

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/piotrnar/gocoin/lib/btc"
	"io/ioutil"
	"testing"
)

type one_scr_tst struct {
	Tx       string   `json:"tx"`
	Prevouts []string `json:"prevouts"`
	Index    int      `json:"index"`
	Success  struct {
		ScriptSig string   `json:"scriptSig"`
		Witness   []string `json:"witness"`
	} `json:"success"`
	Failure struct {
		ScriptSig string   `json:"scriptSig"`
		Witness   []string `json:"witness"`
	} `json:"failure"`
	Flags   string `json:"flags"`
	Final   bool   `json:"final"`
	Comment string `json:"comment"`
}

func dump_test(tst *one_scr_tst) {
	b, er := json.MarshalIndent(tst, "", "  ")
	if er == nil {
		fmt.Println(string(b))
	}
}

func TestTaprootScritps(t *testing.T) {
	var tests []one_scr_tst
	var res bool

	DBG_ERR = false
	dat, er := ioutil.ReadFile("../test/bip341_script_tests.json")
	if er != nil {
		t.Error(er.Error())
		return
	}
	er = json.Unmarshal(dat, &tests)
	if er != nil {
		t.Error(er.Error())
		return
	}
	for i := 0; i < len(tests); i++ {
		//println("+++++++++++++", i, "+++++++++++++++")
		tv := tests[i]

		d, e := hex.DecodeString(tv.Tx)
		if e != nil {
			t.Fatal(i, e.Error())
		}
		tx, off := btc.NewTx(d)
		if tx == nil {
			t.Fatal(i, "Tx decode error", off, tv.Tx)
		}
		if off != len(d) {
			t.Fatal(i, "Tx not fully decoded", off, len(d), tv.Tx)
		}

		tx.Spent_outputs = make([]*btc.TxOut, len(tv.Prevouts))

		/*
		_b := new(bytes.Buffer)
		btc.WriteVlen(_b, uint64(len(tv.Prevouts)))
		outs := _b.Bytes()
		*/

		for i, pks := range tv.Prevouts {
			d, e = hex.DecodeString(pks)
			if e != nil {
				t.Fatal(i, e.Error())
			}
			tx.Spent_outputs[i] = new(btc.TxOut)
			rd := bytes.NewReader(d)
			e = binary.Read(rd, binary.LittleEndian, &tx.Spent_outputs[i].Value)
			if e != nil {
				t.Fatal(i, e.Error())
			}
			le, e := btc.ReadVLen(rd)
			if e != nil {
				t.Fatal(i, e.Error())
			}
			tx.Spent_outputs[i].Pk_script = make([]byte, int(le))
			_, e = rd.Read(tx.Spent_outputs[i].Pk_script)
			if e != nil {
				t.Fatal(i, e.Error())
			}
			//outs = append(outs, txout_serialize(tx.Spent_outputs[i])...)
		}

		idx := tv.Index
		if tv.Success.ScriptSig != "" {
			if d, er = hex.DecodeString(tv.Success.ScriptSig); er != nil {
				t.Fatal(i, e.Error())
			}
			tx.TxIn[idx].ScriptSig = d
		}
		if len(tv.Success.Witness) > 0 {
			tx.SegWit = make([][][]byte, len(tx.TxIn))
			tx.SegWit[idx] = make([][]byte, len(tv.Success.Witness))
			for i := range tv.Success.Witness {
				tx.SegWit[idx][i], e = hex.DecodeString(tv.Success.Witness[i])
				//println("wit", idx, i, hex.EncodeToString(tx.SegWit[idx][i]))
				if er != nil {
					t.Fatal(i, e.Error())
				}
			}
		}
		flags, er := decode_flags(tv.Flags)
		if er != nil {
			t.Fatal(i, er.Error())
		}

		//DBG_ERR = true
		res = VerifyTxScript(tx.Spent_outputs[idx].Pk_script, &SigChecker{Tx: tx, Idx: idx, Amount: tx.Spent_outputs[idx].Value}, flags)

		if false {
			hasz := tx.TaprootSigHash(&btc.ScriptExecutionData{
				M_tapleaf_hash:      btc.NewUint256FromString("b45b31b6d43e11c6e3c38b09942a7e6d8178eaa97965f387b0872b5857c6768d").Hash[:],
				M_codeseparator_pos: 0xffffffff}, idx, 2, false)

			println("hasz:", btc.NewUint256(hasz).String())
			break
		}

		if !res {
			//dump_test(&tv)
			t.Fatal(i, "Verify Failed for", tv.Comment)
		}

		if tv.Failure.ScriptSig != "" || len(tv.Failure.Witness) > 0 {
			if tv.Failure.ScriptSig != "" {
				if d, er = hex.DecodeString(tv.Failure.ScriptSig); er != nil {
					t.Fatal(i, e.Error())
				}
				tx.TxIn[idx].ScriptSig = d
			}
			if len(tv.Failure.Witness) > 0 {
				tx.SegWit = make([][][]byte, len(tx.TxIn))
				tx.SegWit[idx] = make([][]byte, len(tv.Failure.Witness))
				for i := range tv.Failure.Witness {
					tx.SegWit[idx][i], e = hex.DecodeString(tv.Failure.Witness[i])
					if er != nil {
						t.Fatal(i, e.Error())
					}
				}
			}

			res = VerifyTxScript(tx.Spent_outputs[idx].Pk_script, &SigChecker{Tx: tx, Idx: idx, Amount: tx.Spent_outputs[idx].Value}, flags)

			if res {
				dump_test(&tv)
				t.Fatal(i, "Verify not Failed but should")
			}
		}

		//break
	}
	//println("counters:", btc.EcdsaVerifyCnt(), btc.SchnorrVerifyCnt(), btc.CheckPay2ContractCnt())
}
