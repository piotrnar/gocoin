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
	Nonce uint32
	TxCount, TxOffset int  // Number of transactions and byte offset to the first one

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
	bl.Nonce = binary.LittleEndian.Uint32(data[76:80])
	bl.TxCount, bl.TxOffset = VLen(data[80:])
	if bl.TxOffset == 0 {
		return nil, errors.New("Block's txn_count field corrupt")
	}
	bl.TxOffset += 80
	return &bl, nil
}


// Parses block's transactions and adds them to the structure, calculating hashes BTW.
// It would be more elegant to use bytes.Reader here, but this solution is ~20% faster.
func (bl *Block) BuildTxList() (e error) {
	bl.Txs = make([]*Tx, bl.TxCount)

	offs := bl.TxOffset

	done := make(chan bool, UseThreads)
	for i:=0; i<UseThreads; i++ {
		done <- false
	}

	for i:=0; i<bl.TxCount; i++ {
		var n int
		bl.Txs[i], n = NewTx(bl.Raw[offs:])
		if bl.Txs[i] == nil || n==0 {
			e = errors.New("NewTx failed")
			break
		}
		bl.Txs[i].Size = uint32(n)
		_ = <- done // wait here, if we have too many threads already
		go func(h **Uint256, b []byte) {
			*h = NewSha2Hash(b) // Calculate tx hash in a background
			done <- true // indicate mission completed
		}(&bl.Txs[i].Hash, bl.Raw[offs:offs+n])
		offs += n
	}

	// Wait for all the pending missions to complete...
	for i:=0; i<UseThreads; i++ {
		_ = <- done
	}
	return
}


func GetBlockReward(height uint32) (uint64) {
	return 50e8 >> (height/210000)
}
