package script

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/secp256k1"
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
	dat, er := os.ReadFile("../test/bip341_script_tests.json")
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

		tx.AllocVerVars()
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

// poc_internal_key is the x-only pubkey of private key 1 (same convention as
// bitcoin core's script_tests.json harness).
var poc_internal_key = []byte{
	0x79, 0xbe, 0x66, 0x7e, 0xf9, 0xdc, 0xbb, 0xac,
	0x55, 0xa0, 0x62, 0x95, 0xce, 0x87, 0x0b, 0x07,
	0x02, 0x9b, 0xfc, 0xdb, 0x2d, 0xce, 0x28, 0xd9,
	0x59, 0xf2, 0x81, 0x5b, 0x16, 0xf8, 0x17, 0x98,
}

// poc_make_taproot_leaf builds the control block and the tweaked output key
// for a tree consisting of a single tapscript leaf.
func poc_make_taproot_leaf(script []byte) (control, output []byte) {
	sha := btc.Hasher(btc.HASHER_TAPLEAF)
	sha.Write([]byte{TAPROOT_LEAF_TAPSCRIPT})
	btc.WriteVlen(sha, uint64(len(script)))
	sha.Write(script)
	merkle_root := sha.Sum(nil)

	sha = btc.Hasher(btc.HASHER_TAPTWEAK)
	sha.Write(poc_internal_key)
	sha.Write(merkle_root)

	var tweak secp256k1.Number
	tweak.SetBytes(sha.Sum(nil))

	var pk secp256k1.XY
	pk.ParseXOnlyPubkey(poc_internal_key)
	if !pk.ECPublicTweakAdd(&tweak) {
		panic("cannot tweak the taproot internal key")
	}
	pk.X.Normalize()
	pk.Y.Normalize()

	output = make([]byte, 32)
	pk.X.GetB32(output)

	control = make([]byte, 33)
	control[0] = TAPROOT_LEAF_TAPSCRIPT
	if pk.Y.IsOdd() {
		control[0] |= 1
	}
	copy(control[1:], poc_internal_key)
	return
}

func poc_credit_tx(pk_scr []byte, value uint64) (input_tx *btc.Tx) {
	input_tx = new(btc.Tx)
	input_tx.Version = 1
	input_tx.TxIn = []*btc.TxIn{{Input: btc.TxPrevOut{Vout: 0xffffffff},
		ScriptSig: []byte{0, 0}, Sequence: 0xffffffff}}
	input_tx.TxOut = []*btc.TxOut{{Pk_script: pk_scr, Value: value}}
	input_tx.SetHash(input_tx.Serialize())
	return
}

func poc_spend_tx(input_tx *btc.Tx, sig_scr []byte, witness [][]byte) (output_tx *btc.Tx) {
	output_tx = new(btc.Tx)
	output_tx.Version = 1
	output_tx.TxIn = []*btc.TxIn{{Input: btc.TxPrevOut{Hash: btc.Sha2Sum(input_tx.Serialize()), Vout: 0},
		ScriptSig: sig_scr, Sequence: 0xffffffff}}
	output_tx.TxOut = []*btc.TxOut{{Value: input_tx.TxOut[0].Value}}
	if len(witness) > 0 {
		output_tx.SegWit = make([][][]byte, 1)
		output_tx.SegWit[0] = witness
	}
	output_tx.SetHash(output_tx.Serialize())
	return
}

func TestPoCInvalidHashtype(t *testing.T) {
	DBG_ERR = false
	// keypair with private key 2
	sk := make([]byte, 32)
	sk[31] = 2
	var n secp256k1.Number
	n.SetBytes(sk)
	var xyz secp256k1.XYZ
	secp256k1.ECmultGen(&xyz, &n)
	var pk secp256k1.XY
	pk.SetXYZ(&xyz)
	pk.X.Normalize()
	pub32 := make([]byte, 32)
	pk.X.GetB32(pub32)

	// tapscript: <32-byte pubkey> OP_CHECKSIG
	tapscript := append([]byte{0x20}, pub32...)
	tapscript = append(tapscript, 0xac)

	control, output := poc_make_taproot_leaf(tapscript)

	// Sign the all-zero message: this is what gocoin hashes when the
	// hashtype is invalid (TaprootSigHash returns make([]byte, 32)).
	var zero32, aux [32]byte
	sig := secp256k1.SchnorrSign(zero32[:], sk, aux[:])
	if sig == nil {
		t.Fatal("SchnorrSign failed")
	}
	sig = append(sig, 0x04) // invalid hashtype byte

	pkscr := append([]byte{0x51, 0x20}, output...) // P2TR output
	credit_tx := poc_credit_tx(pkscr, 1e8)
	spend_tx := poc_spend_tx(credit_tx, nil, [][]byte{sig, tapscript, control})
	spend_tx.Raw = spend_tx.SerializeNew()
	spend_tx.AllocVerVars()
	spend_tx.Spent_outputs = []*btc.TxOut{{Pk_script: pkscr, Value: 1e8}}

	flags := uint32(VER_P2SH | VER_WITNESS | VER_TAPROOT)
	res := VerifyTxScript(pkscr, &SigChecker{Amount: 1e8, Idx: 0, Tx: spend_tx}, flags)
	t.Logf("gocoin accepts invalid-hashtype(0x04) tapscript spend: %v  (Bitcoin Core rejects it)", res)
	if res {
		t.Error("CONSENSUS DIVERGENCE CONFIRMED")
	}
}
