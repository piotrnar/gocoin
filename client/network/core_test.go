package network

import (
	"bytes"
	"encoding/binary"
	"math/rand"
	"testing"

	"github.com/piotrnar/gocoin/client/common"
	"github.com/piotrnar/gocoin/lib/btc"
)

// rawMsg builds a message the way SendRawMsg() is expected to put it into the send buffer.
func rawMsg(cmd string, pl []byte) []byte {
	var hdr [24]byte
	copy(hdr[0:4], common.Magic[:])
	copy(hdr[4:16], cmd)
	binary.LittleEndian.PutUint32(hdr[16:20], uint32(len(pl)))
	sh := btc.Sha2Sum(pl)
	copy(hdr[20:24], sh[:4])
	return append(hdr[:], pl...)
}

// TestSendBufferGrowing checks that the data comes out of the send buffer intact and
// in order, while the buffer grows, wraps around and gets replaced in the middle of
// a write (the consumer below does what writing_thread() does).
func TestSendBufferGrowing(t *testing.T) {
	rnd := rand.New(rand.NewSource(1))
	c := NewConnection(nil)
	if len(c.sendBuf) != SendBufMinSize {
		t.Fatal("unexpected initial size of the send buffer", len(c.sendBuf))
	}
	var expected []byte // what has been sent, but not consumed yet

	send := func(size int) {
		pl := make([]byte, size)
		rnd.Read(pl)
		if er := c.SendRawMsg("test", pl, false); er != nil {
			t.Fatal("SendRawMsg failed:", er)
		}
		expected = append(expected, rawMsg("test", pl)...)
		if c.BytesToSent() != len(expected) {
			t.Fatal("BytesToSent", c.BytesToSent(), "expected", len(expected))
		}
		if l := len(c.sendBuf); l&(l-1) != 0 || l > SendBufSize {
			t.Fatal("bad size of the send buffer", l)
		}
	}
	// pending returns the continuous part of the data, like writing_thread() takes it
	pending := func() []byte {
		end := c.SendBufProd
		if end < c.SendBufCons {
			end = len(c.sendBuf)
		}
		return c.sendBuf[c.SendBufCons:end]
	}
	consumed := func(buf []byte, n int) {
		if !bytes.Equal(buf[:n], expected[:n]) {
			t.Fatal("data taken from the send buffer does not match")
		}
		expected = expected[n:]
		c.SendBufCons = (c.SendBufCons + n) & (len(c.sendBuf) - 1)
	}

	for i := 0; i < 2000; i++ {
		switch rnd.Intn(4) {
		case 0, 1:
			send(rnd.Intn(40000))
		case 2:
			if buf := pending(); len(buf) > 0 {
				consumed(buf, 1+rnd.Intn(len(buf)))
			}
		case 3:
			// the buffer gets enlarged after the writer took the data, but before it is done
			if buf := pending(); len(buf) > 0 && len(c.sendBuf) <= 1<<20 {
				old_size := len(c.sendBuf)
				send(old_size)
				if len(c.sendBuf) == old_size {
					t.Fatal("the send buffer was expected to grow")
				}
				consumed(buf, 1+rnd.Intn(len(buf)))
			}
		}
		// do not let it reach the limit
		for len(expected) > SendBufSize/4 {
			buf := pending()
			consumed(buf, len(buf))
		}
	}
	for len(expected) > 0 {
		buf := pending()
		if len(buf) == 0 {
			t.Fatal("send buffer empty, but", len(expected), "bytes still expected")
		}
		consumed(buf, len(buf))
	}
	if c.BytesToSent() != 0 {
		t.Fatal("send buffer not empty at the end")
	}
}

// TestMakeSendRoom checks when the send buffer grows and where it stops.
func TestMakeSendRoom(t *testing.T) {
	c := NewConnection(nil)
	if !c.makeSendRoom(SendBufMinSize-1) || len(c.sendBuf) != SendBufMinSize {
		t.Fatal("it should fit without growing")
	}
	if !c.makeSendRoom(SendBufMinSize) || len(c.sendBuf) != 2*SendBufMinSize {
		t.Fatal("it should have doubled the buffer")
	}
	if !c.makeSendRoom(SendBufSize-1) || len(c.sendBuf) != SendBufSize {
		t.Fatal("it should have grown to the maximum size")
	}
	if c.makeSendRoom(SendBufSize) {
		t.Fatal("it should not fit in the biggest buffer")
	}
}

// TestExpireMisbehave checks that each record expires after an hour, taking exactly
// its own points away - also when the 16-bit timestamp wraps around in the meantime.
func TestExpireMisbehave(t *testing.T) {
	for _, start := range []int64{1700000000, 0x65000000 + 0xffff - 100, 0x65000000 + 0xffff - 3700} {
		c := NewConnection(nil)
		add := func(now int64, val int) {
			c.misbehave += val
			c.misbehave_history = append(c.misbehave_history, [2]uint16{uint16(now), uint16(val)})
		}
		add(start, 10)
		add(start+100, 200)
		add(start+200, 3)

		check := func(now int64, expected int) {
			c.expire_misbehave(now)
			if c.misbehave != expected {
				t.Fatal("start", start, "after", now-start, "sec: misbehave is", c.misbehave, "expected", expected)
			}
		}
		check(start+3599, 213)
		check(start+3600, 203)
		check(start+3699, 203)
		check(start+3700, 3)
		check(start+3799, 3)
		check(start+3800, 0)
		if c.misbehave_history != nil {
			t.Fatal("history not empty")
		}
	}
}
