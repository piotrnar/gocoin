package btc

import (
	"errors"
	"bytes"
	"time"
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


func calcHash(h **Uint256, b []byte, ) {
	*h = NewSha2Hash(b[:])
	taskDone <- true
}


func (bl *Block) BuildTxList() (e error) {
	offs := int(80)
	txcnt, n := VLen(bl.Raw[offs:])
	offs += n
	bl.Txs = make([]*Tx, txcnt)

	for i:=0; i<useThreads; i++ {
		taskDone <- false
	}

	for i:=0; i<int(txcnt); i++ {
		_ = <- taskDone // wait if we have too many threads already
		bl.Txs[i], n = NewTx(bl.Raw[offs:])
		bl.Txs[i].Size = uint32(n)
		go calcHash(&bl.Txs[i].Hash, bl.Raw[offs:offs+n])
		offs += n
	}
	
	// Wait for pending hashing to finish...
	for i:=0; i<useThreads; i++ {
		_ = <- taskDone
	}
	return
}


func (bl *Block) CheckBlock() (er error) {
	// Size limits
	if len(bl.Raw)<81 || len(bl.Raw)>1e6 {
		return errors.New("CheckBlock() : size limits failed")
	}

	// Check timestamp (must not be higher than now +2 minutes)
	if int64(bl.BlockTime) > time.Now().Unix() + 2 * 60 * 60 {
		return errors.New("CheckBlock() : block timestamp too far in the future")
	}

	er = bl.BuildTxList()
	if er != nil {
		return
	}
	
	if !bl.Trusted {
		// First transaction must be coinbase, the rest must not be
		if len(bl.Txs)==0 || !bl.Txs[0].IsCoinBase() {
			return errors.New("CheckBlock() : first tx is not coinbase: "+bl.Hash.String())
		}
		for i:=1; i<len(bl.Txs); i++ {
			if bl.Txs[i].IsCoinBase() {
				return errors.New("CheckBlock() : more than one coinbase")
			}
		}

		// Check Merkle Root
		if !bytes.Equal(getMerkel(bl.Txs), bl.MerkleRoot) {
			return errors.New("CheckBlock() : Merkle Root mismatch")
		}
		
		// Check transactions
		for i:=0; i<len(bl.Txs); i++ {
			er = bl.Txs[i].CheckTransaction()
			if er!=nil {
				return errors.New("CheckBlock() : CheckTransaction failed\n"+er.Error())
			}
		}
	}

	return 
}


func GetBlockReward(height uint32) (uint64) {
	return 50e8 >> (height/210000)
}

