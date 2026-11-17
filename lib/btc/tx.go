package btc

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sync"
)

const (
	SIGHASH_ALL          = 1
	SIGHASH_NONE         = 2
	SIGHASH_SINGLE       = 3
	SIGHASH_ANYONECANPAY = 0x80

	SIGHASH_DEFAULT     = 0 //!< Taproot only; implied when sighash byte is missing, and equivalent to SIGHASH_ALL
	SIGHASH_OUTPUT_MASK = 3
	SIGHASH_INPUT_MASK  = 0x80
)

type TxPrevOut struct {
	Hash [32]byte
	Vout uint32
}

type TxIn struct {
	ScriptSig []byte
	Input     TxPrevOut
	Sequence  uint32
	//PrvOut *TxOut  // this field is used only during verification
}

type TxOut struct {
	Pk_script   []byte
	Value       uint64
	BlockHeight uint32
	VoutCount   uint32 // number of outputs in the transaction that it came from
	WasCoinbase bool
}

// Make sure to call tx's AllocVerVars() before using any of these fields and Clean() once done
type TxVerVars struct {
	hashPrevouts     *[32]byte
	hashSequence     *[32]byte
	hashOutputs      *[32]byte
	tapSingleHashes  *taprootSHType
	tapOutSingleHash *[32]byte
	Spent_outputs    []*TxOut // this is used by TaprootSigHash()
	CalculatedFee    uint64
	hashLock         sync.Mutex
}

type Tx struct {
	*TxVerVars
	TxIn      []*TxIn
	TxOut     []*TxOut
	SegWit    [][][]byte
	Raw       []byte
	Hash      Uint256
	wTxID     Uint256
	Version   uint32
	Lock_time uint32
	Size      uint32
	NoWitSize uint32
}

type AddrValue struct {
	Value  uint64
	Addr20 [20]byte
}

func (t *Tx) AllocVerVars() {
	if t.TxVerVars == nil {
		t.TxVerVars = new(TxVerVars)
	} else {
		fmt.Println("ERROR: AllocVerVars() not necessary")
	}
}

func (t *Tx) Clean() {
	t.TxVerVars = nil
}

func UIdx(hash []byte, vout uint32) uint64 {
	// use different bytes than BIDX, just to make sure we're not mixing them
	return binary.LittleEndian.Uint64(hash[24:32]) ^ uint64(vout)
}

func (po *TxPrevOut) UIdx() uint64 {
	// use different bytes than BIDX, just to make sure we're not mixing them
	return binary.LittleEndian.Uint64(po.Hash[24:32]) ^ uint64(po.Vout)
}

func (to *TxOut) String(testnet bool) (s string) {
	s = fmt.Sprintf("%.8f BTC", float64(to.Value)/1e8)
	s += fmt.Sprint(" in block ", to.BlockHeight)
	a := NewAddrFromPkScript(to.Pk_script, testnet)
	if a != nil {
		s += " to " + a.String()
	} else {
		s += " pk_scr:" + hex.EncodeToString(to.Pk_script)
	}
	return
}

// Non-SegWit format
func (t *Tx) WriteSerialized(wr io.Writer) {
	wr.Write(t.Serialize())
}

func (ti *TxIn) appendSerialized(b []byte) []byte {
	b = append(b, ti.Input.Hash[:]...)
	b = binary.LittleEndian.AppendUint32(b, ti.Input.Vout)
	b = AppendVlen(b, uint64(len(ti.ScriptSig)))
	b = append(b, ti.ScriptSig...)
	return binary.LittleEndian.AppendUint32(b, ti.Sequence)
}

func (to *TxOut) appendSerialized(b []byte) []byte {
	b = binary.LittleEndian.AppendUint64(b, to.Value)
	b = AppendVlen(b, uint64(len(to.Pk_script)))
	return append(b, to.Pk_script...)
}

// serializedSize returns the exact number of bytes that appendSerialized() will add.
func (t *Tx) serializedSize(witness bool) (n int) {
	n = 4 + VLenSize(uint64(len(t.TxIn))) + VLenSize(uint64(len(t.TxOut))) + 4
	for _, ti := range t.TxIn {
		n += 36 + VLenSize(uint64(len(ti.ScriptSig))) + len(ti.ScriptSig) + 4
	}
	for _, to := range t.TxOut {
		n += 8 + VLenSize(uint64(len(to.Pk_script))) + len(to.Pk_script)
	}
	if witness && t.SegWit != nil {
		n += 2
		for _, sw := range t.SegWit {
			n += VLenSize(uint64(len(sw)))
			for _, sws := range sw {
				n += VLenSize(uint64(len(sws))) + len(sws)
			}
		}
	}
	return
}

// appendSerialized appends the serialized transaction to the given buffer.
// The witness data is only added if requested and if the tx has any.
func (t *Tx) appendSerialized(b []byte, witness bool) []byte {
	witness = witness && t.SegWit != nil

	b = binary.LittleEndian.AppendUint32(b, t.Version)
	if witness {
		b = append(b, 0x00, 0x01) // Marker & flag
	}

	b = AppendVlen(b, uint64(len(t.TxIn)))
	for _, ti := range t.TxIn {
		b = ti.appendSerialized(b)
	}

	b = AppendVlen(b, uint64(len(t.TxOut)))
	for _, to := range t.TxOut {
		b = to.appendSerialized(b)
	}

	if witness {
		for _, sw := range t.SegWit {
			b = AppendVlen(b, uint64(len(sw)))
			for _, sws := range sw {
				b = AppendVlen(b, uint64(len(sws)))
				b = append(b, sws...)
			}
		}
	}

	return binary.LittleEndian.AppendUint32(b, t.Lock_time)
}

// Non-SegWit format
func (t *Tx) Serialize() []byte {
	return t.appendSerialized(make([]byte, 0, t.serializedSize(false)), false)
}

// SignatureHash returns the transaction's hash, that is about to get signed/verified.
func (t *Tx) SignatureHash(scriptCode []byte, nIn int, hashType int32) []byte {
	// Remove any OP_CODESEPARATOR
	var idx int
	var nd []byte
	for idx < len(scriptCode) {
		op, _, n, e := GetOpcode(scriptCode[idx:])
		if e != nil {
			break
		}
		if op != 0xab {
			nd = append(nd, scriptCode[idx:idx+n]...)
		}
		idx += n
	}
	scriptCode = nd

	ht := hashType & 0x1f

	// Build the whole preimage in one buffer and then hash it
	le := binary.LittleEndian
	b := make([]byte, 0, t.serializedSize(false)+len(scriptCode)+8)

	b = le.AppendUint32(b, t.Version)

	if (hashType & SIGHASH_ANYONECANPAY) != 0 {
		b = append(b, 1) // only 1 input
		// The one input:
		b = append(b, t.TxIn[nIn].Input.Hash[:]...)
		b = le.AppendUint32(b, t.TxIn[nIn].Input.Vout)
		b = AppendVlen(b, uint64(len(scriptCode)))
		b = append(b, scriptCode...)
		b = le.AppendUint32(b, t.TxIn[nIn].Sequence)
	} else {
		b = AppendVlen(b, uint64(len(t.TxIn)))
		for i := range t.TxIn {
			b = append(b, t.TxIn[i].Input.Hash[:]...)
			b = le.AppendUint32(b, t.TxIn[i].Input.Vout)

			if i == nIn {
				b = AppendVlen(b, uint64(len(scriptCode)))
				b = append(b, scriptCode...)
			} else {
				b = append(b, 0)
			}

			if (ht == SIGHASH_NONE || ht == SIGHASH_SINGLE) && i != nIn {
				b = append(b, 0, 0, 0, 0)
			} else {
				b = le.AppendUint32(b, t.TxIn[i].Sequence)
			}
		}
	}

	if ht == SIGHASH_NONE {
		b = append(b, 0)
	} else if ht == SIGHASH_SINGLE {
		nOut := nIn
		if nOut >= len(t.TxOut) {
			// Return 1 as the satoshi client (do not ask me why 1, and not something else)
			return []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0}
		}
		b = AppendVlen(b, uint64(nOut+1))
		for i := 0; i < nOut; i++ {
			b = append(b, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0)
		}
		b = t.TxOut[nOut].appendSerialized(b)
	} else {
		b = AppendVlen(b, uint64(len(t.TxOut)))
		for i := range t.TxOut {
			b = t.TxOut[i].appendSerialized(b)
		}
	}

	b = le.AppendUint32(b, t.Lock_time)
	b = le.AppendUint32(b, uint32(hashType))

	h := sha256.Sum256(b)
	h = sha256.Sum256(h[:])
	return h[:]
}

// Sign signs a specified transaction input.
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
	busig.WriteByte(byte(4 + len(rb) + len(sb)))
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

// SignWitness signs a specified transaction input.
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
	busig.WriteByte(byte(4 + len(rb) + len(sb)))
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

func (t *TxPrevOut) String() (s string) {
	for i := 0; i < 32; i++ {
		s += fmt.Sprintf("%02x", t.Hash[31-i])
	}
	s += fmt.Sprintf("-%03d", t.Vout)
	return
}

func (in *TxPrevOut) IsNull() bool {
	return allzeros(in.Hash[:]) && in.Vout == 0xffffffff
}

func (tx *Tx) IsCoinBase() bool {
	if len(tx.TxIn) == 1 {
		inp := tx.TxIn[0].Input
		if inp.IsNull() {
			return true
		}
	}
	return false
}

func (tx *Tx) CheckTransaction() error {
	// Basic checks that utils.IsOn't depend on any context
	if len(tx.TxIn) == 0 {
		return errors.New("CheckTransaction() : vin empty - RPC_Result:bad-txns-vin-empty")
	}
	if len(tx.TxOut) == 0 {
		return errors.New("CheckTransaction() : vout empty - RPC_Result:bad-txns-vout-empty")
	}

	// Size limits
	if tx.NoWitSize*4 > MAX_BLOCK_WEIGHT {
		return errors.New("CheckTransaction() : size limits failed - RPC_Result:bad-txns-oversize")
	}

	// Check for output value overflow (CVE-2010-5139).
	// Note: satoshi's client stores values as signed int64 and rejects negative ones
	// with bad-txns-vout-negative. Here the values are unsigned, so the wire encoding
	// of a "negative" amount simply reads as a value above MAX_MONEY and is rejected
	// by the check below - the accept/reject outcome is the same.
	var valueout uint64
	for i := range tx.TxOut {
		if tx.TxOut[i].Value > MAX_MONEY {
			return errors.New("CheckTransaction() : txout.Value too high - RPC_Result:bad-txns-vout-toolarge")
		}
		valueout += tx.TxOut[i].Value
		if valueout > MAX_MONEY {
			return errors.New("CheckTransaction() : txout total too high - RPC_Result:bad-txns-txouttotal-toolarge")
		}
	}

	if tx.IsCoinBase() {
		if len(tx.TxIn[0].ScriptSig) < 2 || len(tx.TxIn[0].ScriptSig) > 100 {
			return fmt.Errorf("CheckTransaction() : coinbase script size %d - RPC_Result:bad-cb-length",
				len(tx.TxIn[0].ScriptSig))
		}
	} else {
		for i := range tx.TxIn {
			if tx.TxIn[i].Input.IsNull() {
				return errors.New("CheckTransaction() : prevout is null - RPC_Result:bad-txns-prevout-null")
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

// NewTxOut decodes a raw transaction output from a given bytes slice.
// Returns the output and the size it took in the buffer.
func NewTxOut(b []byte) (txout *TxOut, offs int) {
	var le, n int

	txout = new(TxOut)

	txout.Value = binary.LittleEndian.Uint64(b[0:8])
	offs = 8

	le, n = VLen(b[offs:])
	if n == 0 {
		return nil, 0
	}
	offs += n

	if le < 0 || le > len(b)-offs {
		return nil, 0
	}
	txout.Pk_script = make([]byte, le)
	copy(txout.Pk_script[:], b[offs:offs+le])
	offs += le

	return
}

// NewTxIn decodes a raw transaction input from a given bytes slice.
// Returns the input and the size it took in the buffer.
func NewTxIn(b []byte) (txin *TxIn, offs int) {
	var le, n int

	txin = new(TxIn)

	copy(txin.Input.Hash[:], b[0:32])
	txin.Input.Vout = binary.LittleEndian.Uint32(b[32:36])
	offs = 36

	le, n = VLen(b[offs:])
	if n == 0 {
		return nil, 0
	}
	offs += n

	if le < 0 || le > len(b)-offs {
		return nil, 0
	}
	txin.ScriptSig = make([]byte, le)
	copy(txin.ScriptSig[:], b[offs:offs+le])
	offs += le

	// Sequence
	txin.Sequence = binary.LittleEndian.Uint32(b[offs : offs+4])
	offs += 4

	return
}

// NewTx decodes a raw transaction from a given bytes slice.
// Returns the transaction and the size it took in the buffer.
// WARNING: This function does not set Tx.Hash, Tx.Size and Tx.Raw
func NewTx(b []byte) (tx *Tx, offs int) {
	defer func() { // In case if the buffer was too short, to recover from a panic
		if r := recover(); r != nil {
			fmt.Println("NewTx failed")
			tx = nil
			offs = 0
		}
	}()

	var le, n, lel, idx int
	var segwit bool

	tx = new(Tx)

	tx.Version = binary.LittleEndian.Uint32(b[0:4])
	offs = 4

	if b[offs] == 0 && b[offs+1] == 1 {
		segwit = true // flag is 0x01
		offs += 2
	}

	// TxIn
	le, n = VLen(b[offs:])
	if n == 0 {
		return nil, 0
	}
	offs += n
	if le < 0 || le > len(b)-offs {
		return nil, 0
	}
	tx.TxIn = make([]*TxIn, le)
	for i := range tx.TxIn {
		var ti *TxIn
		ti, n = NewTxIn(b[offs:])
		if ti == nil || n == 0 {
			return nil, 0
		}
		offs += n
		tx.TxIn[i] = ti
	}

	// TxOut
	le, n = VLen(b[offs:])
	if n == 0 {
		return nil, 0
	}
	offs += n
	if le < 0 || le > len(b)-offs {
		return nil, 0
	}
	tx.TxOut = make([]*TxOut, le)
	for i := range tx.TxOut {
		var to *TxOut
		to, n = NewTxOut(b[offs:])
		if to == nil || n == 0 {
			return nil, 0
		}
		offs += n
		tx.TxOut[i] = to
	}

	if !segwit {
		tx.NoWitSize = uint32(offs + 4) // +4 bytes for the lock time at the end
	} else {
		tx.NoWitSize = uint32(offs - 2 + 4) // -2 bytes for segwit marker and +4 bytes for the lock time at the end
		tx.SegWit = make([][][]byte, len(tx.TxIn))
		var has_witness bool
		for i := range tx.TxIn {
			le, n = VLen(b[offs:])
			if n == 0 {
				return nil, 0
			}
			offs += n
			if le < 0 || le > len(b)-offs {
				return nil, 0
			}
			if le > 0 {
				has_witness = true
			}
			tx.SegWit[i] = make([][]byte, le)
			for idx = 0; idx < le; idx++ {
				lel, n = VLen(b[offs:])
				if n == 0 {
					return nil, 0
				}
				offs += n
				if lel < 0 || lel > len(b)-offs {
					return nil, 0
				}
				tx.SegWit[i][idx] = make([]byte, lel)
				copy(tx.SegWit[i][idx], b[offs:offs+lel])
				offs += lel
			}
		}
		if !has_witness {
			// It's illegal to encode witnesses when all witness stacks are empty
			// (satoshi's client: "Superfluous witness record")
			return nil, 0
		}
	}

	tx.Lock_time = binary.LittleEndian.Uint32(b[offs : offs+4])
	offs += 4

	return
}

func TxInSize(b []byte) int {
	le, n := VLen(b[36:])
	if n == 0 {
		return 0
	}
	return 36 + n + le + 4
}

func TxOutSize(b []byte) int {
	le, n := VLen(b[8:])
	if n == 0 {
		return 0
	}
	return 8 + n + le
}

func TxSize(b []byte) (offs int) {
	defer func() { // In case if the buffer was too short, to recover from a panic
		if r := recover(); r != nil {
			fmt.Println("NewSize failed")
			offs = 0
		}
	}()

	var le, txincnt, n, lel int
	var segwit bool

	offs = 4 // version

	if b[offs] == 0 && b[offs+1] == 1 {
		segwit = true // flag is 0x01
		offs += 2
	}

	// TxIn
	txincnt, n = VLen(b[offs:]) // in_cnt
	if n == 0 {
		return 0
	}
	offs += n
	for le = txincnt; le > 0; le-- {
		n = TxInSize(b[offs:])
		if n <= 0 || n > len(b)-offs {
			// must not continue without advancing, as the counter comes from the (untrusted) data
			return 0
		}
		offs += n
	}

	// TxOut
	le, n = VLen(b[offs:])
	if n == 0 {
		return 0
	}
	offs += n
	for ; le > 0; le-- {
		n = TxOutSize(b[offs:])
		if n <= 0 || n > len(b)-offs {
			return 0
		}
		offs += n
	}

	if segwit {
		for ; txincnt > 0; txincnt-- {
			le, n = VLen(b[offs:])
			if n == 0 {
				return 0
			}
			offs += n
			for ; le > 0; le-- {
				lel, n = VLen(b[offs:])
				if n == 0 {
					return 0
				}
				offs += n
				if lel > len(b)-offs {
					return 0
				}
				offs += lel
			}
		}
	}

	if len(b)-offs < 4 {
		return 0 // the tx does not fit in the buffer
	}
	offs += 4 // Lock_time

	return
}

func (txin *TxIn) GetKeyAndSig() (sig *Signature, key *PublicKey, e error) {
	sig, e = NewSignature(txin.ScriptSig[1 : 1+txin.ScriptSig[0]])
	if e != nil {
		return
	}
	offs := 1 + txin.ScriptSig[0]
	key, e = NewPublicKey(txin.ScriptSig[1+offs : 1+offs+txin.ScriptSig[offs]])
	return
}

func (tx *Tx) GetLegacySigOpCount() (nSigOps uint) {
	for i := 0; i < len(tx.TxIn); i++ {
		nSigOps += GetSigOpCount(tx.TxIn[i].ScriptSig, false)
	}
	for i := 0; i < len(tx.TxOut); i++ {
		nSigOps += GetSigOpCount(tx.TxOut[i].Pk_script, false)
	}
	return
}

func (tx *Tx) WitnessSigHash(scriptCode []byte, amount uint64, nIn int, hashType int32) []byte {
	var nullHash, singleOut [32]byte
	var hashPrevouts, hashSequence, hashOutputs []byte
	le := binary.LittleEndian

	ht := hashType & 0x1f
	anyone := (hashType & SIGHASH_ANYONECANPAY) != 0
	all_outs := ht != SIGHASH_SINGLE && ht != SIGHASH_NONE

	// The lock only protects the hashes that are shared by all the inputs of the tx
	// (calculated once), so the rest can be done by many threads at the same time.
	tx.hashLock.Lock()
	if !anyone {
		if tx.hashPrevouts == nil {
			b := make([]byte, 0, 36*len(tx.TxIn))
			for _, vin := range tx.TxIn {
				b = append(b, vin.Input.Hash[:]...)
				b = le.AppendUint32(b, vin.Input.Vout)
			}
			h := Sha2Sum(b)
			tx.hashPrevouts = &h
		}
		hashPrevouts = tx.hashPrevouts[:]
	} else {
		hashPrevouts = nullHash[:]
	}

	if !anyone && all_outs {
		if tx.hashSequence == nil {
			b := make([]byte, 0, 4*len(tx.TxIn))
			for _, vin := range tx.TxIn {
				b = le.AppendUint32(b, vin.Sequence)
			}
			h := Sha2Sum(b)
			tx.hashSequence = &h
		}
		hashSequence = tx.hashSequence[:]
	} else {
		hashSequence = nullHash[:]
	}

	if all_outs {
		if tx.hashOutputs == nil {
			var b []byte
			for _, vout := range tx.TxOut {
				b = vout.appendSerialized(b)
			}
			h := Sha2Sum(b)
			tx.hashOutputs = &h
		}
		hashOutputs = tx.hashOutputs[:]
	}
	tx.hashLock.Unlock()

	if !all_outs {
		if ht == SIGHASH_SINGLE && nIn < len(tx.TxOut) {
			singleOut = Sha2Sum(tx.TxOut[nIn].appendSerialized(nil))
			hashOutputs = singleOut[:]
		} else {
			hashOutputs = nullHash[:]
		}
	}

	b := make([]byte, 0, 4+32+32+36+9+len(scriptCode)+8+4+32+4+4)
	b = le.AppendUint32(b, tx.Version)
	b = append(b, hashPrevouts...)
	b = append(b, hashSequence...)
	b = append(b, tx.TxIn[nIn].Input.Hash[:]...)
	b = le.AppendUint32(b, tx.TxIn[nIn].Input.Vout)

	b = AppendVlen(b, uint64(len(scriptCode)))
	b = append(b, scriptCode...)
	b = le.AppendUint64(b, amount)
	b = le.AppendUint32(b, tx.TxIn[nIn].Sequence)
	b = append(b, hashOutputs...)

	b = le.AppendUint32(b, tx.Lock_time)
	b = le.AppendUint32(b, uint32(hashType))

	h := Sha2Sum(b)
	return h[:]
}

func (tx *Tx) CountWitnessSigOps(inp int, scriptPubKey []byte) uint {
	scriptSig := tx.TxIn[inp].ScriptSig
	var witness [][]byte

	if len(tx.SegWit) > inp {
		witness = tx.SegWit[inp]
	}

	witnessversion, witnessprogram := IsWitnessProgram(scriptPubKey)
	if witnessprogram != nil {
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
		if witnessprogram != nil {
			return WitnessSigOps(witnessversion, witnessprogram, witness)
		}
	}

	return 0
}

func (tx *Tx) SetHash(raw []byte) {
	if raw == nil {
		raw = tx.Raw
	} else {
		tx.Raw = raw
	}
	var h [32]byte
	ShaHash(raw, h[:])
	tx.Size = uint32(len(raw))
	if tx.SegWit != nil {
		tx.wTxID.Hash = h
		nowit_raw := tx.Serialize()
		tx.Hash.Calc(nowit_raw)
		tx.NoWitSize = uint32(len(nowit_raw))
	} else {
		tx.Hash.Hash = h
		tx.NoWitSize = tx.Size
	}
}

func (t *Tx) WTxID() *Uint256 {
	if t.SegWit == nil {
		return &t.Hash
	} else {
		return &t.wTxID
	}
}

func (tx *Tx) Weight() int {
	return 3*int(tx.NoWitSize) + int(tx.Size)
}

func (tx *Tx) VSize() int {
	if tx.NoWitSize == tx.Size {
		return int(tx.Size)
	}
	return (3*int(tx.NoWitSize+1) + int(tx.Size)) >> 2
}

// SegWit format
func (t *Tx) WriteSerializedNew(wr io.Writer) {
	wr.Write(t.SerializeNew())
}

// SegWit format
func (t *Tx) SerializeNew() []byte {
	return t.appendSerialized(make([]byte, 0, t.serializedSize(true)), true)
}

// ContainsOrdFile checks if there is a file (inscription) inside the transaction
// ... as per github.com/casey/ord
//
//	quick - if set, will not return the scripts (for further decoding)
//
// return true if inscription found
// if quck was not set, also returns all the segwit scripts with the data
// returns false, nil if the transaction does not contain any file
func (tx *Tx) ContainsOrdFile(quick bool) (yes bool, res [][]byte) {
	// Browses through all the Inputs and all the witnesses...
	for _, inp := range tx.SegWit {
		for _, sw := range inp {
			if len(sw) > 40 && sw[0] == 0x20 && bytes.Equal(sw[34:40], []byte{0x00, 0x63, 0x03, 0x6f, 0x72, 0x64}) {
				yes = true
				if quick {
					return
				}
				res = append(res, sw)
			}
		}
	}
	return
}

func (tx *Tx) ContainsRune() bool {
	for _, to := range tx.TxOut {
		if bytes.HasPrefix(to.Pk_script, []byte{OP_RETURN, OP_13}) {
			return true
		}
	}
	return false
}

func (tx *Tx) IsNonMonetary() (yes bool) {
	if tx.ContainsRune() {
		return true
	}
	yes, _ = tx.ContainsOrdFile(true)
	return
}
