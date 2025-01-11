package peersdb

import (
	"os"
	"testing"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/others/qdb"
)

func test_one_addr(t *testing.T, host string, ip [4]byte, port uint16) {
	var p *PeerAddr
	var e error
	p, e = NewAddrFromString(host, false)

	if e != nil {
		t.Fatal(e.Error(), host, port)
	}
	if p.Ip4 != ip {
		t.Error("Bad IP4 returned", host, port)
	}
	if p.Services != Services {
		t.Error("Bad Services returned", host, port)
	}
	if p.Port != port {
		t.Error("Bad port returned", host, port)
	}

	p, e = NewAddrFromString(host+":1234", true)
	if e != nil {
		t.Fatal(e.Error(), host, port)
	}
	if p.Ip4 != ip {
		t.Error("Bad IP4 returned", host, port)
	}
	if p.Services != Services {
		t.Error("Bad Services returned", host, port)
	}
	if p.Port != port {
		t.Error("Bad port returned", host, port)
	}

	p, e = NewAddrFromString(host+":1234", false)
	if e != nil {
		t.Fatal(e.Error(), host, port)
	}
	if p.Ip4 != ip {
		t.Error("Bad IP4 returned", host, port)
	}
	if p.Services != Services {
		t.Error("Bad Services returned", host, port)
	}
	if p.Port != 1234 {
		t.Error("Bad port returned", host, port)
	}

	_, e = NewAddrFromString(host+":123456", false)
	if e == nil {
		t.Error("Error expected as port number too high", host, port)
	}

	_, e = NewAddrFromString(host+":123456", true)
	if e != nil {
		t.Error("No Error expected as port number to be ignored", host, port)
	}
}

func TestNewAddrFromString(t *testing.T) {
	PeerDB, _ = qdb.NewDB("tmpdir", true)

	// mainnet
	common.DefaultTcpPort = 8333
	test_one_addr(t, "fi.gocoin.pl", [4]byte{95, 217, 73, 162}, 8333)
	test_one_addr(t, "1.2.3.4", [4]byte{1, 2, 3, 4}, 8333)

	// Testnet
	common.DefaultTcpPort = 18333
	test_one_addr(t, "kaja.gocoin.pl", [4]byte{195, 136, 152, 164}, 18333)
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
	PeerDB.Close()
	os.RemoveAll("tmpdir")
}
