package btc

import (
	"io"
	"fmt"
	"sync"
	"bytes"
	"errors"
	"encoding/hex"
	"crypto/sha256"
	"encoding/binary"
)

const (
	SIGHASH_ALL = 1
	SIGHASH_NONE = 2
	SIGHASH_SINGLE = 3
	SIGHASH_ANYONECANPAY = 0x80
)


type TxPrevOut struct {
	Hash [32]byte
	Vout uint32
}

type TxIn struct {
	Input TxPrevOut
	ScriptSig []byte
	Sequence uint32
	//PrvOut *TxOut  // this field is used only during verification
}

type TxOut struct {
	Value uint64
	Pk_script []byte
	BlockHeight uint32
	VoutCount uint32 // number of outputs in the transaction that it came from
	WasCoinbase bool
}

type Tx struct {
	Version uint32
	TxIn []*TxIn
	TxOut []*TxOut
	SegWit [][][]byte
	Lock_time uint32

	// These three fields should be set in block.go:
	Raw []byte
	Size, NoWitSize uint32
	Hash *Uint256

	// This field is only set in chain's ProcessBlockTransactions:
	Fee uint64

	wTxID *Uint256

	hash_lock sync.Mutex
	hashPrevouts []byte
	hashSequence []byte
	hashOutputs []byte
}


type AddrValue struct {
	Value uint64
	Addr20 [20]byte
}


func (po *TxPrevOut) UIdx() uint64 {
	return binary.LittleEndian.Uint64(po.Hash[:8]) ^ uint64(po.Vout)
}


func (to *TxOut) String(testnet bool) (s string) {
	s = fmt.Sprintf("%.8f BTC", float64(to.Value)/1e8)
	s += fmt.Sprint(" in block ", to.BlockHeight)
	a := NewAddrFromPkScript(to.Pk_script, testnet)
	if a != nil {
		s += " to "+a.String()
	} else {
		s += " pk_scr:" + hex.EncodeToString(to.Pk_script)
	}
	return
}


// Non-SegWit format
func (t *Tx) WriteSerialized(wr io.Writer) {
	// Version
	binary.Write(wr, binary.LittleEndian, t.Version)

	//TxIns
	WriteVlen(wr, uint64(len(t.TxIn)))
	for i := range t.TxIn {
		wr.Write(t.TxIn[i].Input.Hash[:])
		binary.Write(wr, binary.LittleEndian, t.TxIn[i].Input.Vout)
		WriteVlen(wr, uint64(len(t.TxIn[i].ScriptSig)))
		wr.Write(t.TxIn[i].ScriptSig[:])
		binary.Write(wr, binary.LittleEndian, t.TxIn[i].Sequence)
	}

	//TxOuts
	WriteVlen(wr, uint64(len(t.TxOut)))
	for i := range t.TxOut {
		binary.Write(wr, binary.LittleEndian, t.TxOut[i].Value)
		WriteVlen(wr, uint64(len(t.TxOut[i].Pk_script)))
		wr.Write(t.TxOut[i].Pk_script[:])
	}

	//Lock_time
	binary.Write(wr, binary.LittleEndian, t.Lock_time)
}


// Non-SegWit format
func (t *Tx) Serialize() ([]byte) {
	wr := new(bytes.Buffer)
	t.WriteSerialized(wr)
	return wr.Bytes()
}


// Return the transaction's hash, that is about to get signed/verified
func (t *Tx) SignatureHash(scriptCode []byte, nIn int, hashType int32) ([]byte) {
	// Remove any OP_CODESEPARATOR
	var idx int
	var nd []byte
	for idx < len(scriptCode) {
		op, _, n, e := GetOpcode(scriptCode[idx:])
		if e!=nil {
			break
		}
		if op != 0xab {
			nd = append(nd, scriptCode[idx:idx+n]...)
		}
		idx+= n
	}
	scriptCode = nd

	ht := hashType&0x1f

	sha := sha256.New()

	binary.Write(sha, binary.LittleEndian, uint32(t.Version))

	if (hashType&SIGHASH_ANYONECANPAY)!=0 {
		sha.Write([]byte{1}) // only 1 input
		// The one input:
		sha.Write(t.TxIn[nIn].Input.Hash[:])
		binary.Write(sha, binary.LittleEndian, uint32(t.TxIn[nIn].Input.Vout))
		WriteVlen(sha, uint64(len(scriptCode)))
		sha.Write(scriptCode)
		binary.Write(sha, binary.LittleEndian, uint32(t.TxIn[nIn].Sequence))
	} else {
		WriteVlen(sha, uint64(len(t.TxIn)))
		for i := range t.TxIn {
			sha.Write(t.TxIn[i].Input.Hash[:])
			binary.Write(sha, binary.LittleEndian, uint32(t.TxIn[i].Input.Vout))

			if i==nIn {
				WriteVlen(sha, uint64(len(scriptCode)))
				sha.Write(scriptCode)
			} else {
				sha.Write([]byte{0})
			}

			if (ht==SIGHASH_NONE || ht==SIGHASH_SINGLE) && i!=nIn {
				sha.Write([]byte{0,0,0,0})
			} else {
				binary.Write(sha, binary.LittleEndian, uint32(t.TxIn[i].Sequence))
			}
		}
	}

	if ht==SIGHASH_NONE {
		sha.Write([]byte{0})
	} else if ht==SIGHASH_SINGLE {
		nOut := nIn
		if nOut >= len(t.TxOut) {
			// Return 1 as the satoshi client (utils.IsOn't ask me why 1, and not something else)
			return []byte{1,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0,0}
		}
		WriteVlen(sha, uint64(nOut+1))
		for i:=0; i < nOut; i++ {
			sha.Write([]byte{0xff,0xff,0xff,0xff,0xff,0xff,0xff,0xff,0})
		}
		binary.Write(sha, binary.LittleEndian, uint64(t.TxOut[nOut].Value))
		WriteVlen(sha, uint64(len(t.TxOut[nOut].Pk_script)))
		sha.Write(t.TxOut[nOut].Pk_script)
	} else {
		WriteVlen(sha, uint64(len(t.TxOut)))
		for i := range t.TxOut {
			binary.Write(sha, binary.LittleEndian, uint64(t.TxOut[i].Value))
			WriteVlen(sha, uint64(len(t.TxOut[i].Pk_script)))
			sha.Write(t.TxOut[i].Pk_script)
		}
	}

	binary.Write(sha, binary.LittleEndian, t.Lock_time)
	binary.Write(sha, binary.LittleEndian, hashType)
	tmp := sha.Sum(nil)
	sha.Reset()
	sha.Write(tmp)
	return sha.Sum(nil)
}


// Signs a specified transaction input
func (tx *Tx) Sign(in int, pk_script []byte, hash_type byte, pubkey, priv_key []byte) error {
	if in >= len(tx.TxIn) {
		return errors.New("tx.Sign() - input index overflow")
	}

	//Calculate proper transaction hash
	h := tx.SignatureHash(pk_script, in, int32(hash_type))

	// Sign
	r, s, er := EcdsaSign(priv_key, h)
	if er != nil {
		return er
	}
	rb := r.Bytes()
	sb := s.Bytes()

	if rb[0] >= 0x80 {
		rb = append([]byte{0x00}, rb...)
	}

	if sb[0] >= 0x80 {
		sb = append([]byte{0x00}, sb...)
	}

	// Output the signing result into a buffer, in format expected by bitcoin protocol
	busig := new(bytes.Buffer)
	busig.WriteByte(0x30)
	busig.WriteByte(byte(4+len(rb)+len(sb)))
	busig.WriteByte(0x02)
	busig.WriteByte(byte(len(rb)))
	busig.Write(rb)
	busig.WriteByte(0x02)
	busig.WriteByte(byte(len(sb)))
	busig.Write(sb)
	busig.WriteByte(byte(hash_type))

	// Output the signature and the public key into tx.ScriptSig
	buscr := new(bytes.Buffer)
	buscr.WriteByte(byte(busig.Len()))
	buscr.Write(busig.Bytes())

	buscr.WriteByte(byte(len(pubkey)))
	buscr.Write(pubkey)

	// assign sign script ot the tx:
	tx.TxIn[in].ScriptSig = buscr.Bytes()

	return nil // no error
}


// Signs a specified transaction input
func (tx *Tx) SignWitness(in int, pk_script []byte, amount uint64, hash_type byte, pubkey, priv_key []byte) error {
	if in >= len(tx.TxIn) {
		return errors.New("tx.Sign() - input index overflow")
	}

	//Calculate proper transaction hash
	h := tx.WitnessSigHash(pk_script, amount, in, int32(hash_type))

	// Sign
	r, s, er := EcdsaSign(priv_key, h)
	if er != nil {
		return er
	}
	rb := r.Bytes()
	sb := s.Bytes()

	if rb[0] >= 0x80 {
		rb = append([]byte{0x00}, rb...)
	}

	if sb[0] >= 0x80 {
		sb = append([]byte{0x00}, sb...)
	}

	// Output the signing result into a buffer, in format expected by bitcoin protocol
	busig := new(bytes.Buffer)
	busig.WriteByte(0x30)
	busig.WriteByte(byte(4+len(rb)+len(sb)))
	busig.WriteByte(0x02)
	busig.WriteByte(byte(len(rb)))
	busig.Write(rb)
	busig.WriteByte(0x02)
	busig.WriteByte(byte(len(sb)))
	busig.Write(sb)
	busig.WriteByte(byte(hash_type))

	if tx.SegWit == nil {
		tx.SegWit = make([][][]byte, len(tx.TxIn))
	}

	tx.SegWit[in] = [][]byte{busig.Bytes(), pubkey}

	return nil // no error
}


func (t *TxPrevOut)String() (s string) {
	for i := 0; i<32; i++ {
		s+= fmt.Sprintf("%02x", t.Hash[31-i])
	}
	s+= fmt.Sprintf("-%03d", t.Vout)
	return
}

func (in *TxPrevOut)IsNull() bool {
	return allzeros(in.Hash[:]) && in.Vout==0xffffffff
}


func (tx *Tx) IsCoinBase() bool {
	if len(tx.TxIn)==1 {
		inp := tx.TxIn[0].Input
		if inp.IsNull() {
			return true
		}
	}
	return false
}


func (tx *Tx) CheckTransaction() error {
	// Basic checks that utils.IsOn't depend on any context
	if len(tx.TxIn)==0 {
		return errors.New("CheckTransaction() : vin empty")
	}
	if len(tx.TxOut)==0 {
		return errors.New("CheckTransaction() : vout empty")
	}

	// Size limits
	if tx.NoWitSize * 4 > MAX_BLOCK_WEIGHT {
		return errors.New("CheckTransaction() : size limits failed")
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


func (tx *Tx) IsFinal(blockheight, timestamp uint32) bool {
	if tx.Lock_time == 0 {
		return true
	}

	if tx.Lock_time < LOCKTIME_THRESHOLD {
		if tx.Lock_time < blockheight {
			return true
		}
	} else {
		if tx.Lock_time < timestamp {
			return true
		}
	}

	for i := range tx.TxIn {
		if tx.TxIn[i].Sequence != 0xffffffff {
			return false
		}
	}

	return true
}


// Decode a raw transaction output from a given bytes slice.
// Returns the output and the size it took in the buffer.
func NewTxOut(b []byte) (txout *TxOut, offs int) {
	var le, n int

	txout = new(TxOut)

	txout.Value = binary.LittleEndian.Uint64(b[0:8])
	offs = 8

	le, n = VLen(b[offs:])
	if n==0 {
		return nil, 0
	}
	offs+= n

	txout.Pk_script = make([]byte, le)
	copy(txout.Pk_script[:], b[offs:offs+le])
	offs += le

	return
}


// Decode a raw transaction input from a given bytes slice.
// Returns the input and the size it took in the buffer.
func NewTxIn(b []byte) (txin *TxIn, offs int) {
	var le, n int

	txin = new(TxIn)

	copy(txin.Input.Hash[:], b[0:32])
	txin.Input.Vout = binary.LittleEndian.Uint32(b[32:36])
	offs = 36

	le, n = VLen(b[offs:])
	if n==0 {
		return nil, 0
	}
	offs+= n

	txin.ScriptSig = make([]byte, le)
	copy(txin.ScriptSig[:], b[offs:offs+le])
	offs+= le

	// Sequence
	txin.Sequence = binary.LittleEndian.Uint32(b[offs:offs+4])
	offs += 4

	return
}


// Decode a raw transaction from a given bytes slice.
// Returns the transaction and the size it took in the buffer.
// WARNING: This function does not set Tx.Hash, Tx.Size and Tx.Raw
func NewTx(b []byte) (tx *Tx, offs int) {
	defer func() { // In case if the buffer was too short, to recover from a panic
		if r := recover(); r != nil {
			println("NewTx failed")
			tx = nil
			offs = 0
		}
	}()

	var le, n, lel, idx int
	var segwit bool

	tx = new(Tx)

	tx.Version = binary.LittleEndian.Uint32(b[0:4])
	offs = 4

	if b[offs]==0 && b[offs+1]==1 {
		segwit = true // flag is 0x01
		offs += 2
	}

	// TxIn
	le, n = VLen(b[offs:])
	if n==0 {
		return nil, 0
	}
	offs += n
	tx.TxIn = make([]*TxIn, le)
	for i := range tx.TxIn {
		tx.TxIn[i], n = NewTxIn(b[offs:])
		offs += n
	}

	// TxOut
	le, n = VLen(b[offs:])
	if n==0 {
		return nil, 0
	}
	offs += n
	tx.TxOut = make([]*TxOut, le)
	for i := range tx.TxOut {
		tx.TxOut[i], n = NewTxOut(b[offs:])
		offs += n
	}

	if segwit {
		tx.SegWit = make([][][]byte, len(tx.TxIn))
		for i := range tx.TxIn {
			le, n = VLen(b[offs:])
			if n==0 {
				return nil, 0
			}
			offs += n
			tx.SegWit[i] = make([][]byte, le)
			for idx=0; idx<le; idx++ {
				lel, n = VLen(b[offs:])
				if n==0 {
					return nil, 0
				}
				offs += n
				tx.SegWit[i][idx] = make([]byte, lel)
				copy(tx.SegWit[i][idx], b[offs:offs+lel])
				offs += lel
			}
		}
	}

	tx.Lock_time = binary.LittleEndian.Uint32(b[offs:offs+4])
	offs += 4

	return
}


func TxInSize(b []byte) int {
	le, n := VLen(b[36:])
	if n==0 {
		return 0
	}
	return 36+n+le+4
}


func TxOutSize(b []byte) int {
	le, n := VLen(b[8:])
	if n==0 {
		return 0
	}
	return 8+n+le
}


func TxSize(b []byte) (offs int) {
	defer func() { // In case if the buffer was too short, to recover from a panic
		if r := recover(); r != nil {
			println("NewSize failed")
			offs = 0
		}
	}()

	var le, txincnt, n, lel int
	var segwit bool

	offs = 4 // version

	if b[offs]==0 && b[offs+1]==1 {
		segwit = true // flag is 0x01
		offs += 2
	}

	// TxIn
	txincnt, n = VLen(b[offs:])  // in_cnt
	if n==0 {
		return 0
	}
	offs += n
	for le=txincnt; le>0; le-- {
		n = TxInSize(b[offs:])
		offs += n
	}

	// TxOut
	le, n = VLen(b[offs:])
	if n==0 {
		return 0
	}
	offs += n
	for ; le>0; le-- {
		n = TxOutSize(b[offs:])
		offs += n
	}

	if segwit {
		for ; txincnt>0; txincnt-- {
			le, n = VLen(b[offs:])
			if n==0 {
				return 0
			}
			offs += n
			for ; le>0; le-- {
				lel, n = VLen(b[offs:])
				if n==0 {
					return 0
				}
				offs += n+lel
			}
		}
	}

	offs += 4  // Lock_time

	return
}


func (txin *TxIn) GetKeyAndSig() (sig *Signature, key *PublicKey, e error) {
	sig, e = NewSignature(txin.ScriptSig[1:1+txin.ScriptSig[0]])
	if e != nil {
		return
	}
	offs := 1+txin.ScriptSig[0]
	key, e = NewPublicKey(txin.ScriptSig[1+offs:1+offs+txin.ScriptSig[offs]])
	return
}


func (tx *Tx) GetLegacySigOpCount() (nSigOps uint) {
	for i:=0; i<len(tx.TxIn); i++ {
		nSigOps += GetSigOpCount(tx.TxIn[i].ScriptSig, false)
	}
	for i:=0; i<len(tx.TxOut); i++ {
		nSigOps += GetSigOpCount(tx.TxOut[i].Pk_script, false)
	}
	return
}


func (tx *Tx) WitnessSigHash(scriptCode []byte, amount uint64, nIn int, hashType int32) ([]byte) {
	var nullHash [32]byte
	var hashPrevouts []byte
	var hashSequence []byte
	var hashOutputs []byte

	tx.hash_lock.Lock()
	defer tx.hash_lock.Unlock()

	sha := sha256.New()

	if (hashType & SIGHASH_ANYONECANPAY)==0 {
		if tx.hashPrevouts==nil {
			for _, vin := range tx.TxIn {
				sha.Write(vin.Input.Hash[:])
				binary.Write(sha, binary.LittleEndian, vin.Input.Vout)
			}
			hashPrevouts = sha.Sum(nil)
			sha.Reset()
			sha.Write(hashPrevouts)
			tx.hashPrevouts = sha.Sum(nil)
			sha.Reset()
		}
		hashPrevouts = tx.hashPrevouts
	} else {
		hashPrevouts = nullHash[:]
	}

	if ((hashType & SIGHASH_ANYONECANPAY)==0 && (hashType & 0x1f) != SIGHASH_SINGLE && (hashType & 0x1f) != SIGHASH_NONE) {
		if tx.hashSequence==nil {
			for _, vin := range tx.TxIn {
				binary.Write(sha, binary.LittleEndian, vin.Sequence)
			}
			hashSequence = sha.Sum(nil)
			sha.Reset()
			sha.Write(hashSequence)
			tx.hashSequence = sha.Sum(nil)
			sha.Reset()
		}
		hashSequence = tx.hashSequence
	} else {
		hashSequence = nullHash[:]
	}

	if ((hashType & 0x1f) != SIGHASH_SINGLE && (hashType & 0x1f) != SIGHASH_NONE) {
		if tx.hashOutputs==nil {
			for _, vout := range tx.TxOut {
				binary.Write(sha, binary.LittleEndian, vout.Value)
				WriteVlen(sha, uint64(len(vout.Pk_script)))
				sha.Write(vout.Pk_script)
			}
			hashOutputs = sha.Sum(nil)
			sha.Reset()
			sha.Write(hashOutputs)
			tx.hashOutputs = sha.Sum(nil)
			sha.Reset()
		}
		hashOutputs = tx.hashOutputs
	} else if (hashType & 0x1f) == SIGHASH_SINGLE && nIn < len(tx.TxOut) {
		binary.Write(sha, binary.LittleEndian, tx.TxOut[nIn].Value)
		WriteVlen(sha, uint64(len(tx.TxOut[nIn].Pk_script)))
		sha.Write(tx.TxOut[nIn].Pk_script)
		hashOutputs = sha.Sum(nil)
		sha.Reset()
		sha.Write(hashOutputs)
		hashOutputs = sha.Sum(nil)
		sha.Reset()
	} else {
		hashOutputs = nullHash[:]
	}

	binary.Write(sha, binary.LittleEndian, tx.Version)
	sha.Write(hashPrevouts)
	sha.Write(hashSequence)
	sha.Write(tx.TxIn[nIn].Input.Hash[:])
	binary.Write(sha, binary.LittleEndian, tx.TxIn[nIn].Input.Vout)

	WriteVlen(sha, uint64(len(scriptCode)))
	sha.Write(scriptCode)
	binary.Write(sha, binary.LittleEndian, amount)
	binary.Write(sha, binary.LittleEndian, tx.TxIn[nIn].Sequence)
	sha.Write(hashOutputs)

	binary.Write(sha, binary.LittleEndian, tx.Lock_time)
	binary.Write(sha, binary.LittleEndian, hashType)

	hashPrevouts = sha.Sum(nil)
	sha.Reset()
	sha.Write(hashPrevouts)
	return sha.Sum(nil)
}

func (tx *Tx) CountWitnessSigOps(inp int, scriptPubKey []byte) uint {
	scriptSig := tx.TxIn[inp].ScriptSig
	var witness [][]byte

	if len(tx.SegWit) > inp {
		witness = tx.SegWit[inp]
	}

	witnessversion, witnessprogram := IsWitnessProgram(scriptPubKey)
	if witnessprogram!=nil {
		return WitnessSigOps(witnessversion, witnessprogram, witness)
	}

	if IsP2SH(scriptPubKey) && IsPushOnly(scriptSig) {
		var pc, n int
		var data []byte
		for pc < len(scriptSig) {
			_, data, n, _ = GetOpcode(scriptSig[pc:])
			pc += n
		}
		witnessversion, witnessprogram := IsWitnessProgram(data)
		if witnessprogram!=nil {
			return WitnessSigOps(witnessversion, witnessprogram, witness)
		}
	}

	return 0
}

func (tx *Tx) SetHash(raw []byte) {
	if raw==nil {
		raw = tx.Raw
	} else {
		tx.Raw = raw
	}
	h := NewSha2Hash(raw)
	tx.Size = uint32(len(raw))
	if tx.SegWit != nil {
		tx.wTxID = h
		nowit_raw := tx.Serialize()
		tx.Hash = NewSha2Hash(nowit_raw)
		tx.NoWitSize = uint32(len(nowit_raw))
	} else {
		tx.Hash = h
		tx.NoWitSize = tx.Size
	}
}

func (t *Tx) WTxID() *Uint256 {
	if t.wTxID==nil {
		return t.Hash
	} else {
		return t.wTxID
	}
}

func (tx *Tx) VSize() int {
	if tx.NoWitSize==tx.Size {
		return int(tx.Size)
	}
	return ( 3 * int(tx.NoWitSize+1) + int(tx.Size) ) >> 2
}


// SegWit format
func (t *Tx) WriteSerializedNew(wr io.Writer) {
	if t.SegWit == nil {
		t.WriteSerialized(wr)
		return
	}

	// Version
	binary.Write(wr, binary.LittleEndian, t.Version)

	// Marker & flag
	wr.Write([]byte{0x00,0x01})

	//TxIns
	WriteVlen(wr, uint64(len(t.TxIn)))
	for i := range t.TxIn {
		wr.Write(t.TxIn[i].Input.Hash[:])
		binary.Write(wr, binary.LittleEndian, t.TxIn[i].Input.Vout)
		WriteVlen(wr, uint64(len(t.TxIn[i].ScriptSig)))
		wr.Write(t.TxIn[i].ScriptSig[:])
		binary.Write(wr, binary.LittleEndian, t.TxIn[i].Sequence)
	}

	//TxOuts
	WriteVlen(wr, uint64(len(t.TxOut)))
	for i := range t.TxOut {
		binary.Write(wr, binary.LittleEndian, t.TxOut[i].Value)
		WriteVlen(wr, uint64(len(t.TxOut[i].Pk_script)))
		wr.Write(t.TxOut[i].Pk_script[:])
	}

	// Witness
	for _, sw := range t.SegWit {
		WriteVlen(wr, uint64(len(sw)))
		for _, sws := range sw {
			WriteVlen(wr, uint64(len(sws)))
			wr.Write(sws)
		}
	}

	//Lock_time
	binary.Write(wr, binary.LittleEndian, t.Lock_time)
}


// SegWit format
func (t *Tx) SerializeNew() ([]byte) {
	wr := new(bytes.Buffer)
	t.WriteSerializedNew(wr)
	return wr.Bytes()
}
