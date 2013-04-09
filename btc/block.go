package btc

import (
//	"os"
	"errors"
	"bytes"
	"time"
)

type Block struct {
	Raw []byte
	Hash *Uint256
	Txs []*Tx
}

func (bl *Block)GetVersion() uint32 {
	return uint32(lsb2uint(bl.Raw[:4]))
}

func (bl *Block)GetParent() (res []byte) {
	return bl.Raw[4:36]
}

func (bl *Block)GetMerkleRoot() (res []byte) {
	return bl.Raw[36:68]
}


func (bl *Block)GetBlockTime() uint32 {
	return uint32(lsb2uint(bl.Raw[68:72]))
}

func (bl *Block)GetBits() uint32 {
	return uint32(lsb2uint(bl.Raw[72:76]))
}

func (bl *Block)GetNonce() uint32 {
	return uint32(lsb2uint(bl.Raw[76:80]))
}

func NewBlock(data []byte) (*Block, error) {
	if len(data)<81 {
		return nil, errors.New("Block too short")
	}
	var bl Block
	bl.Hash = NewSha2Hash(data[:80])
	bl.Raw = data
	return &bl, nil
}


func (bl *Block)BuildTxList() (e error) {
	var txcnt uint64
	txblock := bl.Raw[80:]
	rd := bytes.NewReader(txblock)
	txcnt, e = ReadVLen64(rd)
	if e != nil {
		return
	}
	
	bl.Txs = make([]*Tx, txcnt)
	for i:=0; i<int(txcnt); i++ {
		sta, _ := rd.Seek(0, 1)
		bl.Txs[i], e = NewTx(rd)
		if e != nil {
			return
		}
		sto, _ := rd.Seek(0, 1)
		bl.Txs[i].Size = uint32(sto-sta)
		bl.Txs[i].Hash = NewSha2Hash(txblock[sta:sto])
	}
	return
}


func (bl *Block)CheckBlock() (er error) {
	// Size limits
	if len(bl.Raw)<81 || len(bl.Raw)>1e6 {
		return errors.New("CheckBlock() : size limits failed")
	}

	// TODO: Check proof of work matches claimed amount
	
	
	// Check timestamp
	if int64(bl.GetBlockTime()) > time.Now().Unix() + 2 * 60 * 60 {
		return errors.New("CheckBlock() : block timestamp too far in the future")
	}

	er = bl.BuildTxList()
	if er != nil {
		return
	}
	
	txcnt := len(bl.Txs)
	
	// First transaction must be coinbase, the rest must not be
	if txcnt==0 || !bl.Txs[0].IsCoinBase() {
		return errors.New("CheckBlock() : first tx is not coinbase")
	}
	for i:=1; i<txcnt; i++ {
		if bl.Txs[i].IsCoinBase() {
			return errors.New("CheckBlock() : more than one coinbase")
		}
	}

	// Check transactions
	for i:=0; i<txcnt; i++ {
		er = bl.Txs[i].CheckTransaction()
		if er!=nil {
			return errors.New("CheckBlock() : CheckTransaction failed\n"+er.Error())
		}
	}

	// TODO: Build the merkle tree already
    
    
	// Check for duplicate txids. This is caught by ConnectInputs(),
	uniqueTx := make(map[[32]byte]bool, txcnt)
	for i:=1; i<txcnt; i++ {
		_, present := uniqueTx[bl.Txs[i].Hash.Hash]
		if present {
			return errors.New("CheckBlock() : duplicate transaction")
		}
		uniqueTx[bl.Txs[i].Hash.Hash] = true
	}

	//TODO: check out-of-bounds SigOpCount

	//TODO: Check merkle root

	return 
}

