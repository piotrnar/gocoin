package secp256k1

import (
	"bytes"
	"encoding/csv"
	"encoding/hex"
	"os"
	"testing"
)

func TestSchnorr(t *testing.T) {
	f, er := os.Open("../test/bip340_test_vectors.csv")
	if er != nil {
		t.Error(er.Error())
		return
	}
	cf := csv.NewReader(f)
	tas, er := cf.ReadAll()
	f.Close()
	if er != nil {
		t.Error(er.Error())
		return
	}
	for i := range tas {
		if i == 0 {
			continue // skip column names
		}
		priv, _ := hex.DecodeString(tas[i][1])
		pkey, _ := hex.DecodeString(tas[i][2])
		rand, _ := hex.DecodeString(tas[i][3])
		hasz, _ := hex.DecodeString(tas[i][4])
		sign, _ := hex.DecodeString(tas[i][5])

		if len(priv) == 32 {
			_sig := SchnorrSign(hasz, priv, rand)
			if !bytes.Equal(sign, _sig) {
				println(hex.EncodeToString(_sig))
				println(hex.EncodeToString(sign))
				t.Error(i, "Generated signature mismatch")
				return
			}
		}

		if tas[i][6] == "FALSE" {
			res := SchnorrVerify(pkey, sign, hasz)
			if res {
				t.Error("SchnorrVerify not failed")
			}
			continue
		}

		res := SchnorrVerify(pkey, sign, hasz)
		if !res {
			t.Error("SchnorrVerify failed", i, tas[i][0])
		}
		hasz[0]++
		res = SchnorrVerify(pkey, sign, hasz)
		if res {
			t.Error("SchnorrVerify not failed while it should")
		}
	}
}

func BenchmarkSchnorrVerify(b *testing.B) {
	pkey, _ := hex.DecodeString("DFF1D77F2A671C5F36183726DB2341BE58FEAE1DA2DECED843240F7B502BA659")
	hash, _ := hex.DecodeString("243F6A8885A308D313198A2E03707344A4093822299F31D0082EFA98EC4E6C89")
	sign, _ := hex.DecodeString("6896BD60EEAE296DB48A229FF71DFE071BDE413E6D43F917DC8DCF8C78DE33418906D11AC976ABCCB20B091292BFF4EA897EFCB639EA871CFA95F6DE339E4B0A")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !SchnorrVerify(pkey, sign, hash) {
			b.Fatal("sig_verify failed")
		}
	}
}

func BenchmarkCheckPayToContract(b *testing.B) {
	pkey, _ := hex.DecodeString("afaf8a67be00186668f74740e34ffce748139c2b73c9fbd2c1f33e48a612a75d")
	base, _ := hex.DecodeString("f1cbd3f2430910916144d5d2bf63d48a6281e5b8e6ade31413adccff3d8839d4")
	hash, _ := hex.DecodeString("93a760e87123883022cbd462ac40571176cf09d9d2c6168759fee6c2b079fdd8")
	parity := true
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !CheckPayToContract(pkey, base, hash, parity) {
			b.Fatal("CheckPayToContract failed")
		}
	}
}
