package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

const (
	SECRET      = "test_secret"
	SEED_PASS   = "qwerty12345"
	CONFIG_FILE = "test_wallet.cfg"

	OTHERS = "test_others"
)

func start() error {
	PassSeedFilename = SECRET
	RawKeysFilename = OTHERS
	os.Setenv("GOCOIN_WALLET_CONFIG", CONFIG_FILE)
	return ioutil.WriteFile(SECRET, []byte(SEED_PASS), 0600)
}

func reset_wallet() {
	keys = nil
}

func stop() {
	os.Remove(SECRET)
	os.Remove(OTHERS)
}

func mkwal_check(t *testing.T, exp string) {
	reset_wallet()
	make_wallet()
	if int(keycnt) != len(keys) {
		t.Error("keys - wrong number")
	}
	if keys[keycnt-1].BtcAddr.String() != exp {
		t.Error("Expected address mismatch", keys[keycnt-1].BtcAddr.String(), exp, keycnt)
	}
}

func TestMakeWallet(t *testing.T) {
	defer stop()
	if start() != nil {
		t.Error("start failed")
	}

	keycnt = 300

	// Type-3
	waltype = 3
	testnet = false
	mkwal_check(t, "1M8UbAaJ132nzgWQEhBxhydswWgHpASA2R")

	testnet = true
	mkwal_check(t, "n1eRtDfGp4U3mnz1xGALXtrCoWGzhjrDDr")

	// Type-4 / 0
	testnet = false
	waltype = 4
	hdpath = "m/0'"
	keycnt = 20
	mkwal_check(t, "1FvWLNinb9RfQ4pFanWVMZJKq3DiB817X9")

	// Type-4 / Electrum
	hdpath = "m/0/0"
	mkwal_check(t, "13M4ypZeacDM2rZ62rqG8jZNg1LVRHhSGy")

	bip39wrds = 12
	mkwal_check(t, "1PP9HRai5dfWW8JByuP8jBEeu42b7AFRfR")

	bip39wrds = 15
	mkwal_check(t, "1DvsyQDNhnX1wSWBnaFfhaBCTMAiGAG6ig")

	bip39wrds = 18
	mkwal_check(t, "1BAkYsi4CzAjvgMBUe78QEVYhPJnmkNAyQ")

	bip39wrds = 21
	mkwal_check(t, "192TT86GEBkhRJUT6qD2YPgAVzKRU3f6V6")

	bip39wrds = 24
	mkwal_check(t, "1JRQ1zkTSuWkmVFDtz9A9ErD9x4BNNAYmY")

	// Type-4 / Bitcon Core
	hdpath = "m/0'/0'/0'"
	bip39wrds = 12
	mkwal_check(t, "14iD1SLEFL9SHWoo8WrT9Wa6Mde3b2R79j")

	// Type-4 / Multibit HD, BRD Wallet
	hdpath = "m/0'/0/0"
	bip39wrds = 0
	mkwal_check(t, "1HDTrCbonnRdN6dBmhEDmLstkDxTT6BEQM")

	// Type-4 / 4
	hdpath = "m/44'/0'/0'/0"
	bip39wrds = 0
	mkwal_check(t, "1ABhTNjkFGquAo9Wq8yj2UirN65oUSiKWR")

	// Type-4: BIP84
	hdpath = "m/84'/0'/0'/0/0"
	bip39wrds = 12
	keycnt = 10
	segwit_mode = true
	bech32_mode = true
	mkwal_check(t, "1DCw8Gjgy3pfAh2NWQvgJEXHBjZvB4PAoD")
	if segwit[9].String() != "bc1qsh35g0djgwj7yw6evkhlkajke3twaua30ke3em" {
		t.Error("Expected address mismatch", segwit[9].String())
	}

	segwit_mode = false
	bech32_mode = false
	bip39wrds = 0
}

func import_check(t *testing.T, pk, exp string) {
	ioutil.WriteFile(OTHERS, []byte(fmt.Sprintln(pk, exp+"lab")), 0600)
	reset_wallet()
	make_wallet()
	if int(keycnt)+1 != len(keys) {
		t.Error("keys - wrong number")
	}
	if keys[0].BtcAddr.Extra.Label != exp+"lab" {
		t.Error("Expected label mismatch", keys[0].BtcAddr.String(), exp)
	}

	if keys[0].BtcAddr.String() != exp {
		t.Error("Expected address mismatch", keys[0].BtcAddr.String(), exp)
	}
}

func TestImportPriv(t *testing.T) {
	defer stop()
	if start() != nil {
		t.Error("start failed")
	}

	waltype = 3
	uncompressed = false
	testnet = false
	keycnt = 1

	// compressed key
	import_check(t, "KzAqX6gJsmvZmJjNrHk3UDZrgDytgF88KzE21TnGVXPC6e3zRHGi", "1M8UbAaJ132nzgWQEhBxhydswWgHpASA2R")
	if !keys[0].BtcAddr.IsCompressed() {
		t.Error("Should be compressed")
	}

	// uncompressed key
	import_check(t, "5HqNqndG7xYfJu8KkkJ7AjVUfVsiWxT5AyLUpBsi2Upe5c2WaRj", "1AV28sMrWe81SgBK21o3KjznwUd5dTngnp")
	if keys[0].BtcAddr.IsCompressed() {
		t.Error("Should be uncompressed")
	}
}
