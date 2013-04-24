package btc

import (
	"fmt"
	"encoding/binary"
)

type NetAddr struct {
	Services uint64
	//Ip6 string
	Ip4 string
	Port uint16
}

func NewNetAddr(b []byte) (na *NetAddr) {
	if len(b) != 26 {
		println("Incorrect input data length", len(b))
		return
	}
	na = new(NetAddr)
	na.Services = binary.LittleEndian.Uint64(b[0:8])
	na.Ip4 = fmt.Sprintf("%d.%d.%d.%d", b[20], b[21], b[22], b[23])
	/*for i:=0; i<16; i++ {
		na.Ip6 += 
	}
	na.Ip6 = fmt.Sprintf("%x:%x:%x:%x:%x:%x:",
		binary.BigEndian.Uint16(b[8:10]),
		binary.BigEndian.Uint16(b[10:12]),
		binary.BigEndian.Uint16(b[12:14]),
		binary.BigEndian.Uint16(b[14:16]),
		binary.BigEndian.Uint16(b[16:18]),
		binary.BigEndian.Uint16(b[18:20]),
		) + na.Ip4*/
	na.Port = binary.BigEndian.Uint16(b[24:26])
	return
}

func (a *NetAddr) String() string {
	return fmt.Sprint(a.Ip4, ":", a.Port, " services:", a.Services)
}
