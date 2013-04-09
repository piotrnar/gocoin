package btc

import (
	"fmt"
	"errors"
	"os"
)

type TxPrevOut struct {
	Hash [32]byte
	Vout uint32
}

type TxIn struct {
	Input TxPrevOut
	ScriptSig []byte
	Sequence uint32
}

type TxOut struct {
	Value uint64
	Pk_script []byte
}

type Tx struct {
	Version uint32
	TxIn []*TxIn
	TxOut []*TxOut
	Lock_time uint32
	
	// These two fields should be set in block.go:
	Size uint32
	Hash *Uint256
}


type AddrValue struct {
	Value uint64
	Addr20 [20]byte
}


func (t *Tx) Unsigned() (res []byte) {
	var buf [0x10000]byte
	var off uint32
	
	off += put32lsb(buf[:4], t.Version)
	
	off += putVlen(buf[off:], len(t.TxIn))
	for i := range t.TxIn {
		copy(buf[off:off+32], t.TxIn[i].Input.Hash[:])
		off += 32
		off += put32lsb(buf[off:], t.TxIn[i].Input.Vout)
		
		//off += putVlen(buf[off:], 0) // no subScript in Unsiged
		panic("TODO: here you need to put output script from the tx which you are spending")
		
		off += put32lsb(buf[off:], t.TxIn[i].Sequence)
	}

	off += putVlen(buf[off:], len(t.TxOut))
	for i := range t.TxOut {
		off += put64lsb(buf[off:], t.TxOut[i].Value)
		off += putVlen(buf[off:], len(t.TxOut[i].Pk_script))
		copy(buf[off:], t.TxOut[i].Pk_script[:])
		off += uint32(len(t.TxOut[i].Pk_script))
	}

	off += put32lsb(buf[off:], t.Lock_time)

	res = make([]byte, off)
	copy(res[:], buf[:off])
	return
}


func (oi *TxPrevOut)Load(f *os.File) bool {
	_, e := f.Read(oi.Hash[:])
	if e!=nil {
		return false
	}
	oi.Vout, _ = read32bit(f)
	return true
}

func (oi *TxPrevOut)Save(f *os.File) {
	f.Write(oi.Hash[:])
	write32bit(f, oi.Vout)
}


func (t *TxPrevOut)String() (s string) {
	for i := 0; i<32; i++ {
		s+= fmt.Sprintf("%02x", t.Hash[31-i])
	}
	s+= fmt.Sprintf("-%03d", t.Vout)
	return
}


func (to *TxOut)Load(f *os.File) {
	to.Value, _ = read64bit(f)
	le, _ := read32bit(f)
	to.Pk_script = make([]byte, le)
	f.Read(to.Pk_script[:])
}


func (to *TxOut)Save(f *os.File) {
	write64bit(f, to.Value)
	write32bit(f, uint32(len(to.Pk_script)))
	f.Write(to.Pk_script[:])
}


func (to *TxOut)Size() uint32 {
	return uint32(8+4+len(to.Pk_script[:]))
}


func (in *TxPrevOut)IsNull() bool {
	return allzeros(in.Hash[:]) && in.Vout==0xffffffff
}

func (tx *Tx) IsCoinBase() bool {
	return len(tx.TxIn)==1 && tx.TxIn[0].Input.IsNull()
}

func (tx *Tx) CheckTransaction() error {
	// Basic checks that don't depend on any context
	if len(tx.TxIn)==0 {
		return errors.New("CheckTransaction() : vin empty")
	}
	if len(tx.TxOut)==0 {
		return errors.New("CheckTransaction() : vout empty")
	}
    
	// Size limits
	if tx.Size > MAX_BLOCK_SIZE {
		return errors.New("CheckTransaction() : size limits failed")
	}
	
	// Check for negative or overflow output values
	var nValueOut uint64
	for i := range tx.TxOut {
		if (tx.TxOut[i].Value > MAX_MONEY) {
			return errors.New("CheckTransaction() : txout.nValue too high")
		}
		nValueOut += tx.TxOut[i].Value
		if (nValueOut > MAX_MONEY) {
			return errors.New("CheckTransaction() : txout total out of range")
		}
	}

	// Check for duplicate inputs
	vInOutPoints := make(map[TxPrevOut]bool, len(tx.TxIn))
	for i := range tx.TxIn {
		_, present := vInOutPoints[tx.TxIn[i].Input]
		if present {
			return errors.New("CheckTransaction() : duplicate inputs")
		}
		vInOutPoints[tx.TxIn[i].Input] = true
	}

	if tx.IsCoinBase() {
		if len(tx.TxIn[0].ScriptSig) < 2 || len(tx.TxIn[0].ScriptSig) > 100 {
			return errors.New(fmt.Sprintf("CheckTransaction() : coinbase script size %d", 
				len(tx.TxIn[0].ScriptSig)))
		}
	} else {
		for i := range tx.TxIn {
			if tx.TxIn[i].Input.IsNull() {
				return errors.New("CheckTransaction() : prevout is null")
			}
		}
	}

	return nil
}


