package chain

import (
	"fmt"
	"time"
	"bytes"
	"errors"
	"github.com/piotrnar/gocoin/lib/btc"
)

func (ch *Chain) CheckBlock(bl *btc.Block) (er error, dos bool, maybelater bool) {
	// Size limits
	if len(bl.Raw)<81 || len(bl.Raw)>btc.MAX_BLOCK_SIZE {
		er = errors.New("CheckBlock() : size limits failed")
		dos = true
		return
	}

	// Check timestamp (must not be higher than now +2 hours)
	if int64(bl.BlockTime()) > time.Now().Unix() + 2 * 60 * 60 {
		er = errors.New("CheckBlock() : block timestamp too far in the future")
		dos = true
		return
	}

	if prv, pres := ch.BlockIndex[bl.Hash.BIdx()]; pres {
		if prv.Parent == nil {
			// This is genesis block
			er = errors.New("Genesis")
			return
		} else {
			er = errors.New("CheckBlock: "+bl.Hash.String()+" already in")
			return
		}
	}

	prevblk, ok := ch.BlockIndex[btc.NewUint256(bl.ParentHash()).BIdx()]
	if !ok {
		er = errors.New("CheckBlock: "+bl.Hash.String()+" parent not found")
		maybelater = true
		return
	}

	height := prevblk.Height+1

	// Reject the block if it reaches into the chain deeper than our unwind buffer
	if prevblk!=ch.BlockTreeEnd && int(ch.BlockTreeEnd.Height)-int(height)>=MovingCheckopintDepth {
		er = errors.New(fmt.Sprint("CheckBlock: btc.Block ", bl.Hash.String(),
			" hooks too deep into the chain: ", height, "/", ch.BlockTreeEnd.Height, " ",
			btc.NewUint256(bl.ParentHash()).String()))
		return
	}

	// Check proof of work
	gnwr := ch.GetNextWorkRequired(prevblk, bl.BlockTime())
	if bl.Bits() != gnwr {
		println("AcceptBlock() : incorrect proof of work ", bl.Bits," at block", height, " exp:", gnwr)

		// Here is a "solution" for whatever shit there is in testnet3, that nobody can explain me:
		if !ch.testnet() || (height%2016)!=0 {
			er = errors.New("CheckBlock: incorrect proof of work")
			dos = true
			return
		}
	}

	if bl.Txs==nil {
		er = bl.BuildTxList()
		if er != nil {
			dos = true
			return
		}
	}

	if !bl.Trusted {
		if bl.Version()==0 || (height>=ForceBlockVer2From && !ch.testnet() && bl.Version()<2) {
			er = errors.New("CheckBlock() : Block version too low: "+bl.Hash.String())
			dos = true
			return
		}

		if bl.Version()>=2 {
			var exp []byte
			if height >= 0x800000 {
				if height >= 0x80000000 {
					exp = []byte{5, byte(height), byte(height>>8), byte(height>>16), byte(height>>24), 0}
				} else {
					exp = []byte{4, byte(height), byte(height>>8), byte(height>>16), byte(height>>24)}
				}
			} else {
				exp = []byte{3, byte(height), byte(height>>8), byte(height>>16)}
			}
			if len(bl.Txs[0].TxIn[0].ScriptSig)<len(exp) || !bytes.Equal(exp, bl.Txs[0].TxIn[0].ScriptSig[:len(exp)]) {
				er = errors.New("CheckBlock() : Unexpected block number in coinbase: "+bl.Hash.String())
				dos = true
				return
			}
		}

		// This is a stupid check, but well, we need to be satoshi compatible
		if len(bl.Txs)==0 || !bl.Txs[0].IsCoinBase() {
			er = errors.New("CheckBlock() : first tx is not coinbase: "+bl.Hash.String())
			dos = true
			return
		}

		// Check Merkle Root - that's importnant
		if !bytes.Equal(btc.GetMerkel(bl.Txs), bl.MerkleRoot()) {
			er = errors.New("CheckBlock() : Merkle Root mismatch")
			dos = true
			return
		}

		// Check transactions - this is the most time consuming task
		for i:=0; i<len(bl.Txs); i++ {
			er = bl.Txs[i].CheckTransaction()
			if er!=nil {
				er = errors.New("CheckBlock() : CheckTransaction failed: "+er.Error())
				dos = true
				return
			}
			if !bl.Txs[i].IsFinal(height, bl.BlockTime()) {
				er = errors.New("CheckBlock() : Contains transaction that is not final")
				return
			}
		}
	}

	return
}
