package peersdb

import (
	"testing"
)

func test_one_addr(t *testing.T, host string, ip [4]byte, port uint16) {
	var p *PeerAddr
	var e error
	p, e = NewAddrFromString(host, false)

	if e != nil {
		t.Fatal(e.Error(), host, port, Testnet)
	}
	if p.Ip4 != ip {
		t.Error("Bad IP4 returned", host, port, Testnet)
	}
	if p.Services != Services {
		t.Error("Bad Services returned", host, port, Testnet)
	}
	if p.Port != port {
		t.Error("Bad port returned", host, port, Testnet)
	}

	p, e = NewAddrFromString(host+":1234", true)
	if e != nil {
		t.Fatal(e.Error(), host, port, Testnet)
	}
	if p.Ip4 != ip {
		t.Error("Bad IP4 returned", host, port, Testnet)
	}
	if p.Services != Services {
		t.Error("Bad Services returned", host, port, Testnet)
	}
	if p.Port != port {
		t.Error("Bad port returned", host, port, Testnet)
	}

	p, e = NewAddrFromString(host+":1234", false)
	if e != nil {
		t.Fatal(e.Error(), host, port, Testnet)
	}
	if p.Ip4 != ip {
		t.Error("Bad IP4 returned", host, port, Testnet)
	}
	if p.Services != Services {
		t.Error("Bad Services returned", host, port, Testnet)
	}
	if p.Port != 1234 {
		t.Error("Bad port returned", host, port, Testnet)
	}

	p, e = NewAddrFromString(host+":123456", false)
	if e == nil {
		t.Error("Error expected as port number too high", host, port, Testnet)
	}

	p, e = NewAddrFromString(host+":123456", true)
	if e != nil {
		t.Error("No Error expected as port number to be ignored", host, port, Testnet)
	}
}

func TestNewAddrFromString(t *testing.T) {

	// mainnet
	Testnet = false
	test_one_addr(t, "ssdn.gocoin.pl", [4]byte{172, 93, 52, 164}, 8333)
	test_one_addr(t, "1.2.3.4", [4]byte{1, 2, 3, 4}, 8333)

	// Testnet
	Testnet = true
	test_one_addr(t, "ssdn.gocoin.pl", [4]byte{172, 93, 52, 164}, 18333)
	test_one_addr(t, "255.254.253.252", [4]byte{255, 254, 253, 252}, 18333)

	var e error
	_, e = NewAddrFromString("1.2.3.4.5", false)
	if e == nil {
		println("error expected")
	}

	_, e = NewAddrFromString("1.2.3.256", false)
	if e == nil {
		println("error expected")
	}
}
