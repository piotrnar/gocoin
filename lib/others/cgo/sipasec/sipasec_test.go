package sipasec

import (
	"encoding/csv"
	"encoding/hex"
	"os"
	"testing"
)

var ta = [][3]string{
	{ // [0]-pubScr, [1]-sigScript, [2]-unsignedTx
		"040eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66dbeb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16",
		"3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c01",
		"3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6",
	},
	{
		"020eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66d",
		"3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c01",
		"3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6",
	},
	{
		"0411db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5cb2e0eaddfb84ccf9744464f82e160bfa9b8b64f9d4c03f999b8643f656b412a3",
		"304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56cbbac4622082221a8768d1d0901",
		"7a05c6145f10101e9d6325494245adf1297d80f8f38d4d576d57cdba220bcb19",
	},
	{
		"0311db93e1dcdb8a016b49840f8c53bc1eb68a382e97b1482ecad7b148a6909a5c",
		"304402204e45e16932b8af514961a1d3a1a25fdf3f4f7732e9d624c6c61548ab5fb8cd410220181522ec8eca07de4860a4acdd12909d831cc56cbbac4622082221a8768d1d0901",
		"7a05c6145f10101e9d6325494245adf1297d80f8f38d4d576d57cdba220bcb19",
	},
	{
		"0428f42723f81c70664e200088437282d0e11ae0d4ae139f88bdeef1550471271692970342db8e3f9c6f0123fab9414f7865d2db90c24824da775f00e228b791fd",
		"3045022100d557da5d9bf886e0c3f98fd6d5d337487cd01d5b887498679a57e3d32bd5d0af0220153217b63a75c3145b14f58c64901675fe28dba2352c2fa9f2a1579c74a2de1701",
		"c22de395adbb0720941e009e8a4e488791b2e428af775432ed94d2c7ec8e421a",
	},
	{
		"0328f42723f81c70664e200088437282d0e11ae0d4ae139f88bdeef15504712716",
		"3045022100d557da5d9bf886e0c3f98fd6d5d337487cd01d5b887498679a57e3d32bd5d0af0220153217b63a75c3145b14f58c64901675fe28dba2352c2fa9f2a1579c74a2de1701",
		"c22de395adbb0720941e009e8a4e488791b2e428af775432ed94d2c7ec8e421a",
	},
	{
		"041f2a00036b3cbd1abe71dca54d406a1e9dd5d376bf125bb109726ff8f2662edcd848bd2c44a86a7772442095c7003248cc619bfec3ddb65130b0937f8311c787",
		"3045022100ec6eb6b2aa0580c8e75e8e316a78942c70f46dd175b23b704c0330ab34a86a34022067a73509df89072095a16dbf350cc5f1ca5906404a9275ebed8a4ba219627d6701",
		"7c8e7c2cb887682ed04dc82c9121e16f6d669ea3d57a2756785c5863d05d2e6a",
	},
	{
		"031f2a00036b3cbd1abe71dca54d406a1e9dd5d376bf125bb109726ff8f2662edc",
		"3045022100ec6eb6b2aa0580c8e75e8e316a78942c70f46dd175b23b704c0330ab34a86a34022067a73509df89072095a16dbf350cc5f1ca5906404a9275ebed8a4ba219627d6701",
		"7c8e7c2cb887682ed04dc82c9121e16f6d669ea3d57a2756785c5863d05d2e6a",
	},
	{
		"04ee90bfdd4e07eb1cfe9c6342479ca26c0827f84bfe1ab39e32fc3e94a0fe00e6f7d8cd895704e974978766dd0f9fad3c97b1a0f23684e93b400cc9022b7ae532",
		"3045022100fe1f6e2c2c2cbc916f9f9d16497df2f66a4834e5582d6da0ee0474731c4a27580220682bad9359cd946dc97bb07ea8fad48a36f9b61186d47c6798ccce7ba20cc22701",
		"baff983e6dfb1052918f982090aa932f56d9301d1de9a726d2e85d5f6bb75464",
	},
}

func TestVerify(t *testing.T) {
	for i := range ta {
		pkey, _ := hex.DecodeString(ta[i][0])
		sign, _ := hex.DecodeString(ta[i][1])
		hasz, _ := hex.DecodeString(ta[i][2])

		res := EC_Verify(pkey, sign, hasz)
		if res != 1 {
			t.Error("Verify failed")
		}
		hasz[0]++
		res = EC_Verify(pkey, sign, hasz)
		if res != 0 {
			t.Error("Verify not failed while it should")
		}
		res = EC_Verify(pkey[:1], sign, hasz)
		if res >= 0 {
			t.Error("Negative result expected", res)
		}
		res = EC_Verify(pkey, sign[:1], hasz)
		if res >= 0 {
			t.Error("Yet negative result expected", res)
		}
		res = EC_Verify(pkey, sign, hasz[:1])
		if res != 0 {
			t.Error("Zero expected", res)
		}
	}

}

func TestSchnorrVerify(t *testing.T) {
	f, er := os.Open("../../../test/bip340_test_vectors.csv")
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
			res := Schnorr_Verify(pkey, sign, hasz)
			if res != 0 {
				t.Error("Schnorr_Verify not failed")
			}
			continue
		}

		res := Schnorr_Verify(pkey, sign, hasz)
		if res != 1 {
			t.Error("Schnorr_Verify failed")
		}
		hasz[0]++
		res = Schnorr_Verify(pkey, sign, hasz)
		if res != 0 {
			t.Error("SchnorrVerify not failed while it should")
		}
	}
}

func BenchmarkVerifyUncompressed(b *testing.B) {
	key, _ := hex.DecodeString("040eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66dbeb26b67d7a00e2447baeccc8a4cef7cd3cad67376ac1c5785aeebb4f6441c16")
	sig, _ := hex.DecodeString("3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c01")
	msg, _ := hex.DecodeString("3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EC_Verify(key, sig, msg)
	}
}

func BenchmarkVerifyCompressed(b *testing.B) {
	key_compr, _ := hex.DecodeString("020eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66d")
	sig, _ := hex.DecodeString("3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c01")
	msg, _ := hex.DecodeString("3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		EC_Verify(key_compr, sig, msg)
	}
}

func BenchmarkSchnorrVerify(b *testing.B) {
	key, _ := hex.DecodeString("DFF1D77F2A671C5F36183726DB2341BE58FEAE1DA2DECED843240F7B502BA659")
	sig, _ := hex.DecodeString("6896BD60EEAE296DB48A229FF71DFE071BDE413E6D43F917DC8DCF8C78DE33418906D11AC976ABCCB20B091292BFF4EA897EFCB639EA871CFA95F6DE339E4B0A")
	msg, _ := hex.DecodeString("243F6A8885A308D313198A2E03707344A4093822299F31D0082EFA98EC4E6C89")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Schnorr_Verify(key, sig, msg)
	}
}

func BenchmarkCheckPayToContract(b *testing.B) {
	pkey, _ := hex.DecodeString("afaf8a67be00186668f74740e34ffce748139c2b73c9fbd2c1f33e48a612a75d")
	base, _ := hex.DecodeString("f1cbd3f2430910916144d5d2bf63d48a6281e5b8e6ade31413adccff3d8839d4")
	hash, _ := hex.DecodeString("93a760e87123883022cbd462ac40571176cf09d9d2c6168759fee6c2b079fdd8")
	parity := true
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if CheckPayToContract(pkey, base, hash, parity) != 1 {
			b.Fatal("CheckPayToContract failed")
		}
	}
}
