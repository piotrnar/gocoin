package btc

import (
	"crypto/sha256"
	"encoding/binary"
)

type ScriptExecutionData struct {
	//! The tapleaf hash.
	M_tapleaf_hash []byte

	//! Hash of the annex data.
	M_annex_hash []byte

	//! How much validation weight is left (decremented for every successful non-empty signature check).
	M_validation_weight_left int64

	//! Opcode position of the last executed OP_CODESEPARATOR (or 0xFFFFFFFF if none executed).
	M_codeseparator_pos uint32

	//! Whether m_codeseparator_pos is initialized.
	M_codeseparator_pos_init bool

	//! Whether m_validation_weight_left is initialized.
	M_validation_weight_left_init bool
}

type taprootSHType struct {
	prevouts     [32]byte
	sequences    [32]byte
	spentAmounts [32]byte
	spentScripts [32]byte
}

// TaprootSigHash implements taproot's sighash algorithm
// script - if true uses TAPSCRIPT mode (not TAPROOT)
// Returns nil if the sighash is undefined for the given parameters, in which
// case the caller must fail the script (BIP341; core's
// SignatureHashSchnorr() returns false in exactly the same situations).
// Never return a "dummy" hash here - a hash an attacker can predict is a hash
// an attacker can sign. Both SchnorrVerify() and SchnorrSign() reject a
// message that is not 32 bytes long, so a nil returned from here fails closed.
func (tx *Tx) TaprootSigHash(execdata *ScriptExecutionData, in_pos int, hash_type byte, script bool) []byte {
	var ext_flag, key_version byte
	le := binary.LittleEndian

	if script {
		ext_flag = 1
	}

	if in_pos < 0 || in_pos >= len(tx.TxIn) || len(tx.Spent_outputs) < len(tx.TxIn) {
		return nil
	}

	// Hash type
	var output_type byte
	if hash_type == SIGHASH_DEFAULT {
		output_type = SIGHASH_ALL
	} else {
		output_type = (hash_type & SIGHASH_OUTPUT_MASK) // Default (no sighash byte) is equivalent to SIGHASH_ALL
	}

	input_type := hash_type & SIGHASH_INPUT_MASK
	if !(hash_type <= 0x03 || (hash_type >= 0x81 && hash_type <= 0x83)) {
		return nil
	}
	if output_type == SIGHASH_SINGLE && in_pos >= len(tx.TxOut) {
		return nil
	}

	// The lock only protects the hashes that are shared by all the inputs of the tx
	// (calculated once), so the rest can be done by many threads at the same time.
	var tap_single *taprootSHType
	var tap_outs *[32]byte
	tx.hashLock.Lock()
	if input_type != SIGHASH_ANYONECANPAY {
		if tx.tapSingleHashes == nil {
			tsh := new(taprootSHType)
			var b []byte
			for _, ti := range tx.TxIn {
				b = append(b, ti.Input.Hash[:]...)
				b = le.AppendUint32(b, ti.Input.Vout)
			}
			tsh.prevouts = sha256.Sum256(b)

			b = b[:0]
			for i := range tx.TxIn {
				b = le.AppendUint64(b, tx.Spent_outputs[i].Value)
			}
			tsh.spentAmounts = sha256.Sum256(b)

			b = b[:0]
			for i := range tx.TxIn {
				b = AppendVlen(b, uint64(len(tx.Spent_outputs[i].Pk_script)))
				b = append(b, tx.Spent_outputs[i].Pk_script...)
			}
			tsh.spentScripts = sha256.Sum256(b)

			b = b[:0]
			for _, vin := range tx.TxIn {
				b = le.AppendUint32(b, vin.Sequence)
			}
			tsh.sequences = sha256.Sum256(b)

			tx.tapSingleHashes = tsh
		}
		tap_single = tx.tapSingleHashes
	}
	if output_type == SIGHASH_ALL {
		if tx.tapOutSingleHash == nil {
			var b []byte
			for _, vout := range tx.TxOut {
				b = vout.appendSerialized(b)
			}
			h := sha256.Sum256(b)
			tx.tapOutSingleHash = &h
		}
		tap_outs = tx.tapOutSingleHash
	}
	tx.hashLock.Unlock()

	b := make([]byte, 0, 256+len(tx.Spent_outputs[in_pos].Pk_script))

	b = append(b, 0) // EPOCH
	b = append(b, hash_type)

	// Transaction level data
	b = le.AppendUint32(b, tx.Version)
	b = le.AppendUint32(b, tx.Lock_time)

	if tap_single != nil {
		b = append(b, tap_single.prevouts[:]...)
		b = append(b, tap_single.spentAmounts[:]...)
		b = append(b, tap_single.spentScripts[:]...)
		b = append(b, tap_single.sequences[:]...)
	}

	if tap_outs != nil {
		b = append(b, tap_outs[:]...)
	}

	// Data about the input/prevout being spent
	have_annex := execdata.M_annex_hash != nil
	spend_type := (ext_flag << 1)
	if have_annex {
		spend_type++
	}
	b = append(b, spend_type)

	if input_type == SIGHASH_ANYONECANPAY {
		b = append(b, tx.TxIn[in_pos].Input.Hash[:]...)
		b = le.AppendUint32(b, tx.TxIn[in_pos].Input.Vout)
		b = le.AppendUint64(b, tx.Spent_outputs[in_pos].Value)
		b = AppendVlen(b, uint64(len(tx.Spent_outputs[in_pos].Pk_script)))
		b = append(b, tx.Spent_outputs[in_pos].Pk_script...)
		b = le.AppendUint32(b, tx.TxIn[in_pos].Sequence)
	} else {
		b = le.AppendUint32(b, uint32(in_pos))
	}
	if have_annex {
		b = append(b, execdata.M_annex_hash...)
	}

	// Data about the output (if only one).
	if output_type == SIGHASH_SINGLE {
		h := sha256.Sum256(tx.TxOut[in_pos].appendSerialized(nil))
		b = append(b, h[:]...)
	}

	// Additional data for BIP 342 signatures
	if script {
		b = append(b, execdata.M_tapleaf_hash...)
		b = append(b, key_version)
		b = le.AppendUint32(b, execdata.M_codeseparator_pos)
	}

	sha := Hasher(HASHER_TAPSIGHASH)
	sha.Write(b)
	return sha.Sum(nil)
}
