package btc

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"strings"
	"testing"
)

func TestAddr(t *testing.T) {
	var ta = []string{
		"mhXjRE6owowGYs8TocxRWw3n1TzCgvSkMA",
		"mqMmY5Uc6AgWoemdbRsvkpTes5hF6p5d8w",
		"mmS8FqnakrybtSzXSHXcGjeMfHUQqojx6Q",
		"mpu4t3bSgcWneVDKdjB8JHcGu2RgXT6QhJ",
		"mwZSC78JGfS6NY7R57aFeJQp4HgRCadHze",
		"mtGE6YtAVfCJ13QsudWCxKXBa893hQqnbi",
		"moY4DavYucRFGvExYPb9HX1jeHGBA4cWTX",
		"mqFh6A8tmZu5LBMYGFEJg2zCp3VSrNRvNN",
		"mqFh6A8tmZu5LBMYGFEJg2zCp3VSrNRvNN",
		"mtGE6YtAVfCJ13QsudWCxKXBa893hQqnbi",
		"mtGE6YtAVfCJ13QsudWCxKXBa893hQqnbi",

		"1F5rEq8JZnDYkjGPZgtfSxjaY4KQknAVpf",
		"17FpGMVndZUUwayXMsantBmyb3Pbe5Wq4c",
		"1JgV28xpDzpK4QXrgaZ6c9dxNoKQHVsLwZ",
		"1Mq2Q1BMicK4ECE6GNR6mDTPdkxwxDe3mc",
		"19ntWSdcFqBRfxdgXm95Q3G9Mviu3Yvwjn",
		"1BrjM9kqGry1cgL7pKutdtGHxiUszfSjND",
		"1KgeSeM9TS1XVsybdvChR7rh3ddHqU57xY",
		"12mwAvJywtQF5rJXHzdBQd4yq8EVD7NZqX",
		"1FtjnusT4cHCEvh2bezA4V95jTh2xBiP4",
		"1CGPxUjd82XNwMk1pJPimzumbxdkneKfZz",
		"1Bgu2w4vBTXDDkpWkjAeomVnRRYASnCaJV",
		"1MHZZ4vTsvtbEACHYDyvtK34qyLaXhcVNV",
		"15cEMwTeSCKgoQqL2sYaSruSnwU8z9dzpt",
		"15BJS6GqnSw5L7UFXUJNCNz6SDyi9vh4s9",
		"17RcRhWHjZnrFCp2pay3YRjyhQYUU5fSAm",
		"1Gq49ewtMAnXTD2nsU4nb3Dj5zpGgMGCQa",
		"13kq3JsH7kw6u6icKmwePHkexHs5UrabF9",
		"13A46EHyLvX6Ss8Kf7hsxSwxqQuM5mdUYL",
		"1MDypxCYTtaCp56Lqfc7BxuDFNsRFK9h5E",
		"18q9J5iMkvU8kgmYm2oShdPPbaLZWHTAns",
		"1591TioQC4j8iUnkEmxMHbhbaH9vZ5xnBo",
		"1AdraNiQ1wAdFUaJ3fcnw5WJGutgK8jXXh",
		"1GLadosEkeAsLReqS3yQ51E1R3wVtbJCDF",
		"1LhhedZHAtYPRcCvNwSedbh335ZAgFMoFq",
		"12wEJ6Tj46JDs8LUDHZrc2bbVHpZoTchNg",
		"1E2efCrpH6cFJBze9VCR7bTTVsrTfrRV2s",
		"1AHpyCoucKqE5FhLSaS1ysHZLM9fyAEQxX",
		"19bfaunL2LB7v3KKmVtK5WbWbEZGVyvTGo",
		"1EB6ubsAGvC7PR2eTUiikpVtXjRXymXDLE",
		"1AXKZp4KU4tA4yPoFtHQPHoUi2DmWgHdEd",
		"12iKAxgiU45J7wpERFvopT4DnXrtxj5Cnt",
		"1QCXT5GDVrCZyAeD2752QuUjM9b1xRaeoJ",
		"1HSWmDg3e1KRnwaWDPutMnNmKqVB2uY6v7",
		"1FxoPFyMtAonhpAg9N7nvWwTkfsXbuUGP9",
		"1Pp6es67oM884HnvCro8Fc3VpRr9YLJG1h",
		"1ADRkScDqBHLMdmJY3211Y5Mk5W4c8GQwt",
		"1ZKrazKLvfxNCYqk4BbboxiPonB9U3Pwh",
		"1LaW6vXiSEKGDqmJgovPho9yLcosGUPE3m",
		"1Q6BQNUyhB3U5DryX8o7MWzDXG7hBc45VH",
		"1G9muwbHvJBbqofznm7ECyXY6fKDGXmhNV",
		"1NJJAWtYEHXBFPuU7vCouGWwtRVmeccd82",
		"196uPpfZD8Dw14gDx5sM8eoBbTDrtNyYoT",
		"19vPUYV7JE45ZP9z11RZCFcBHU1KXpUcNv",
		"16nENzntoWJHBB4Pk1KKwLH2536A5xUuY2",
		"1PHxLcD7w3hGMxZz3LXrmL7zfRdcPh3Asf",
		"1NfXM69erAWPDdNrZ2k2UkuV7HDPc6Ebwk",
		"1DcyjVKb6HyFXLYNLgVtGxXciiydq4c1B1",
		"19KDwaNX9xk6aHaBzt2DjZRk6wLj4vAtfF",
		"13GZwHAPgnvb8z5KrYVmrMTUdTp25Mo7bJ",
		"17Gt94xaBPy6KrNWnDRsbmtzdnBVnbtBzz",
		"1FVwrwLeRgZx1XKJTisyT6EQsvUHDoyHt6",
		"115tTroRo3B9ZDQ6ATJGDCHcNEVbjJoZnF",
	}

	for l := 0; l < 10; l++ {
		for i := range ta {
			//println(ta[i], "...")
			a, e := NewAddrFromString(ta[i])
			if e != nil {
				t.Error("NewAddrFromString caused error", e.Error())
				return
			}

			a3 := NewAddrFromHash160(a.Hash160[:], a.Version)
			if a3.String() != ta[i] {
				t.Error("NewAddrFromHash160 failed")
				return
			}
		}
	}
}

func TestBase58(t *testing.T) {
	d, _ := ioutil.ReadFile("../test/base58_encode_decode.json")
	var vecs [][2]string
	e := json.Unmarshal(d, &vecs)
	if e != nil {
		t.Fatal(e.Error())
		return
	}
	for i := range vecs {
		bin, _ := hex.DecodeString(vecs[i][0])
		str := Encodeb58(bin)
		if str != vecs[i][1] {
			t.Error("Encode mismatch at vector", i, vecs[i][0], vecs[i][1])
		}

		d = Decodeb58(vecs[i][1])
		if !bytes.Equal(bin, d) {
			t.Error("Decode mismatch at vector", i, vecs[i][0], vecs[i][1])
		}
	}
}

func test_one_segwit(t *testing.T, s string, valid bool, ver int, plen int) {
	ad, er := NewAddrFromString(s)
	if valid {
		if ad == nil || er != nil {
			t.Error(s, "- Address not decoded")
		} else {
			if ad.Version != 0 {
				t.Error(s, "- Address Version mismatch", ad.Version, ver)
			}
			if len(ad.Program) != plen {
				t.Error(s, "- Address Program length mismatch", len(ad.Program), plen)
			}
		}
	} else {
		if ad != nil || er == nil {
			t.Error(s, "- Address somehow decoded", ad, er)
		}
	}
}

func test_both_segwit(t *testing.T, s string, valid bool, ver int, plen int) {
	test_one_segwit(t, strings.ToLower(s), valid, ver, plen)
	test_one_segwit(t, strings.ToUpper(s), valid, ver, plen)
}

func TestSegwit(t *testing.T) {
	test_both_segwit(t, "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4", true, 0, 20)
	test_both_segwit(t, "BC1QW508D6QEJXTDG4Y5R3ZARVAYR0C5XW7KV8F3T4", false, 0, 0)
	test_both_segwit(t, "bc1qrp33g0q5c5txsp9arysrx4k6zdkfs4nce4xj0gdcccefvpysxf3qccfmv3", true, 0, 32)
	test_both_segwit(t, "bc1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqzk5jj0", true, 1, 32)
	test_both_segwit(t, "bc1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqh2y7hd", false, 0, 0)
}
