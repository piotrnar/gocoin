package utils

import (
	"bytes"
	"encoding/binary"
	"hash/crc64"

	"github.com/piotrnar/gocoin/lib/btc"
)

type OnePeer struct {
	btc.NetAddr
	Time      uint32 // When seen last time
	Banned    uint32 // time when this address baned or zero if never
	SeenAlive bool
	BanReason string
}

var crctab = crc64.MakeTable(crc64.ISO)

/*
Serialized peer record (all values are LSB unless specified otherwise):
 [0:4] - Unix timestamp of when last the peer was seen
 [4:12] - Services
 [12:24] - IPv6 (network order)
 [24:28] - IPv4 (network order)
 [28:30] - TCP port (big endian)
 [30:34] - OPTIONAL:
 	highest bit: set to 1 of peer has been seen "alive"
	low 31 bits: if present, unix timestamp of when the peer was banned divided by 2
 [35] - OPTIONAL flags
    bit(0) - Indicates BanReadon present (byte_len followed by the string)
	bits(1-7) - reserved
*/

func NewPeer(v []byte) (p *OnePeer) {
	if len(v) < 30 {
		println("NewPeer: unexpected length", len(v))
		return
	}
	p = new(OnePeer)
	p.Time = binary.LittleEndian.Uint32(v[0:4])
	p.Services = binary.LittleEndian.Uint64(v[4:12])
	copy(p.Ip6[:], v[12:24])
	copy(p.Ip4[:], v[24:28])
	p.Port = binary.BigEndian.Uint16(v[28:30])
	if len(v) >= 34 {
		xd := binary.LittleEndian.Uint32(v[30:34])
		p.SeenAlive = (xd & 0x80000000) != 0
		p.Banned = (xd & 0x7fffffff) << 1
		if !p.SeenAlive && p.Banned > 1893452400 /*Year 2030*/ {
			// Convert from the old DB - TODO: remove it at some point (now is 14th of July 2021)
			p.Banned >>= 1
		}
		if len(v) >= 35 {
			extra_fields := v[34]
			if (extra_fields&0x01) != 0 && len(v) >= 37 {
				slen := int(v[35])
				if len(v) >= 36+slen {
					p.BanReason = string(v[36 : 36+slen])
				}
			}
		}
	}
	return
}

func (p *OnePeer) Bytes() (res []byte) {
	b := new(bytes.Buffer)
	binary.Write(b, binary.LittleEndian, p.Time)
	binary.Write(b, binary.LittleEndian, p.Services)
	b.Write(p.Ip6[:])
	b.Write(p.Ip4[:])
	binary.Write(b, binary.LittleEndian, p.Port)
	if p.Banned != 0 || p.SeenAlive {
		xd := p.Banned >> 1
		if p.SeenAlive {
			xd |= 0x80000000
		}
		binary.Write(b, binary.LittleEndian, xd)
		if p.BanReason != "" {
			b.Write([]byte{0x01, byte(len(p.BanReason))})
			b.Write([]byte(p.BanReason))
		}

	}
	res = b.Bytes()
	return
}

func (p *OnePeer) UniqID() uint64 {
	h := crc64.New(crctab)
	h.Write(p.Ip6[:])
	h.Write(p.Ip4[:])
	h.Write([]byte{byte(p.Port >> 8), byte(p.Port)})
	return h.Sum64()
}
