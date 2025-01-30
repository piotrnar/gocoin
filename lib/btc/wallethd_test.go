/*
This code is taken from:
 * https://github.com/WeMeetAgain/go-hdwallet
*/

package btc

import (
	"bytes"
	"encoding/hex"
	"testing"
)

// implements https://github.com/bitcoin/bips/blob/master/bip-0032.mediawiki#test-vectors

var (
	masterhex1                           string = "000102030405060708090a0b0c0d0e0f"
	m_pub1                               string = "xpub661MyMwAqRbcFtXgS5sYJABqqG9YLmC4Q1Rdap9gSE8NqtwybGhePY2gZ29ESFjqJoCu1Rupje8YtGqsefD265TMg7usUDFdp6W1EGMcet8"
	m_prv1                               string = "xprv9s21ZrQH143K3QTDL4LXw2F7HEK3wJUD2nW2nRk4stbPy6cq3jPPqjiChkVvvNKmPGJxWUtg6LnF5kejMRNNU3TGtRBeJgk33yuGBxrMPHi"
	m_0p_pub1                            string = "xpub68Gmy5EdvgibQVfPdqkBBCHxA5htiqg55crXYuXoQRKfDBFA1WEjWgP6LHhwBZeNK1VTsfTFUHCdrfp1bgwQ9xv5ski8PX9rL2dZXvgGDnw"
	m_0p_prv1                            string = "xprv9uHRZZhk6KAJC1avXpDAp4MDc3sQKNxDiPvvkX8Br5ngLNv1TxvUxt4cV1rGL5hj6KCesnDYUhd7oWgT11eZG7XnxHrnYeSvkzY7d2bhkJ7"
	m_0p_1_pub1                          string = "xpub6ASuArnXKPbfEwhqN6e3mwBcDTgzisQN1wXN9BJcM47sSikHjJf3UFHKkNAWbWMiGj7Wf5uMash7SyYq527Hqck2AxYysAA7xmALppuCkwQ"
	m_0p_1_prv1                          string = "xprv9wTYmMFdV23N2TdNG573QoEsfRrWKQgWeibmLntzniatZvR9BmLnvSxqu53Kw1UmYPxLgboyZQaXwTCg8MSY3H2EU4pWcQDnRnrVA1xe8fs"
	m_0p_1_2p_pub1                       string = "xpub6D4BDPcP2GT577Vvch3R8wDkScZWzQzMMUm3PWbmWvVJrZwQY4VUNgqFJPMM3No2dFDFGTsxxpG5uJh7n7epu4trkrX7x7DogT5Uv6fcLW5"
	m_0p_1_2p_prv1                       string = "xprv9z4pot5VBttmtdRTWfWQmoH1taj2axGVzFqSb8C9xaxKymcFzXBDptWmT7FwuEzG3ryjH4ktypQSAewRiNMjANTtpgP4mLTj34bhnZX7UiM"
	m_0p_1_2p_2_pub1                     string = "xpub6FHa3pjLCk84BayeJxFW2SP4XRrFd1JYnxeLeU8EqN3vDfZmbqBqaGJAyiLjTAwm6ZLRQUMv1ZACTj37sR62cfN7fe5JnJ7dh8zL4fiyLHV"
	m_0p_1_2p_2_prv1                     string = "xprvA2JDeKCSNNZky6uBCviVfJSKyQ1mDYahRjijr5idH2WwLsEd4Hsb2Tyh8RfQMuPh7f7RtyzTtdrbdqqsunu5Mm3wDvUAKRHSC34sJ7in334"
	m_0p_1_2p_2_1000000000_pub1          string = "xpub6H1LXWLaKsWFhvm6RVpEL9P4KfRZSW7abD2ttkWP3SSQvnyA8FSVqNTEcYFgJS2UaFcxupHiYkro49S8yGasTvXEYBVPamhGW6cFJodrTHy"
	m_0p_1_2p_2_1000000000_prv1          string = "xprvA41z7zogVVwxVSgdKUHDy1SKmdb533PjDz7J6N6mV6uS3ze1ai8FHa8kmHScGpWmj4WggLyQjgPie1rFSruoUihUZREPSL39UNdE3BBDu76"
	masterhex2                           string = "fffcf9f6f3f0edeae7e4e1dedbd8d5d2cfccc9c6c3c0bdbab7b4b1aeaba8a5a29f9c999693908d8a8784817e7b7875726f6c696663605d5a5754514e4b484542"
	m_pub2                               string = "xpub661MyMwAqRbcFW31YEwpkMuc5THy2PSt5bDMsktWQcFF8syAmRUapSCGu8ED9W6oDMSgv6Zz8idoc4a6mr8BDzTJY47LJhkJ8UB7WEGuduB"
	m_prv2                               string = "xprv9s21ZrQH143K31xYSDQpPDxsXRTUcvj2iNHm5NUtrGiGG5e2DtALGdso3pGz6ssrdK4PFmM8NSpSBHNqPqm55Qn3LqFtT2emdEXVYsCzC2U"
	m_0_pub2                             string = "xpub69H7F5d8KSRgmmdJg2KhpAK8SR3DjMwAdkxj3ZuxV27CprR9LgpeyGmXUbC6wb7ERfvrnKZjXoUmmDznezpbZb7ap6r1D3tgFxHmwMkQTPH"
	m_0_prv2                             string = "xprv9vHkqa6EV4sPZHYqZznhT2NPtPCjKuDKGY38FBWLvgaDx45zo9WQRUT3dKYnjwih2yJD9mkrocEZXo1ex8G81dwSM1fwqWpWkeS3v86pgKt"
	m_0_2147483647p_pub2                 string = "xpub6ASAVgeehLbnwdqV6UKMHVzgqAG8Gr6riv3Fxxpj8ksbH9ebxaEyBLZ85ySDhKiLDBrQSARLq1uNRts8RuJiHjaDMBU4Zn9h8LZNnBC5y4a"
	m_0_2147483647p_prv2                 string = "xprv9wSp6B7kry3Vj9m1zSnLvN3xH8RdsPP1Mh7fAaR7aRLcQMKTR2vidYEeEg2mUCTAwCd6vnxVrcjfy2kRgVsFawNzmjuHc2YmYRmagcEPdU9"
	m_0_2147483647p_1_pub2               string = "xpub6DF8uhdarytz3FWdA8TvFSvvAh8dP3283MY7p2V4SeE2wyWmG5mg5EwVvmdMVCQcoNJxGoWaU9DCWh89LojfZ537wTfunKau47EL2dhHKon"
	m_0_2147483647p_1_prv2               string = "xprv9zFnWC6h2cLgpmSA46vutJzBcfJ8yaJGg8cX1e5StJh45BBciYTRXSd25UEPVuesF9yog62tGAQtHjXajPPdbRCHuWS6T8XA2ECKADdw4Ef"
	m_0_2147483647p_1_2147483646p_pub2   string = "xpub6ERApfZwUNrhLCkDtcHTcxd75RbzS1ed54G1LkBUHQVHQKqhMkhgbmJbZRkrgZw4koxb5JaHWkY4ALHY2grBGRjaDMzQLcgJvLJuZZvRcEL"
	m_0_2147483647p_1_2147483646p_prv2   string = "xprvA1RpRA33e1JQ7ifknakTFpgNXPmW2YvmhqLQYMmrj4xJXXWYpDPS3xz7iAxn8L39njGVyuoseXzU6rcxFLJ8HFsTjSyQbLYnMpCqE2VbFWc"
	m_0_2147483647p_1_2147483646p_2_pub2 string = "xpub6FnCn6nSzZAw5Tw7cgR9bi15UV96gLZhjDstkXXxvCLsUXBGXPdSnLFbdpq8p9HmGsApME5hQTZ3emM2rnY5agb9rXpVGyy3bdW6EEgAtqt"
	m_0_2147483647p_1_2147483646p_2_prv2 string = "xprvA2nrNbFZABcdryreWet9Ea4LvTJcGsqrMzxHx98MMrotbir7yrKCEXw7nadnHM8Dq38EGfSh6dqA9QWTyefMLEcBYJUuekgW4BYPJcr9E7j"
)

func testChild(t *testing.T, key, ref_key string, i uint32) {
	child_key := StringChild(key, i)
	if child_key != ref_key {
		t.Errorf("\n%s\nsupposed to be\n%s", child_key, ref_key)
	}
}

func testMasterKey(t *testing.T, seed []byte, ref_key string) {
	masterprv := MasterKey(seed, false).String()
	if masterprv != ref_key {
		t.Errorf("\n%s\nsupposed to be\n%s", masterprv, ref_key)
	}
}

func testPub(t *testing.T, prv, ref_pub string) {
	w, err := StringWallet(prv)
	if err != nil {
		t.Errorf("%s should have been nil", err.Error())
	}
	pub := w.Pub().String()
	if pub != ref_pub {
		t.Errorf("\n%s\nsupposed to be\n%s", pub, ref_pub)
	}
}

func TestVector1(t *testing.T) {
	seed, _ := hex.DecodeString(masterhex1)
	t.Logf("master key")
	testMasterKey(t, seed, m_prv1)
	t.Logf("master key -> pub")
	testPub(t, m_prv1, m_pub1)
	var i uint32
	i = 0x80000000
	t.Logf("first child")
	testChild(t, m_prv1, m_0p_prv1, i)
	t.Logf("first child -> pub")
	testPub(t, m_0p_prv1, m_0p_pub1)
	t.Logf("second child")
	testChild(t, m_0p_prv1, m_0p_1_prv1, 1)
	t.Logf("second child -> pub")
	testPub(t, m_0p_1_prv1, m_0p_1_pub1)
	t.Logf("third child")
	i = 0x80000002
	testChild(t, m_0p_1_prv1, m_0p_1_2p_prv1, i)
	t.Logf("third child -> pub")
	testPub(t, m_0p_1_2p_prv1, m_0p_1_2p_pub1)
	t.Logf("fourth child")
	testChild(t, m_0p_1_2p_prv1, m_0p_1_2p_2_prv1, 2)
	t.Logf("fourth child -> pub")
	testPub(t, m_0p_1_2p_2_prv1, m_0p_1_2p_2_pub1)
	t.Logf("fifth child")
	i = 1000000000 % 0x80000000
	testChild(t, m_0p_1_2p_2_prv1, m_0p_1_2p_2_1000000000_prv1, i)
	t.Logf("fifth child -> pub")
	testPub(t, m_0p_1_2p_2_1000000000_prv1, m_0p_1_2p_2_1000000000_pub1)
}

func TestVector2(t *testing.T) {
	seed, _ := hex.DecodeString(masterhex2)
	t.Logf("master key")
	testMasterKey(t, seed, m_prv2)
	t.Logf("master key -> pub")
	testPub(t, m_prv2, m_pub2)
	var i uint32
	t.Logf("first child")
	testChild(t, m_prv2, m_0_prv2, 0)
	t.Logf("first child -> pub")
	testPub(t, m_0_prv2, m_0_pub2)
	i = 2147483647 + 0x80000000
	t.Logf("second child")
	testChild(t, m_0_prv2, m_0_2147483647p_prv2, i)
	t.Logf("second child -> pub")
	testPub(t, m_0_2147483647p_prv2, m_0_2147483647p_pub2)
	t.Logf("third child")
	testChild(t, m_0_2147483647p_prv2, m_0_2147483647p_1_prv2, 1)
	t.Logf("third child -> pub")
	testPub(t, m_0_2147483647p_1_prv2, m_0_2147483647p_1_pub2)
	i = 2147483646 + 0x80000000
	t.Logf("fourth child")
	testChild(t, m_0_2147483647p_1_prv2, m_0_2147483647p_1_2147483646p_prv2, i)
	t.Logf("fourth child -> pub")
	testPub(t, m_0_2147483647p_1_2147483646p_prv2, m_0_2147483647p_1_2147483646p_pub2)
	t.Logf("fifth child")
	testChild(t, m_0_2147483647p_1_2147483646p_prv2, m_0_2147483647p_1_2147483646p_2_prv2, 2)
	t.Logf("fifth child -> pub")
	testPub(t, m_0_2147483647p_1_2147483646p_2_prv2, m_0_2147483647p_1_2147483646p_2_pub2)
}

func TestChildPub(t *testing.T) {
	testChild(t, m_pub2, m_0_pub2, 0)
}

func TestChildPrv(t *testing.T) {
	testChild(t, m_prv2, m_0_prv2, 0)
}

func TestSerialize(t *testing.T) {
	w, err := StringWallet(m_prv2)
	if err != nil {
		t.Errorf("%s should have been nil", err.Error())
	}
	if m_prv2 != w.String() {
		t.Errorf("private key not de/reserializing properly")
	}
	w, err = StringWallet(m_pub2)
	if err != nil {
		t.Errorf("%s should have been nil", err.Error())
	}
	if m_pub2 != w.String() {
		t.Errorf("public key not de/reserializing properly")
	}
}

// Used this site to create test http://gobittest.appspot.com/Address
// Public key: 04CBCAA9C98C877A26977D00825C956A238E8DDDFBD322CCE4F74B0B5BD6ACE4A77BD3305D363C26F82C1E41C667E4B3561C06C60A2104D2B548E6DD059056AA51
// Expected address: 1AEg9dFEw29kMgaN4BNHALu7AzX5XUfzSU
func TestAddress(t *testing.T) {
	addr, err := StringAddress(m_pub2)
	if err != nil {
		t.Errorf("%s should have been nil", err.Error())
	}
	expected_addr := "1JEoxevbLLG8cVqeoGKQiAwoWbNYSUyYjg"
	if addr != expected_addr {
		t.Errorf("\n%s\nshould be\n%s", addr, expected_addr)
	}
}

func TestStringCheck(t *testing.T) {
	if err := StringCheck(m_pub2); err != nil {
		t.Errorf("%s should have been nil", err.Error())
	}
	if err := StringCheck(m_prv2); err != nil {
		t.Errorf("%s should have been nil", err.Error())
	}
}

func TestChildren(t *testing.T) {
	hdwal := MasterKey([]byte("Random seed"), false)
	hdpub := hdwal.Pub()

	for i := 0; i < 1000; i++ {
		prv := hdwal.Child(uint32(i | 0x80000000))
		if len(prv.Key) != 33 || prv.Key[0] != 0 {
			t.Error("Bad private derivated key", i)
		}

		prv = hdwal.Child(uint32(i))
		pub := hdpub.Child(uint32(i))
		if len(prv.Key) != 33 || prv.Key[0] != 0 {
			t.Error("Bad private key", i)
		}
		if len(pub.Key) != 33 || (pub.Key[0] != 2 && pub.Key[0] != 3) {
			t.Error("Bad public key", i)
		}
		pu2 := PublicFromPrivate(prv.Key[1:], true)
		if !bytes.Equal(pub.Key, pu2) {
			t.Error("Private/public mismatch on Child", i)
		}

		var p [32]byte
		copy(p[:], prv.Key[1:])
		pu2 = PublicFromPrivate(p[:], true)
		if !bytes.Equal(pub.Key, pu2) {
			t.Error("Private/public other mismatch on Child", i)
		}
	}
}

// benchmarks

func BenchmarkStringChildPub(b *testing.B) {
	for i := 0; i < b.N; i++ {
		StringChild(m_pub2, 0)
	}
}

func BenchmarkStringChildPrv(b *testing.B) {
	a := uint32(0x80000000)
	for i := 0; i < b.N; i++ {
		StringChild(m_prv1, a)
	}
}

func BenchmarkStringPubString(b *testing.B) {
	for i := 0; i < b.N; i++ {
		w, _ := StringWallet(m_prv2)
		_ = w.Pub().String()
	}
}

func BenchmarkStringAddress(b *testing.B) {
	for i := 0; i < b.N; i++ {
		StringAddress(m_pub2)
	}
}

func BenchmarkStringCheck(b *testing.B) {
	for i := 0; i < b.N; i++ {
		StringCheck(m_pub2)
	}
}
