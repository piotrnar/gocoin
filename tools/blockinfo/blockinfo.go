package main

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/piotrnar/gocoin/lib/btc"
)

func main() {
	if len(os.Args) != 2 {
		println("Specify filename with raw block data")
		return
	}
	d, er := os.ReadFile(os.Args[1])
	if er != nil {
		println("Error opening file:", er.Error())
		return
	}
	bl, er := btc.NewBlock(d)
	if er != nil {
		println("Error decoding block:", er.Error())
		return
	}
	fmt.Println("Block Version:", bl.Version())
	fmt.Println("Parent Hash:", btc.NewUint256(bl.ParentHash()).String())
	fmt.Println("Merkle Root:", btc.NewUint256(bl.MerkleRoot()).String(), "   OK:", bl.MerkleRootMatch())
	fmt.Println("Time Stamp:", time.Unix(int64(bl.BlockTime()), 0).Format("2006-01-02 15:04:05"))
	fmt.Println("Nonce:", binary.LittleEndian.Uint32(bl.Raw[72:76]))
	bits := bl.Bits()
	var nul [32]byte
	target := btc.SetCompact(bits)
	bts := target.Bytes()
	fmt.Printf("Bits: 0x%08x  =>  Diff:%.1f  / Target:%s", bits, btc.GetDifficulty(bits), hex.EncodeToString(append(nul[:32-len(bts)], bts...)))

}
