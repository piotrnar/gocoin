package main

import (
	"os"
	"fmt"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
)

func main() {
	var buf [48]byte
	if len(os.Args)!=2 {
		fmt.Println("Specify the filename containing UTXO database")
		return
	}
	f, er := os.Open(os.Args[1])
	if er != nil {
		fmt.Println(er.Error())
		return
	}
	er = btc.ReadAll(f, buf[:])
	f.Close()
	if er != nil {
		fmt.Println(er.Error())
		return
	}
	fmt.Println("Last Block Height:", binary.LittleEndian.Uint64(buf[:8]))
	fmt.Println("Last Block Hash:", btc.NewUint256(buf[8:40]).String())
	fmt.Println("Number of UTXO records:", binary.LittleEndian.Uint64(buf[40:48]))
}
