package network

import (
	"encoding/binary"
	"testing"

	"github.com/piotrnar/gocoin/client/peersdb"
)

// baseVersionPayload builds the fixed first 80 bytes of a version message
// (version, services, timestamp, addr_recv, addr_from, nonce). The bytes past
// the nonce (the user-agent var_string and beyond) are appended by the caller.
func baseVersionPayload() []byte {
	pl := make([]byte, 80)
	binary.LittleEndian.PutUint32(pl[0:4], 70016) // protocol version
	// leave services / timestamp / addresses zeroed - not relevant here
	// a non-zero nonce so we don't trip the "NullNonce" path if it were reached
	pl[72] = 0x01
	return pl
}

// TestHandleVersionMalformedUserAgentLen feeds version messages whose user-agent
// var_int length is hostile: values with bit 63 set used to decode to a negative
// int, bypass the length guard, and panic at pl[of : of+le] (high < low). A
// single such message could be sent by any unauthenticated peer as its very
// first message, so this must return an error and never panic.
func TestHandleVersionMalformedUserAgentLen(t *testing.T) {
	evil := [][]byte{
		{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}, // 2^64-1
		{0xff, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x80}, // 2^63
		{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0x7f}, // 2^63-1 (huge, but positive)
		{0xfe, 0xff, 0xff, 0xff, 0xff},                         // 4G claimed, tiny buffer
		{0xfd, 0xff, 0xff},                                     // 65535 claimed, tiny buffer
	}
	for i, ua := range evil {
		c := &OneConnection{PeerAddr: &peersdb.PeerAddr{}}
		pl := append(baseVersionPayload(), ua...)
		// Must return an error and, crucially, must not panic.
		if err := c.HandleVersion(pl); err == nil {
			t.Fatalf("case %d: expected an error for malformed user-agent length, got nil", i)
		}
	}
}

// TestHandleVersionShortPayloads makes sure truncated messages are rejected
// cleanly rather than panicking on an out-of-range read.
func TestHandleVersionShortPayloads(t *testing.T) {
	full := append(baseVersionPayload(), 0x00) // 81 bytes: fixed part + one spare
	for l := 0; l < len(full); l++ {
		c := &OneConnection{PeerAddr: &peersdb.PeerAddr{}}
		// Any panic here fails the test; short messages either return an error
		// (len<80) or parse the fixed part and stop (80 or 81 bytes present).
		_ = c.HandleVersion(full[:l])
	}
}
