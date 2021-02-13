package main

import (
	"os"
	"fmt"
	"testing"
	"io/ioutil"
)

const (
	SECRET = "test_secret"
	SEED_PASS = "qwerty12345"
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
	uncompressed = false
	testnet = false
	mkwal_check(t, "1M8UbAaJ132nzgWQEhBxhydswWgHpASA2R")

	testnet = true
	mkwal_check(t, "n1eRtDfGp4U3mnz1xGALXtrCoWGzhjrDDr")

	uncompressed = true
	mkwal_check(t, "morWAwVM5Btv2v3k3SMgtHFSR6VWgkwukW")

	testnet = false
	mkwal_check(t, "19LYstQNGATfFoa8KsPK4N37Z6tojngQaX")

    // Type-4 / 0
	uncompressed = false
	testnet = false
    waltype = 4
    hdwaltype = 0
	keycnt = 20
	mkwal_check(t, "1FvWLNinb9RfQ4pFanWVMZJKq3DiB817X9")
    
    // Type-4 / 1
    hdwaltype = 1
	mkwal_check(t, "13M4ypZeacDM2rZ62rqG8jZNg1LVRHhSGy")
    
    bip39bits = 128
	mkwal_check(t, "1PP9HRai5dfWW8JByuP8jBEeu42b7AFRfR")
    
    bip39bits = 160
	mkwal_check(t, "1DvsyQDNhnX1wSWBnaFfhaBCTMAiGAG6ig")
    
    bip39bits = 192
	mkwal_check(t, "1BAkYsi4CzAjvgMBUe78QEVYhPJnmkNAyQ")
    
    bip39bits = 224
	mkwal_check(t, "192TT86GEBkhRJUT6qD2YPgAVzKRU3f6V6")
    
    bip39bits = 256
	mkwal_check(t, "1JRQ1zkTSuWkmVFDtz9A9ErD9x4BNNAYmY")
    
    // Type-4 / 2
    hdwaltype = 2
    bip39bits = 128
	mkwal_check(t, "14iD1SLEFL9SHWoo8WrT9Wa6Mde3b2R79j")
    
    // Type-4 / 3
    hdwaltype = 3
    bip39bits = 0
	mkwal_check(t, "1HDTrCbonnRdN6dBmhEDmLstkDxTT6BEQM")
    
    // Type-4 / 4
    hdwaltype = 4
    bip39bits = 0
	mkwal_check(t, "1ABhTNjkFGquAo9Wq8yj2UirN65oUSiKWR")
    
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
