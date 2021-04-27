package secp256k1

import (
	"encoding/csv"
	"encoding/hex"
	"os"
	"testing"
)

func TestSchnorrVerify(t *testing.T) {
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
		pkey, _ := hex.DecodeString(tas[i][2])
		hasz, _ := hex.DecodeString(tas[i][4])
		sign, _ := hex.DecodeString(tas[i][5])
		//println(i, len(pkey), len(hasz), len(sign), tas[i][6], tas[i][2])

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
		continue
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


