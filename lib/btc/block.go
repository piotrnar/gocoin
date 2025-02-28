package btc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"sync"
	"sync/atomic"

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
	var bl_TotalInputs, bl_PaidTxsVSize uint64
	var bl_OrbTxCnt, bl_OrbTxSize, bl_OrbTxWeight uint64
	bl_NoWitnessSize := 80 + uint64(VLenSize(uint64(bl.TxCount)))
	bl_BlockWeight := 4 * bl_NoWitnessSize

	do_txs := func(txlist []*Tx) {
		var data2hash, witness2hash []byte
		for _, tx := range txlist {
			coinbase := tx == bl.Txs[0]
			tx.Size = uint32(len(tx.Raw))
			if coinbase {
				for _, ou := range bl.Txs[0].TxOut {
					ou.WasCoinbase = true
				}
			} else {
				// Coinbase tx does not have an input
				bl_TotalInputs += uint64(len(tx.TxIn))
			}
			if tx.SegWit != nil {
				data2hash = tx.Serialize()
				tx.NoWitSize = uint32(len(data2hash))
				if !coinbase {
					witness2hash = tx.Raw
				}
			} else {
				data2hash = tx.Raw
				tx.NoWitSize = tx.Size
				witness2hash = nil
			}
			weight := uint64(3*tx.NoWitSize + tx.Size)
			atomic.AddUint64(&bl_BlockWeight, weight)
			if !coinbase {
				atomic.AddUint64(&bl_PaidTxsVSize, uint64(tx.VSize()))
			}
			atomic.AddUint64(&bl_NoWitnessSize, uint64(len(data2hash)))
			if !coinbase {
				if yes, _ := tx.ContainsOrdFile(true); yes {
					atomic.AddUint64(&bl_OrbTxCnt, 1)
					atomic.AddUint64(&bl_OrbTxSize, uint64(tx.Size))
					atomic.AddUint64(&bl_OrbTxWeight, weight)
				}
			}

			if dohash {
				tx.Hash.Calc(data2hash) // Calculate tx hash in a background
				if witness2hash != nil {
					tx.wTxID.Calc(witness2hash)
				}
			}
		}
		wg.Done()
	}

	const TXS_PACK_SIZE = 4096
	var pack_start_idx int
	pack_start_offs := offs
	for i := 0; i < bl.TxCount; i++ {
		var n int
		bl.Txs[i], n = NewTx(bl.Raw[offs:])
		if bl.Txs[i] == nil || n == 0 {
			e = errors.New("NewTx failed")
			bl.Txs = bl.Txs[:i] // make sure we don't leave any nil pointers in bl.Txs
			break
		}
		bl.Txs[i].Raw = bl.Raw[offs : offs+n]
		offs += n
		if offs-pack_start_offs >= TXS_PACK_SIZE {
			wg.Add(1)
			go do_txs(bl.Txs[pack_start_idx : i+1])
			pack_start_offs = offs
			pack_start_idx = i + 1
		}
	}
	if offs > pack_start_offs {
		wg.Add(1)
		go do_txs(bl.Txs[pack_start_idx:])
	}
	wg.Wait()
	bl.NoWitnessSize = int(bl_NoWitnessSize)
	bl.BlockWeight = uint(bl_BlockWeight)
	bl.TotalInputs = int(bl_TotalInputs)
	bl.PaidTxsVSize = uint(bl_PaidTxsVSize)
	bl.OrbTxCnt = uint(bl_OrbTxCnt)
	bl.OrbTxSize = uint(bl_OrbTxSize)
	bl.OrbTxWeight = uint(bl_OrbTxWeight)
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
