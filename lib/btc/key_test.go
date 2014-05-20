package btc

import (
//	"math/big"
	"encoding/hex"
	"testing"
)

func TestPbkyues(t *testing.T) {
	var tstvcs = []string {
		"04CEB28DE33FBC5ED8B343DE5B00E68A53B73653C204D236694BE2C2DD6A959AEB450163075FAE68D21D5EA9E2D07FE8742229AFAF02983034E84C614D16CF7107",
		"03CEB28DE33FBC5ED8B343DE5B00E68A53B73653C204D236694BE2C2DD6A959AEB",

		"0476232786465DE7FD05B68DB84A3C1D84AAEF2928E907D99969196383E71717A9F73B5631781A152BDBF3037AB605573A8D18E7628339EB8C4A8280E5D2E161EA",
		"0276232786465DE7FD05B68DB84A3C1D84AAEF2928E907D99969196383E71717A9",

		"04672E9066C6A7980359514621D1FE787CCD5B6539B67614B940929723BDF5623C1216E7F69AAE46FED4621DE63CA6EE1251BCC584C2362B7040DEF99F668C810B",
		"03672E9066C6A7980359514621D1FE787CCD5B6539B67614B940929723BDF5623C",

		"04B0B9D37E47380CB037C422365779C169723E4D05C300B7A8BD4A48A9B4E60F0647DEA3378908B133D69BB3E0943CCCF6872C0A94678166D819FEA558B96A812D",
		"03B0B9D37E47380CB037C422365779C169723E4D05C300B7A8BD4A48A9B4E60F06",

		"0491643DB1E5096E72FAF027D65A79085320186734BC4A5416B5F747F8761551566AD3B3EC7AA0360DEE4EBCCDBCB6FC5260881B9C747E10C671519D8713FFA4F5",
		"0391643DB1E5096E72FAF027D65A79085320186734BC4A5416B5F747F876155156",

		"04BD22E9E7AE9238EBD7937DCAF2887535B13DEB2EF9E95D5FB9225D29BDCD450FC99915A84D69CBA5E1EA1A11D6E9C49C039B949EE135B0133FEFCE156064A708",
		"02BD22E9E7AE9238EBD7937DCAF2887535B13DEB2EF9E95D5FB9225D29BDCD450F",

		"04FC0C0989BF1EC12C483AB65A63792DF96F74059DE1CB1B6599F3217A6E4957C2A9051A5FB9C15D13BBCC2FAEFF50F1BDE56BC692760A5D92DBFD52D2764BF723",
		"03FC0C0989BF1EC12C483AB65A63792DF96F74059DE1CB1B6599F3217A6E4957C2",

		"04259EB3B0BE2E1A92958F353450FD499E158E16C526C582F4016A2F05E2A5D78FEB2B208DDF07239278922A286E97672841EFE4C23C434D44472DAF777D50F145",
		"03259EB3B0BE2E1A92958F353450FD499E158E16C526C582F4016A2F05E2A5D78F",

		"046950778CC3F5B35FB74280770FE038C292C92705887F90FC1B2E184F578EFBC554DF9356EA840217D26E7E557C9D07F29C1BF8406C7DFA2BCA15891FCB8F8173",
		"036950778CC3F5B35FB74280770FE038C292C92705887F90FC1B2E184F578EFBC5",

		"04BF528DCFBAD67F851EE9B378590EDD42A585FD2D4BFD3E70B9DAD822CE4B2BBDDDDECD0FBD948013A29CBECD19FB96A23C8AAF741F85FF12505588AD106CFB24",
		"02BF528DCFBAD67F851EE9B378590EDD42A585FD2D4BFD3E70B9DAD822CE4B2BBD",

		"049F2F10A61354C2ADF5C5A9B82E2CFC4A209F8C5BA9B29B7DC8B56574105E4B80D484D98F6FD138B0DA7915B6E343D1E67DFA4A96FE4AD1B6AC52E0DAD3C01E66",
		"029F2F10A61354C2ADF5C5A9B82E2CFC4A209F8C5BA9B29B7DC8B56574105E4B80",

		"0458C19BD334283CD9935E14A10028B8B836685260E69F5AFA3FED459953A1FCBB55AC2CF10CCE64B372361A97D8C1D3A900838978C508C67B6B12BD376F705EEF",
		"0358C19BD334283CD9935E14A10028B8B836685260E69F5AFA3FED459953A1FCBB",

		"04C92EF93E2B187248460BDEDD1E340B198BD19B800AF5FBEDC3B0010A42E5B28641F4B7964B1AF6987BAAC2DAD2C6A3C0F1FBADD1ACF652721077A4DF5F502F3A",
		"02C92EF93E2B187248460BDEDD1E340B198BD19B800AF5FBEDC3B0010A42E5B286",
	}
	for i:=0; i<len(tstvcs); i+=2 {
		xy, _ := hex.DecodeString(tstvcs[i])
		x, _ := hex.DecodeString(tstvcs[i+1])
		k1, e := NewPublicKey(xy)
		if e != nil {
			t.Error("error new k1")
			return
		}
		k2, e := NewPublicKey(x)
		if e != nil {
			t.Error("error new k2")
			return
		}
		if !k1.X.Equals(&k2.X) {
			t.Error("X error", i)
			return
		}
		if !k1.Y.Equals(&k2.Y) {
			t.Error("Y error", i)
			return
		}
	}
}


/*
If it was up to me, this test should not pass, but we need to follow the original
implementation, because such an inconsistent signatures have been mined alredy.
See more comments about it at the end of NewSignature() in key.go
*/
func TestSignature(t *testing.T) {
	raw, _ := hex.DecodeString("3045022034e4786cf22cd00b45faff8afc3b8789c924378176d934adee0d3b3f4a8bf0dc022100b658dd07beeede1f792d238c3ee29c25200f3b834662f9c900bb4d065526dac90001")
	s, e := NewSignature(raw)
	if e != nil {
		t.Error(e.Error())
		return
	}
	if s.HashType!=1 {
		t.Error("HashType", s.HashType)
		return
	}
}


func BenchmarkKey02swap(b *testing.B) {
	xy, _ := hex.DecodeString("02BD22E9E7AE9238EBD7937DCAF2887535B13DEB2EF9E95D5FB9225D29BDCD450F")
	for i := 0; i < b.N; i++ {
		NewPublicKey(xy)
	}
}

func BenchmarkKey02nswa(b *testing.B) {
	xy, _ := hex.DecodeString("0276232786465DE7FD05B68DB84A3C1D84AAEF2928E907D99969196383E71717A9")
	for i := 0; i < b.N; i++ {
		NewPublicKey(xy)
	}
}

func BenchmarkKey03swap(b *testing.B) {
	xy, _ := hex.DecodeString("03672E9066C6A7980359514621D1FE787CCD5B6539B67614B940929723BDF5623C")
	for i := 0; i < b.N; i++ {
		NewPublicKey(xy)
	}
}

func BenchmarkKey03nswa(b *testing.B) {
	xy, _ := hex.DecodeString("03CEB28DE33FBC5ED8B343DE5B00E68A53B73653C204D236694BE2C2DD6A959AEB")
	for i := 0; i < b.N; i++ {
		NewPublicKey(xy)
	}
}

func BenchmarkKey04full(b *testing.B) {
	xy, _ := hex.DecodeString("049F2F10A61354C2ADF5C5A9B82E2CFC4A209F8C5BA9B29B7DC8B56574105E4B80D484D98F6FD138B0DA7915B6E343D1E67DFA4A96FE4AD1B6AC52E0DAD3C01E66")
	for i := 0; i < b.N; i++ {
		NewPublicKey(xy)
	}
}


type vervec struct {
	addr string
	signature string
	message string
	expected bool
}

func TestVerifyMessage(t *testing.T) {
	var testvcs = []vervec {
		{
			"13XSgyGGJcUso5f1EK8LZ7j194FtEvTfkn",
			"H2AoueOjHJ5yX8vX1dFnNqqq/Mm/FX37S+Yry88JadSIA21KNvojW4+fgVqm9UV6YH+VanGgNb8JcNhXi/IYu1o=",
			"rel net msg",
			true,
		},
		{
			"mqMmY5Uc6AgWoemdbRsvkpTes5hF6p5d8w",
			"H+HUh1GiTw22BMhqRwbSET/4aYCFIuivSgTyU/A+qH7xZp5gz61zp//WMFTbpNDbiMYoYz7pD88NYg/0DekcMpY=",
			"test",
			true,
		},
		{
			"muTPoTTXbVWdurzw4aqTh7DLQ82RRE8hXz",
			"H5iQmSJeZKrDcvKJrkAIOubFfajrxuPiSO0/xMorz+C31EyDF/bmkE+XLAihfkt3EQTEjxSgPURkdKxqJpxTw8Y=",
			"This is some test message",
			true,
		},
		{
			"mmS8FqnakrybtSzXSHXcGjeMfHUQqojx6Q",
			"H0m1/OUAc1amV02c/bMF2Rdv2pJIPYfdSv5To3rax5O0eauXuexvafATfdLN1VFh/71SvpayMm3qoq2/9y+QQBA=",
			"test",
			true,
		},
		{
			"mpu4t3bSgcWneVDKdjB8JHcGu2RgXT6QhJ",
			"H3PJeR3oSKwYfbiCFhzIpSbLjS3aZge2qMEi+gnB1ay+nNENnJo6uaejoVvo7+gBI3M7eU+jk5Jv91tj8DjOIxQ=",
			"test",
			true,
		},
		{
			"mwajpkz1ZthoAN3fvG8bCRvgoEf3BRraSP",
			"H+TE9nhNgZXizuEySs8npLojQMAEhE0r1TpJCC3QxV4dd4l8AEN3smJH5ryw4IcHApJf6Z5m5hxz8Q0vnPd5aVw=",
			"dlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibdlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibdlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibdlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibdlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibdlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibdlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibdlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibmwajpkz1ZthoAN3fvG8bCRvgoEf3BRraSPdlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibdlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibdlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibdlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibdlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibdlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibdlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibdlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibmwajpkz1ZthoAN3fvG8bCRvgoEf3BRraSPdlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibdlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibdlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibdlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibdlrhkjelrhjedslgjekrojirweopjighoerihoeirbeoibdlrhkjelrhjedslgjekrojirweopjighoerihoei",
			true,
		},
		{
			"mmS8FqnakrybtSzXSHXcGjeMfHUQqojx6Q",
			"H3PJeR3oSKwYfbiCFhzIpSbLjS3aZge2qMEi+gnB1ay+nNENnJo6uaejoVvo7+gBI3M7eU+jk5Jv91tj8DjOIxQ=",
			"test",
			false,
		},
		{
			"mqMmY5Uc6AgWoemdbRsvkpTes5hF6p5d8w",
			"H+hUh1GiTw22BMhqRwbSET/4aYCFIuivSgTyU/A+qH7xZp5gz61zp//WMFTbpNDbiMYoYz7pD88NYg/0DekcMpY=",
			"test",
			false,
		},
		{
			"momBPYuZ42xGVBNC1DxQBKM3WT3fa8MLMn",
			"ILRw4C+DSjqq+ie9K0ngcmnpYqUUEPNk6eGVwxNRoF5QVgl4rtdt6dXXgfh+0gaIMu1UXyshvwQGVKLa/2lMiwk=",
			"test",
			true,
		},
	}

	var hash [32]byte
	for i := range testvcs {
		ad, er := NewAddrFromString(testvcs[i].addr)
		if er != nil {
			t.Error(er.Error())
		}

		nv, sig, er := ParseMessageSignature(testvcs[i].signature)
		if er != nil {
			t.Error(er.Error())
		}

		HashFromMessage([]byte(testvcs[i].message), hash[:])

		compressed := nv>=31
		if compressed {
			nv -= 4
		}

		var verified_ok bool
		pub := sig.RecoverPublicKey(hash[:], int(nv-27))
		if pub != nil {
			sa := NewAddrFromPubkey(pub.Bytes(compressed), ad.Version)
			if sa != nil {
				verified_ok = ad.Hash160==sa.Hash160
			} else {
				t.Error("NewAddrFromPubkey failed")
			}
		}
		if verified_ok != testvcs[i].expected {
			t.Error("Result different than expected at index", i, verified_ok, testvcs[i].expected)
		}
	}
}

func BenchmarkRecoverKey(b *testing.B) {
	var hash [32]byte
	HashFromMessage([]byte("rel net msg"), hash[:])
	nv, sig, _ := ParseMessageSignature("H2AoueOjHJ5yX8vX1dFnNqqq/Mm/FX37S+Yry88JadSIA21KNvojW4+fgVqm9UV6YH+VanGgNb8JcNhXi/IYu1o=")
	for i := 0; i < b.N; i++ {
		sig.RecoverPublicKey(hash[:], int(nv-27))
	}
}
