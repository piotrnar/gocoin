package btc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sync"

	"github.com/piotrnar/gocoin/lib/others/sys"
)

type Block struct {
	Raw               []byte
	Hash              *Uint256
	Txs               []*Tx
	TxCount, TxOffset int          // Number of transactions and byte offset to the first one
	Trusted           sys.SyncBool // if the block is trusted, we do not check signatures and some other things...
	LastKnownHeight   uint32

	BlockExtraInfo // If we cache block on disk (between downloading and comitting), this data has to be preserved

	MedianPastTime uint32 // Set in PreCheckBlock() .. last used in PostCheckBlock()

	// These flags are set in BuildTxList() used later (e.g. by script.VerifyTxScript):
	NoWitnessSize int
	BlockWeight   uint
	PaidTxsVSize  uint
	TotalInputs   int

	OrbTxCnt    uint
	OrbTxSize   uint
	OrbTxWeight uint

	NoWitnessData []byte // This is set by BuildNoWitnessData()
}

type BlockExtraInfo struct {
	VerifyFlags uint32
	Height      uint32
}

func NewBlockX(data []byte, hash *Uint256) (bl *Block, er error) {
	bl = new(Block)
	bl.Hash = hash
	er = bl.UpdateContent(data)
	return
}

// tha data may contain only the header (80 bytes)
func NewBlock(data []byte) (bl *Block, er error) {
	if data == nil {
		er = errors.New("nil pointer")
		return
	}
	return NewBlockX(data, NewSha2Hash(data[:80]))
}

// clean all the transactions
func (bl *Block) Clean() {
	for _, t := range bl.Txs[1:] {
		t.Clean()
	}
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
			return errors.New("block's txn_count field corrupt - RPC_Result:bad-blk-length")
		}
		bl.TxOffset += 80
	}
	return nil
}

func (bl *Block) Version() uint32 {
	return binary.LittleEndian.Uint32(bl.Raw[0:4])
}

func (bl *Block) ParentHash() []byte {
	return bl.Raw[4:36]
}

func (bl *Block) MerkleRoot() []byte {
	return bl.Raw[36:68]
}

func (bl *Block) BlockTime() uint32 {
	return binary.LittleEndian.Uint32(bl.Raw[68:72])
}

func (bl *Block) Bits() uint32 {
	return binary.LittleEndian.Uint32(bl.Raw[72:76])
}

// BuildTxListExt parses a block's transactions and adds them to the structure.
//
//	dohash - set it to false if you do not ened TxIDs (it is much faster then)
//
// returns error if block data inconsistent
func (bl *Block) BuildTxListExt(dohash bool) (e error) {
	if bl.TxCount == 0 {
		bl.TxCount, bl.TxOffset = VLen(bl.Raw[80:])
		if bl.TxCount == 0 || bl.TxOffset == 0 {
			e = errors.New("block's txn_count field corrupt - RPC_Result:bad-blk-length")
			return
		}
		bl.TxOffset += 80
	}
	bl.Txs = make([]*Tx, bl.TxCount)

	// It would be more elegant to use bytes.Reader here, but this solution is ~20% faster.
	offs := bl.TxOffset

	var wg sync.WaitGroup
	var data2hash, witness2hash []byte

	bl.NoWitnessSize = 80 + VLenSize(uint64(bl.TxCount))
	bl.BlockWeight = 4 * uint(bl.NoWitnessSize)

	for i := 0; i < bl.TxCount; i++ {
		var n int
		bl.Txs[i], n = NewTx(bl.Raw[offs:])
		if bl.Txs[i] == nil || n == 0 {
			e = errors.New("NewTx failed")
			bl.Txs = bl.Txs[:i] // make sure we don't leave any nil pointers in bl.Txs
			break
		}
		bl.Txs[i].Raw = bl.Raw[offs : offs+n]
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
			if i > 0 {
				witness2hash = bl.Txs[i].Raw
			}
		} else {
			data2hash = bl.Txs[i].Raw
			bl.Txs[i].NoWitSize = bl.Txs[i].Size
			witness2hash = nil
		}
		weight := uint(3*bl.Txs[i].NoWitSize + bl.Txs[i].Size)
		bl.BlockWeight += weight
		if i > 0 {
			bl.PaidTxsVSize += uint(bl.Txs[i].VSize())
		}
		bl.NoWitnessSize += len(data2hash)
		if i != 0 {
			if yes, _ := bl.Txs[i].ContainsOrdFile(true); yes {
				bl.OrbTxCnt++
				bl.OrbTxSize += uint(n)
				bl.OrbTxWeight += weight
			}
		}

		if dohash {
			wg.Add(1)
			go func(tx *Tx, b, w []byte) {
				tx.Hash.Calc(b) // Calculate tx hash in a background
				if w != nil {
					tx.wTxID.Calc(w)
				}
				wg.Done()
			}(bl.Txs[i], data2hash, witness2hash)
		}
		offs += n
	}

	wg.Wait()

	return
}

// BuildTxList parses a block's transactions and adds them to the structure, always calculating TX IDs.
func (bl *Block) BuildTxList() (e error) {
	return bl.BuildTxListExt(true)
}

// The block data in non-segwit format
func (bl *Block) BuildNoWitnessData() (e error) {
	if bl.TxCount == 0 {
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

func GetBlockReward(height uint32) uint64 {
	return 50e8 >> (height / 210000)
}

func (bl *Block) MerkleRootMatch() bool {
	if bl.TxCount == 0 || len(bl.Txs) != bl.TxCount {
		return false
	}
	merkle, mutated := bl.GetMerkle()
	return !mutated && bytes.Equal(merkle, bl.MerkleRoot())
}

func (bl *Block) GetMerkle() (res []byte, mutated bool) {
	mtr := make([][32]byte, len(bl.Txs), 3*len(bl.Txs)) // make the buffer 3 times longer as we use append() inside CalcMerkle
	for i, tx := range bl.Txs {
		if tx == nil {
			println("GetMerkle(): tx missing", i)
			mutated = true
			return
		}
		mtr[i] = tx.Hash.Hash
	}
	res, mutated = CalcMerkle(mtr)
	return
}
