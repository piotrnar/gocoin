package btc

import (
	"hash"
	"crypto/sha256"
	"encoding/binary"
	"encoding"
)

type ScriptExecutionData struct {
    //! Whether m_tapleaf_hash is initialized.
    M_tapleaf_hash_init bool
	//! The tapleaf hash.
    M_tapleaf_hash []byte

    //! Whether m_codeseparator_pos is initialized.
    M_codeseparator_pos_init bool
    //! Opcode position of the last executed OP_CODESEPARATOR (or 0xFFFFFFFF if none executed).
    M_codeseparator_pos uint32

    //! Whether m_annex_present and (when needed) m_annex_hash are initialized.
    M_annex_init bool
    //! Whether an annex is present.
    M_annex_present bool
    //! Hash of the annex data.
    M_annex_hash []byte

    //! Whether m_validation_weight_left is initialized.
    M_validation_weight_left_init bool
    //! How much validation weight is left (decremented for every successful non-empty signature check).
    M_validation_weight_left int64
}

// TaprootSigHash implements taproot's sighash algorithm
// script - if true uses TAPSCRIPT mode (not TAPROOT)
func (tx *Tx) TaprootSigHash(execdata *ScriptExecutionData, spent_outputs []TxOut, in_pos int, hash_type byte, script bool) []byte {
	var ext_flag, key_version byte
	
	if script {
		ext_flag = 1
	}
	
	tx.hash_lock.Lock()
	defer tx.hash_lock.Unlock()

	sha := TapSighashSha()
	
	sha.Write([]byte{0}) // EPOCH
	
	// Hash type
    var output_type byte
	if hash_type == SIGHASH_DEFAULT {
		output_type = SIGHASH_ALL
	} else {
		output_type = (hash_type & SIGHASH_OUTPUT_MASK) // Default (no sighash byte) is equivalent to SIGHASH_ALL
	}
	
	input_type := hash_type & SIGHASH_INPUT_MASK
	if (!(hash_type <= 0x03 || (hash_type >= 0x81 && hash_type <= 0x83))) {
		return make([]byte, 32)
	}
	sha.Write([]byte{hash_type})

    // Transaction level data
	binary.Write(sha, binary.LittleEndian, tx.Version)
	binary.Write(sha, binary.LittleEndian, tx.Lock_time)
    
	if tx.m_prevouts_single_hash == nil {
		sh := sha256.New()
		for _, ti := range tx.TxIn {
			sh.Write(ti.Input.Hash[:])
			binary.Write(sh, binary.LittleEndian, ti.Input.Vout)
		}
		tx.m_prevouts_single_hash = sh.Sum(nil)
		//println("prevouts:", NewUint256(tx.m_prevouts_single_hash).String())
		
		sh.Reset()
		for i := range tx.TxIn {
			binary.Write(sh, binary.LittleEndian, spent_outputs[i].Value)
		}
		tx.m_spent_amounts_single_hash = sh.Sum(nil)
		//println("amounts:", NewUint256(tx.m_spent_amounts_single_hash).String())
		
		sh.Reset()
		for i := range tx.TxIn {
			WriteVlen(sh, uint64(len(spent_outputs[i].Pk_script)))
			sh.Write(spent_outputs[i].Pk_script)
		}
		tx.m_spent_scripts_single_hash = sh.Sum(nil)
		//println("m_spent_scripts_single_hash:", NewUint256(tx.m_spent_scripts_single_hash).String())
		
		sh.Reset()
		for _, vin := range tx.TxIn {
			binary.Write(sh, binary.LittleEndian, vin.Sequence)
		}
		tx.m_sequences_single_hash = sh.Sum(nil)
		//println("m_sequences_single_hash:", NewUint256(tx.m_sequences_single_hash).String())
	
		sh.Reset()
		for _, vout := range tx.TxOut {
			binary.Write(sh, binary.LittleEndian, vout.Value)
			WriteVlen(sh, uint64(len(vout.Pk_script)))
			sh.Write(vout.Pk_script)
		}
		tx.m_outputs_single_hash = sh.Sum(nil)
		//println("m_outputs_single_hash:", NewUint256(tx.m_outputs_single_hash).String())
	}
	
	if input_type != SIGHASH_ANYONECANPAY {
		sha.Write(tx.m_prevouts_single_hash)
		sha.Write(tx.m_spent_amounts_single_hash)
		sha.Write(tx.m_spent_scripts_single_hash)
		sha.Write(tx.m_sequences_single_hash)
    }
    if output_type == SIGHASH_ALL {
		sha.Write(tx.m_outputs_single_hash)
    }
	
	// Data about the input/prevout being spent
	have_annex := execdata.M_annex_present;
	spend_type := (ext_flag << 1)
	if have_annex {
		spend_type++
	}
	sha.Write([]byte{spend_type})

    if input_type == SIGHASH_ANYONECANPAY {
		sha.Write(tx.TxIn[in_pos].Input.Hash[:])
		binary.Write(sha, binary.LittleEndian, tx.TxIn[in_pos].Input.Vout)
		binary.Write(sha, binary.LittleEndian, spent_outputs[in_pos].Value)
		sha.Write(spent_outputs[in_pos].Pk_script)
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

var _TapSighashSha []byte

func TapSighashSha() hash.Hash {
	s := sha256.New()
	unmarshaler, ok := s.(encoding.BinaryUnmarshaler)
	if !ok {
		panic("second does not implement encoding.BinaryUnmarshaler")
	}
	if err := unmarshaler.UnmarshalBinary(_TapSighashSha); err != nil {
		panic("unable to unmarshal hash: " + err.Error())
	}
	return s
}

func TaggedHash(tag string) hash.Hash {
	sha := sha256.New()
	sha.Write([]byte(tag))
	taghash := sha.Sum(nil)
	sha.Reset()
	sha.Write(taghash)
	sha.Write(taghash)
	return sha
}

func init() {
	sha := TaggedHash("TapSighash")
	var err error
	marshaler, ok := sha.(encoding.BinaryMarshaler)
	if !ok {
		panic("first does not implement encoding.BinaryMarshaler")
	}
	_TapSighashSha, err = marshaler.MarshalBinary()
	if err != nil {
		panic("unable to marshal hash: " + err.Error())
	}
}
