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
	type2_secret = nil
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
		t.Error("Expected address mismatch", keys[keycnt-1].BtcAddr.String(), exp)
	}
}


func TestMakeWallet(t *testing.T) {
	defer stop()
	if start() != nil {
		t.Error("start failed")
	}

	keycnt = 300

	// Type-1
	waltype = 1
	uncompressed = false
	testnet = false
	mkwal_check(t, "1DkMmYRVUXvjR1QkrWQTQCgMvaApewxU43")

	testnet = true
	mkwal_check(t, "mtGK4bWUHZMzC7tNa5NqE7tgnZmXaYtpdy")

	uncompressed = true
	mkwal_check(t, "mifm3evqJAgknC5WnK8Cq6xs1riR5oEcpT")

	testnet = false
	mkwal_check(t, "149okbqrV9FW15bu4k9q1BkY9s7iE2ny2Y")

	// Type-2
	waltype = 2
	uncompressed = false
	testnet = false
	mkwal_check(t, "12jYVgCNDB63t3J8HhtBwQzs5Qjcu5G6j4")

	testnet = true
	mkwal_check(t, "mhFVnjHM2CXJf9mk1GrZmLDBwQLKn65QNw")

	uncompressed = true
	mkwal_check(t, "mmPAAMPpuSqvkBs6oYFbN5E9fQPwRFYggW")

	testnet = false
	mkwal_check(t, "16sCsJJr6RQfy5PV5yHDYA1poQoEbRwA7F")

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
