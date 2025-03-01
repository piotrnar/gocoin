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
	TxCount, TxOffset int // Number of transactions and byte offset to the first one
	TotalInputs       int
	BlockWeight       uint
	Trusted           sys.SyncBool // if the block is trusted, we do not check signatures and some other things...
	BlockExtraInfo                 // If we cache block on disk (between downloading and comitting), this data has to be preserved
	LastKnownHeight   uint32
	MedianPastTime    uint32 // Set in PreCheckBlock() .. last used in PostCheckBlock()
}

type BlockUserInfo struct {
	// These flags are set in BuildTxList() used later (e.g. by script.VerifyTxScript):
	NoWitnessSize int
	PaidTxsWeight uint

	OrbTxCnt    uint
	OrbTxSize   uint
	OrbTxWeight uint
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

	offs := bl.TxOffset
	if !dohash {
		bl.BlockWeight = 4 * (80 + uint(VLenSize(uint64(bl.TxCount))))
		for i := 0; i < bl.TxCount; i++ {
			tx, n := NewTx(bl.Raw[offs:])
			if tx == nil || n == 0 {
				e = errors.New("NewTx failed")
				bl.Txs = bl.Txs[:i] // make sure we don't leave any nil pointers in bl.Txs
				break
			}
			tx.Raw = bl.Raw[offs : offs+n]
			tx.Size = uint32(len(tx.Raw))
			bl.TotalInputs += len(tx.TxIn)
			if i == 0 {
				for _, ou := range tx.TxOut {
					ou.WasCoinbase = true
				}
			}
			bl.BlockWeight += uint(3*tx.NoWitSize + tx.Size)
			bl.Txs[i] = tx
			offs += n
		}
		return
	}

	var wg sync.WaitGroup
	block_weight := 4 * (80 + uint64(VLenSize(uint64(bl.TxCount))))
	do_txs := func(txlist []*Tx) {
		for _, tx := range txlist {
			coinbase := tx == bl.Txs[0]
			tx.Size = uint32(len(tx.Raw))
			if coinbase {
				for _, ou := range tx.TxOut {
					ou.WasCoinbase = true
				}
			}
			var data2hash, witness2hash []byte
			if tx.SegWit != nil {
				data2hash = tx.Serialize()
				if tx.NoWitSize != uint32(len(data2hash)) {
					panic("tx.NoWitSize != len(data2hash)")
				}
				if !coinbase {
					witness2hash = tx.Raw
				}
			} else {
				data2hash = tx.Raw
				if tx.NoWitSize != tx.Size {
					panic("tx.NoWitSize != tx.Size")
				}
				witness2hash = nil
			}
			tx.Hash.Calc(data2hash) // Calculate tx hash in a background
			if witness2hash != nil {
				tx.wTxID.Calc(witness2hash)
			}
			weight := uint64(3*tx.NoWitSize + tx.Size)
			atomic.AddUint64(&block_weight, weight)
		}
		wg.Done()
	}

	const TXS_PACK_SIZE = 4096
	var pack_start_idx int
	pack_start_offs := offs
	for i := 0; i < bl.TxCount; i++ {
		tx, n := NewTx(bl.Raw[offs:])
		if tx == nil || n == 0 {
			e = errors.New("NewTx failed")
			bl.Txs = bl.Txs[:i] // make sure we don't leave any nil pointers in bl.Txs
			break
		}
		tx.Raw = bl.Raw[offs : offs+n]
		bl.TotalInputs += len(tx.TxIn)
		bl.Txs[i] = tx
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
	bl.BlockWeight = uint(block_weight)
	return
}

func (bl *Block) GetUserInfo() (res *BlockUserInfo) {
	res = new(BlockUserInfo)
	res.NoWitnessSize = 80 + VLenSize(uint64(bl.TxCount))
	for idx, tx := range bl.Txs {
		coinbase := idx == 0
		tx.Size = uint32(len(tx.Raw))
		res.NoWitnessSize += int(tx.NoWitSize)
		if !coinbase {
			weight := uint64(3*tx.NoWitSize + tx.Size)
			res.PaidTxsWeight += uint(weight)
			if yes, _ := tx.ContainsOrdFile(true); yes {
				res.OrbTxCnt++
				res.OrbTxSize += uint(tx.Size)
				res.OrbTxWeight += uint(weight)
			}
		}
	}
	return
}

// BuildTxList parses a block's transactions and adds them to the structure, always calculating TX IDs.
func (bl *Block) BuildTxList() (e error) {
	return bl.BuildTxListExt(true)
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
