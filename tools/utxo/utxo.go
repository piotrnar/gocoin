package main

import (
	"encoding/binary"
	"fmt"
	"github.com/piotrnar/gocoin/lib/btc"
	"io"
	"os"
)

func main() {
	var buf [48]byte
	if len(os.Args) != 2 {
		fmt.Println("Specify the filename containing UTXO database")
		return
	}
	f, er := os.Open(os.Args[1])
	if er != nil {
		fmt.Println(er.Error())
		return
	}
	_, er = io.ReadFull(f, buf[:])
	f.Close()
	if er != nil {
		fmt.Println(er.Error())
		return
	}
	u64 := binary.LittleEndian.Uint64(buf[:8])
	if (u64 & 0x8000000000000000) != 0 {
		fmt.Println("Records: Compressed")
	} else {
		fmt.Println("Records: Not compressed")
	}
	fmt.Println("Last Block Height:", uint32(u64))
	fmt.Println("Last Block Hash:", btc.NewUint256(buf[8:40]).String())
	fmt.Println("Number of UTXO records:", binary.LittleEndian.Uint64(buf[40:48]))
}
