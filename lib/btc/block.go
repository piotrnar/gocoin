package btc

import (
	"sync"
	"bytes"
	"errors"
	"encoding/binary"
)

type Block struct {
	Raw []byte
	Hash *Uint256
	Txs []*Tx
	TxCount, TxOffset int  // Number of transactions and byte offset to the first one
	Trusted bool // if the block is trusted, we do not check signatures and some other things...
	LastKnownHeight uint32

	BlockExtraInfo // If we cache block on disk (between downloading and comitting), this data has to be preserved

	MedianPastTime uint32 // Set in PreCheckBlock() .. last used in PostCheckBlock()

	// These flags are set in BuildTxList() used later (e.g. by script.VerifyTxScript):
	NoWitnessSize int
	BlockWeight uint
	TotalInputs int

	NoWitnessData []byte // This is set by BuildNoWitnessData()
}


type BlockExtraInfo struct {
	VerifyFlags uint32
	Height uint32
}


// tha data may contain only the header (80 bytes)
func NewBlock(data []byte) (bl *Block, er error) {
	if data==nil {
		er = errors.New("nil pointer")
		return
	}
	bl = new(Block)
	bl.Hash = NewSha2Hash(data[:80])
	er = bl.UpdateContent(data)
	return
}


// tha data may contain only the header (80 bytes)
func (bl *Block) UpdateContent(data []byte) error {
	if len(data) < 80 {
		return errors.New("Block too short")
	}
	bl.Raw = data
	if len(data) > 80 {
		bl.TxCount, bl.TxOffset = VLen(data[80:])
		if bl.TxOffset == 0 {
			return errors.New("Block's txn_count field corrupt - RPC_Result:bad-blk-length")
		}
		bl.TxOffset += 80
	}
	return nil
}

func (bl *Block)Version() uint32 {
	return binary.LittleEndian.Uint32(bl.Raw[0:4])
}

func (bl *Block)ParentHash() []byte {
	return bl.Raw[4:36]
}

func (bl *Block)MerkleRoot() []byte {
	return bl.Raw[36:68]
}

func (bl *Block)BlockTime() uint32 {
	return binary.LittleEndian.Uint32(bl.Raw[68:72])
}

func (bl *Block)Bits() uint32 {
	return binary.LittleEndian.Uint32(bl.Raw[72:76])
}


// Parses block's transactions and adds them to the structure, calculating hashes BTW.
// It would be more elegant to use bytes.Reader here, but this solution is ~20% faster.
func (bl *Block) BuildTxList() (e error) {
	if bl.TxCount==0 {
		bl.TxCount, bl.TxOffset = VLen(bl.Raw[80:])
		if bl.TxCount==0 || bl.TxOffset==0 {
			e = errors.New("Block's txn_count field corrupt - RPC_Result:bad-blk-length")
			return
		}
		bl.TxOffset += 80
	}
	bl.Txs = make([]*Tx, bl.TxCount)

	offs := bl.TxOffset

	var wg sync.WaitGroup
	var data2hash, witness2hash []byte

	bl.NoWitnessSize = 80 + VLenSize(uint64(bl.TxCount))
	bl.BlockWeight = 4 * uint(bl.NoWitnessSize)

	for i := 0; i < bl.TxCount; i++ {
		var n int
		bl.Txs[i], n = NewTx(bl.Raw[offs:])
		if bl.Txs[i] == nil || n==0 {
			e = errors.New("NewTx failed")
			break
		}
		bl.Txs[i].Raw = bl.Raw[offs:offs+n]
		bl.Txs[i].Size = uint32(n)
		if i == 0 {
			for _, ou := range bl.Txs[0].TxOut {
				ou.WasCoinbase = true
			}
		} else {
			// Coinbase tx does not have an input
			bl.TotalInputs += len(bl.Txs[i].TxIn)
		}
		if bl.Txs[i].SegWit != nil {
			data2hash = bl.Txs[i].Serialize()
			bl.Txs[i].NoWitSize = uint32(len(data2hash))
			if i>0 {
				witness2hash = bl.Txs[i].Raw
			}
		} else {
			data2hash = bl.Txs[i].Raw
			bl.Txs[i].NoWitSize = bl.Txs[i].Size
			witness2hash = nil
		}
		bl.BlockWeight += uint(3 * bl.Txs[i].NoWitSize + bl.Txs[i].Size)
		bl.NoWitnessSize += len(data2hash)
		wg.Add(1)
		go func(tx *Tx, b, w []byte) {
			tx.Hash.Calc(b) // Calculate tx hash in a background
			if w != nil {
				tx.wTxID.Calc(w)
			}
			wg.Done()
		}(bl.Txs[i], data2hash, witness2hash)
		offs += n
	}

	wg.Wait()

	return
}


// The block data in non-segwit format
func (bl *Block) BuildNoWitnessData() (e error) {
	if bl.TxCount==0 {
		e = bl.BuildTxList()
		if e != nil {
			return
		}
	}
	old_format_block := new(bytes.Buffer)
	old_format_block.Write(bl.Raw[:80])
	WriteVlen(old_format_block, uint64(bl.TxCount))
	for _, tx := range bl.Txs {
		tx.WriteSerialized(old_format_block)
	}
	bl.NoWitnessData = old_format_block.Bytes()
	if bl.NoWitnessSize == 0 {
		bl.NoWitnessSize = len(bl.NoWitnessData)
	} else if bl.NoWitnessSize != len(bl.NoWitnessData) {
		panic("NoWitnessSize corrupt")
	}
	return
}


func GetBlockReward(height uint32) (uint64) {
	return 50e8 >> (height/210000)
}


func (bl *Block) MerkleRootMatch() bool {
	if bl.TxCount==0 {
		return false
	}
	merkle, mutated := bl.GetMerkle()
	return !mutated && bytes.Equal(merkle, bl.MerkleRoot())
}

func (bl *Block) GetMerkle() (res []byte, mutated bool) {
	mtr := make([][32]byte, len(bl.Txs), 3*len(bl.Txs)) // make the buffer 3 times longer as we use append() inside CalcMerkle
	for i, tx := range bl.Txs {
		mtr[i] = tx.Hash.Hash
	}
	res, mutated = CalcMerkle(mtr)
	return
}
