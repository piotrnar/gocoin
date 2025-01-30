package btc

import (
	"crypto/sha256"
	"encoding/binary"
)

type ScriptExecutionData struct {
	//! The tapleaf hash.
	M_tapleaf_hash []byte

	//! Whether m_codeseparator_pos is initialized.
	M_codeseparator_pos_init bool
	//! Opcode position of the last executed OP_CODESEPARATOR (or 0xFFFFFFFF if none executed).
	M_codeseparator_pos uint32

	//! Hash of the annex data.
	M_annex_hash []byte

	//! Whether m_validation_weight_left is initialized.
	M_validation_weight_left_init bool
	//! How much validation weight is left (decremented for every successful non-empty signature check).
	M_validation_weight_left int64
}

type taprootSHType struct {
	prevouts     [32]byte
	sequences    [32]byte
	spentAmounts [32]byte
	spentScripts [32]byte
}

// TaprootSigHash implements taproot's sighash algorithm
// script - if true uses TAPSCRIPT mode (not TAPROOT)
func (tx *Tx) TaprootSigHash(execdata *ScriptExecutionData, in_pos int, hash_type byte, script bool) []byte {
	var ext_flag, key_version byte

	if script {
		ext_flag = 1
	}

	tx.hashLock.Lock()
	defer tx.hashLock.Unlock()

	sha := Hasher(HASHER_TAPSIGHASH)

	sha.Write([]byte{0}) // EPOCH

	// Hash type
	var output_type byte
	if hash_type == SIGHASH_DEFAULT {
		output_type = SIGHASH_ALL
	} else {
		output_type = (hash_type & SIGHASH_OUTPUT_MASK) // Default (no sighash byte) is equivalent to SIGHASH_ALL
	}

	input_type := hash_type & SIGHASH_INPUT_MASK
	if !(hash_type <= 0x03 || (hash_type >= 0x81 && hash_type <= 0x83)) {
		return make([]byte, 32)
	}
	sha.Write([]byte{hash_type})

	// Transaction level data
	binary.Write(sha, binary.LittleEndian, tx.Version)
	binary.Write(sha, binary.LittleEndian, tx.Lock_time)

	if input_type != SIGHASH_ANYONECANPAY {
		if tx.tapSingleHashes == nil {
			tx.tapSingleHashes = new(taprootSHType)
			sh := sha256.New()
			for _, ti := range tx.TxIn {
				sh.Write(ti.Input.Hash[:])
				binary.Write(sh, binary.LittleEndian, ti.Input.Vout)
			}
			copy(tx.tapSingleHashes.prevouts[:], sh.Sum(nil))

			sh.Reset()
			for i := range tx.TxIn {
				binary.Write(sh, binary.LittleEndian, tx.Spent_outputs[i].Value)
			}
			copy(tx.tapSingleHashes.spentAmounts[:], sh.Sum(nil))

			sh.Reset()
			for i := range tx.TxIn {
				WriteVlen(sh, uint64(len(tx.Spent_outputs[i].Pk_script)))
				sh.Write(tx.Spent_outputs[i].Pk_script)
			}
			copy(tx.tapSingleHashes.spentScripts[:], sh.Sum(nil))

			sh.Reset()
			for _, vin := range tx.TxIn {
				binary.Write(sh, binary.LittleEndian, vin.Sequence)
			}
			copy(tx.tapSingleHashes.sequences[:], sh.Sum(nil))

		}

		sha.Write(tx.tapSingleHashes.prevouts[:])
		sha.Write(tx.tapSingleHashes.spentAmounts[:])
		sha.Write(tx.tapSingleHashes.spentScripts[:])
		sha.Write(tx.tapSingleHashes.sequences[:])
	}

	if output_type == SIGHASH_ALL {
		if tx.tapOutSingleHash == nil {
			sh := sha256.New()
			for _, vout := range tx.TxOut {
				binary.Write(sh, binary.LittleEndian, vout.Value)
				WriteVlen(sh, uint64(len(vout.Pk_script)))
				sh.Write(vout.Pk_script)
			}
			tx.tapOutSingleHash = new([32]byte)
			copy(tx.tapOutSingleHash[:], sh.Sum(nil))

		}

		sha.Write(tx.tapOutSingleHash[:])
	}

	// Data about the input/prevout being spent
	have_annex := execdata.M_annex_hash != nil
	spend_type := (ext_flag << 1)
	if have_annex {
		spend_type++
	}
	sha.Write([]byte{spend_type})

	if input_type == SIGHASH_ANYONECANPAY {
		sha.Write(tx.TxIn[in_pos].Input.Hash[:])
		binary.Write(sha, binary.LittleEndian, tx.TxIn[in_pos].Input.Vout)
		binary.Write(sha, binary.LittleEndian, tx.Spent_outputs[in_pos].Value)
		WriteVlen(sha, uint64(len(tx.Spent_outputs[in_pos].Pk_script)))
		sha.Write(tx.Spent_outputs[in_pos].Pk_script)
		binary.Write(sha, binary.LittleEndian, tx.TxIn[in_pos].Sequence)
	} else {
		binary.Write(sha, binary.LittleEndian, uint32(in_pos))
	}
	if have_annex {
		sha.Write(execdata.M_annex_hash)
	}

	// Data about the output (if only one).
	if output_type == SIGHASH_SINGLE {
		if in_pos >= len(tx.TxOut) {
			return make([]byte, 32)
		}
		sh := sha256.New()
		binary.Write(sh, binary.LittleEndian, tx.TxOut[in_pos].Value)
		WriteVlen(sh, uint64(len(tx.TxOut[in_pos].Pk_script)))
		sh.Write(tx.TxOut[in_pos].Pk_script)
		sha.Write(sh.Sum(nil))
	}

	// Additional data for BIP 342 signatures
	if script {
		sha.Write(execdata.M_tapleaf_hash)
		sha.Write([]byte{key_version})
		binary.Write(sha, binary.LittleEndian, execdata.M_codeseparator_pos)
	}

	return sha.Sum(nil)
}
