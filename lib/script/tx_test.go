package script

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/piotrnar/gocoin/lib/btc"
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

// ALL_VERIFY_FLAGS is a set of all the script verification flags that we know of.
// It is the gocoin's equivalent of the bitcoin core's script_verify_flags full mask.
const ALL_VERIFY_FLAGS = VER_P2SH | VER_STRICTENC | VER_DERSIG | VER_LOW_S |
	VER_NULLDUMMY | VER_SIGPUSHONLY | VER_MINDATA | VER_BLOCK_OPS | VER_CLEANSTACK |
	VER_CLTV | VER_CSV | VER_WITNESS | VER_WITNESS_PROG | VER_MINIMALIF | VER_NULLFAIL |
	VER_WITNESS_PUBKEY | VER_CONST_SCRIPTCODE | VER_TAPROOT | VER_DIS_TAPVER |
	VER_DIS_SUCCESS | VER_DIS_PUBKEYTYPE

// trim_flags removes the flags that cannot be used without their prerequisites.
// It mirrors TrimFlags() from bitcoin core's src/test/transaction_tests.cpp
func trim_flags(fl uint32) uint32 {
	if (fl & VER_P2SH) == 0 { // WITNESS requires P2SH
		fl &^= VER_WITNESS
	}
	if (fl & VER_WITNESS) == 0 { // CLEANSTACK requires WITNESS (and thus also P2SH)
		fl &^= VER_CLEANSTACK
	}
	return fl
}

// parserec decodes a single test vector.
// If excluded_flags is true, the flags field of the record lists the flags that shall
// NOT be applied (all the other ones will be), which is the convention used by the
// current version of tx_valid.json. Otherwise the field lists the flags to be applied.
func parserec(vv []interface{}, excluded_flags bool) (ret *testvector) {
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
	if params == "BADTX" {
		// The tx is expected to fail CheckTransaction(), not the script verification.
		ret.skip = "BADTX"
		return
	}
	var e error
	ret.ver_flags, e = decode_flags(params) // deifned in script_test.go
	if e != nil {
		println("skip", params)
		ret.skip = e.Error()
		return
	}
	if excluded_flags {
		ret.ver_flags = trim_flags(ALL_VERIFY_FLAGS &^ ret.ver_flags)
	}
	return
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

	if !tx.IsCoinBase() {
		for i := range tx.TxIn {
			if tx.TxIn[i].Input.IsNull() {
				return false
			}
		}
	}
	tx.AllocVerVars()

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
	dat, er := os.ReadFile("../test/tx_valid.json")
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

	// Newer versions of tx_valid.json list the flags to be EXCLUDED, instead of the
	// ones to be applied. The file's own header tells us which convention it follows.
	excluded_flags := false
	for _, v := range m {
		if vv, ok := v.([]interface{}); ok && len(vv) == 1 {
			if s, ok := vv[0].(string); ok && strings.Contains(s, "excluded verifyFlags") {
				excluded_flags = true
				break
			}
		}
	}

	cnt := 0
	for _, v := range m {
		switch vv := v.(type) {
		case []interface{}:
			if len(vv) == 3 {
				cnt++
				tv := parserec(vv, excluded_flags)
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
	dat, er := os.ReadFile("../test/tx_invalid.json")
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
				tv := parserec(vv, false)
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

	dat, er := os.ReadFile("../test/sighash.json")
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


// PoC: gocoin accepts a coinbase whose outputs total 2^64 satoshis.
//
// Bitcoin Core rejects such transactions in CheckTransaction
// (src/consensus/tx_check.cpp): bad-txns-vout-negative /
// bad-txns-vout-toolarge / bad-txns-txouttotal-toolarge.
// Gocoin performs no output value checks and its uint64 sums wrap,
// so the block-level checks in chain.commitTxs pass.
//
// Drop this file into lib/script/ and run:
//   go test -v -run TestPoCValueOverflow
//
// Credits: Bitcoin Red Team / @brunoerg

func TestPoCValueOverflow(t *testing.T) {
	// Coinbase with two outputs of 2^63 satoshis each (184.4 billion BTC total)
	tx := new(btc.Tx)
	tx.Version = 1
	tx.TxIn = []*btc.TxIn{{Input: btc.TxPrevOut{Vout: 0xffffffff}, ScriptSig: []byte{3, 1, 2, 3}}}
	tx.TxOut = []*btc.TxOut{
		{Value: 1 << 63, Pk_script: []byte{0x51}},
		{Value: 1 << 63, Pk_script: []byte{0x51}},
	}
	tx.NoWitSize = 100

	er := tx.CheckTransaction()

	// the exact summation done by chain.commitTxs() (lib/chain/chain_accept.go)
	var txoutsum, sumblockin, sumblockout uint64
	sumblockin = btc.GetBlockReward(850000) // current subsidy
	for j := range tx.TxOut {
		txoutsum += tx.TxOut[j].Value
	}
	sumblockout += txoutsum

	t.Logf("CheckTransaction error: %v", er)
	t.Logf("coinbase outputs total: 2 x 2^63 sat = %.0f BTC each", float64(uint64(1)<<63)/1e8)
	t.Logf("wrapped txoutsum: %d ; block check 'sumblockin < sumblockout' fails: %v", txoutsum, sumblockin < sumblockout)
	if er == nil && txoutsum == 0 && !(sumblockin < sumblockout) {
		t.Error("CONSENSUS DIVERGENCE CONFIRMED: 2^64 satoshis can be minted by a gocoin miner")
	}
}
