package btc

import (
	"errors"
	"bytes"
	"time"
)

func (ch *Chain) CheckBlock(bl *Block) (er error, dos bool, maybelater bool) {
	// Size limits
	if len(bl.Raw)<81 || len(bl.Raw)>MAX_BLOCK_SIZE {
		er = errors.New("CheckBlock() : size limits failed")
		dos = true
		return
	}

	// Check timestamp (must not be higher than now +2 minutes)
	if int64(bl.BlockTime) > time.Now().Unix() + 2 * 60 * 60 {
		er = errors.New("CheckBlock() : block timestamp too far in the future")
		dos = true
		return
	}

	if prv, pres := ch.BlockIndex[bl.Hash.BIdx()]; pres {
		if prv.Parent == nil {
			// This is genesis block
			prv.Timestamp = bl.BlockTime
			prv.Bits = bl.Bits
			er = errors.New("CheckBlock: Genesis bock")
			return
		} else {
			er = errors.New("CheckBlock: "+bl.Hash.String()+" already in")
			return
		}
	}

	prevblk, ok := ch.BlockIndex[NewUint256(bl.Parent).BIdx()]
	if !ok {
		er = errors.New("CheckBlock: "+bl.Hash.String()+" parent not found")
		maybelater = true
		return
	}

	// Check proof of work
	//println("block with bits", bl.Bits, "...")
	gnwr := GetNextWorkRequired(prevblk, bl.BlockTime)
	if bl.Bits != gnwr {
		println("AcceptBlock() : incorrect proof of work ", bl.Bits," at block", prevblk.Height+1,
			" exp:", gnwr)

		// Here is a "solution" for whatever shit there is in testnet3, that nobody can explain me:
		if !testnet || ((prevblk.Height+1)%2016)!=0 {
			er = errors.New("CheckBlock: incorrect proof of work")
			dos = true
			return
		}
	}

	er = bl.BuildTxList()
	if er != nil {
		dos = true
		return
	}

	if !bl.Trusted {
		// This is a stupid check, but well..
		if len(bl.Txs)==0 || !bl.Txs[0].IsCoinBase() {
			er = errors.New("CheckBlock() : first tx is not coinbase: "+bl.Hash.String())
			dos = true
			return
		}

		// Check Merkle Root - that's importnant
		if !bytes.Equal(GetMerkel(bl.Txs), bl.MerkleRoot) {
			er = errors.New("CheckBlock() : Merkle Root mismatch")
			dos = true
			return
		}

		// Check transactions - this is the most time consuming task
		for i:=0; i<len(bl.Txs); i++ {
			er = bl.Txs[i].CheckTransaction()
			if er!=nil {
				er = errors.New("CheckBlock() : CheckTransaction failed\n"+er.Error())
				dos = true
				return
			}
		}
	}

	return
}
