package main

import (
	"os"
	"testing"
	"io/ioutil"
)

const (
	SECRET = "test_secret"
	SEED_PASS = "qwerty12345"

	OTHERS = "test_others"
)


func start() error {
	PassSeedFilename = SECRET
	return ioutil.WriteFile(SECRET, []byte(SEED_PASS), 0600)
}

func reset_wallet() {
	publ_addrs = nil
	compressed_key = nil
	labels = nil
	priv_keys = nil
}

func stop() {
	os.Remove(SECRET)
	os.Remove(OTHERS)
}


func mkwal_check(t *testing.T, exp string) {
	reset_wallet()
	make_wallet()
	if int(keycnt) != len(publ_addrs) {
		t.Error("publ_addrs - wrong number", len(publ_addrs))
	}
	if int(keycnt) != len(compressed_key) {
		t.Error("compressed_key - wrong number")
	}
	if int(keycnt) != len(labels) {
		t.Error("labels - wrong number")
	}
	if int(keycnt) != len(priv_keys) {
		t.Error("priv_keys - wrong number")
	}
	if publ_addrs[keycnt-1].String() != exp {
		t.Error("Expected address mismatch", publ_addrs[keycnt-1].String(), exp)
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
