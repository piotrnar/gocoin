package btc

import (
	"errors"
	"encoding/binary"
)

type Block struct {
	Raw []byte
	Hash *Uint256
	Txs []*Tx

	Version uint32
	Parent []byte
	MerkleRoot []byte
	BlockTime uint32
	Bits uint32

	// if the block is trusted, we do not check signatures and some other things...
	Trusted bool
}


func NewBlock(data []byte) (*Block, error) {
	if len(data)<81 {
		return nil, errors.New("Block too short")
	}

	var bl Block
	bl.Hash = NewSha2Hash(data[:80])
	bl.Raw = data
	bl.Version = binary.LittleEndian.Uint32(data[0:4])
	bl.Parent = data[4:36]
	bl.MerkleRoot = data[36:68]
	bl.BlockTime = binary.LittleEndian.Uint32(data[68:72])
	bl.Bits = binary.LittleEndian.Uint32(data[72:76])
	return &bl, nil
}


// Parses block's transactions and adds them to the structure, calculating hashes BTW.
// It would be more elegant to use bytes.Reader here, but this solution is ~20% faster.
func (bl *Block) BuildTxList() (e error) {
	offs := int(80)
	txcnt, n := VLen(bl.Raw[offs:])
	if n == 0 {
		e = errors.New("Unexpected end of the block's payload")
	}
	offs += n
	bl.Txs = make([]*Tx, txcnt)

	done := make(chan bool, UseThreads)
	for i:=0; i<UseThreads; i++ {
		done <- false
	}

	for i:=0; i<int(txcnt); i++ {
		bl.Txs[i], n = NewTx(bl.Raw[offs:])
		if bl.Txs[i] == nil || n==0 {
			e = errors.New("NewTx failed")
			break
		}
		bl.Txs[i].Size = uint32(n)
		_ = <- done // wait here, if we have too many threads already
		go func(h **Uint256, b []byte) {
			*h = NewSha2Hash(b)
			done <- true // indicate mission completed
		}(&bl.Txs[i].Hash, bl.Raw[offs:offs+n])
		offs += n
	}

	// Wait for all the pending mission to complete...
	for i:=0; i<UseThreads; i++ {
		_ = <- done
	}
	return
}


func GetBlockReward(height uint32) (uint64) {
	return 50e8 >> (height/210000)
}
