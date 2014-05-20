package btc

import (
	"fmt"
	"encoding/binary"
)

type NetAddr struct {
	Services uint64
	Ip6 [12]byte
	Ip4 [4]byte
	Port uint16
}

func NewNetAddr(b []byte) (na *NetAddr) {
	if len(b) != 26 {
		println("Incorrect input data length", len(b))
		return
	}
	na = new(NetAddr)
	na.Services = binary.LittleEndian.Uint64(b[0:8])
	copy(na.Ip6[:], b[8:20])
	copy(na.Ip4[:], b[20:24])
	na.Port = binary.BigEndian.Uint16(b[24:26])
	return
}

func (a *NetAddr) Bytes() (res []byte) {
	res = make([]byte, 26)
	binary.LittleEndian.PutUint64(res[0:8], a.Services)
	copy(res[8:20], a.Ip6[:])
	copy(res[20:24], a.Ip4[:])
	binary.BigEndian.PutUint16(res[24:26], a.Port)
	return
}


func (a *NetAddr) String() string {
	return fmt.Sprintf("%d.%d.%d.%d:%d", a.Ip4[0], a.Ip4[1], a.Ip4[2], a.Ip4[3], a.Port)
}
