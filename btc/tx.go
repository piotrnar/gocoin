package btc

import (
	"fmt"
	"errors"
	"os"
)

type TxPrevOut struct {
	Hash [32]byte
	Index uint32
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
	Raw []byte
	Hash *Uint256
	TxIn []TxIn
	TxOut []TxOut
	Lock_time uint32
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
		off += put32lsb(buf[off:], t.TxIn[i].Input.Index)
		off += putVlen(buf[off:], 0) // no subScript in Unsiged
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


func CreateTransaction(spend []TxIn, outs[]AddrValue) (out *Tx) {
	out = new(Tx)
	out.Version = 1
	
	out.TxIn = make([]TxIn, len(spend))
	copy(out.TxIn[:], spend[:])
	
	out.TxOut = make([]TxOut, len(outs))
	for i:=range out.TxOut {
		out.TxOut[i].Value = outs[i].Value
		out.TxOut[i].Pk_script = make([]byte, 25)
		out.TxOut[i].Pk_script[0] = 0x76
		out.TxOut[i].Pk_script[1] = 0xa9
		out.TxOut[i].Pk_script[2] = 0x14
		copy(out.TxOut[i].Pk_script[3:23], outs[i].Addr20[:])
		out.TxOut[i].Pk_script[23] = 0x88
		out.TxOut[i].Pk_script[24] = 0xac
	}
	out.Lock_time = 0xffffffff

	return
}


func (oi *TxPrevOut)Save(f *os.File) {
	f.Write(oi.Hash[:])
	write32bit(f, oi.Index)
}


func (t *TxPrevOut)String() (s string) {
	for i := 0; i<32; i++ {
		s+= fmt.Sprintf("%02x", t.Hash[31-i])
	}
	s+= fmt.Sprintf("-%03d", t.Index)
	return
}

func (to *TxOut)Save(f *os.File) {
	write64bit(f, to.Value)
	write32bit(f, uint32(len(to.Pk_script)))
	f.Write(to.Pk_script[:])
}


func (to *TxOut)Size() uint32 {
	return uint32(8+4+len(to.Pk_script[:]))
}


func (txin *TxIn) set(buf []byte) (size uint32) {
	copy(txin.Input.Hash[:], buf[:32])
	txin.Input.Index = uint32(lsb2uint(buf[32:36]))
	
	size = 36
	le, cnt := getVlen(buf[size:])
	size += uint32(cnt)
	
	// Signature script
	txin.ScriptSig = buf[size:size+uint32(le)]

	/* - not needed since all in txs are freed with the block
	txin.ScriptSig = make([]byte, le)
	copy(txin.ScriptSig[:], buf[size:size+uint32(le)])
	*/

	size += uint32(le)
	
	// Sequence
	txin.Sequence = uint32(lsb2uint(buf[size:size+4]))
	size += 4

	return 
}


func (txout *TxOut) set(buf []byte) (size uint32) {
	txout.Value = lsb2uint(buf[:8])
	size = 8
	le, cnt := getVlen(buf[size:])
	size += cnt
	
	// Allocate own memory since blocks are being freed
	txout.Pk_script = make([]byte, le)
	copy(txout.Pk_script[:], buf[size:size+uint32(le)])
	
	/* This is re-using block's memory:
	txout.Pk_script = buf[size:size+uint32(le)]
	*/
	
	size += uint32(le)
	return
}


func (tx *Tx) set(data []byte) (size uint32) {
	tx.Version = uint32(lsb2uint(data[0:4]))

	size = 4 // skip version field
	
	// TxIn
	le, n := getVlen(data[size:])
	size += uint32(n)
	tx.TxIn = make([]TxIn, le)
	for i := range tx.TxIn {
		size += tx.TxIn[i].set(data[size:])
		/*
		lll := tx.TxIn[i].set(data[size:])
		if i>1 {
			println("tx_:", bin2hex(data[size:size+lll]))
			println("sig:", bin2hex(tx.TxIn[i].ScriptSig[:]))
			os.Exit(1)
		}
		size += lll
		*/
	}
	
	// TxOut
	le, n = getVlen(data[size:])
	size += uint32(n)
	tx.TxOut = make([]TxOut, le)
	for i := range tx.TxOut {
		size += tx.TxOut[i].set(data[size:])
	}
	
	tx.Lock_time = uint32(lsb2uint(data[size:size+4]))
	size += 4

	tx.Raw = data[0:size]
	tx.Hash = NewSha2Hash(tx.Raw)

	return
}


func (in *TxPrevOut)IsNull() bool {
	for i:=0; i<32; i++ {
		if in.Hash[i]!=0 {
			return false
		}
	}
	return in.Index==0xffffffff
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
	if len(tx.Raw) > MAX_BLOCK_SIZE {
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


