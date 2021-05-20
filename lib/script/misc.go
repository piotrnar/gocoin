package script

import (
	"encoding/hex"
	"fmt"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/secp256k1"
)

func IsP2KH(scr []byte) bool {
	return len(scr) == 25 && scr[0] == 0x76 && scr[1] == 0xa9 &&
		scr[2] == 0x14 && scr[23] == 0x88 && scr[24] == 0xac
}

func IsP2SH(scr []byte) bool {
	return len(scr) == 23 && scr[0] == 0xa9 && scr[1] == 0x14 && scr[22] == 0x87
}

func IsP2WPKH(scr []byte) bool {
	return len(scr) == 22 && scr[0] == 0 && scr[1] == 20
}

func IsP2WSH(scr []byte) bool {
	return len(scr) == 34 && scr[0] == 0 && scr[1] == 32
}

func IsP2PK(scr []byte) (bool, []byte) {
	if len(scr) == 35 && scr[0] == 33 && scr[34] == 0xac && (scr[1] == 0x02 || scr[1] == 0x03) {
		return true, scr[1:34]
	}
	if len(scr) == 67 && scr[0] == 65 && scr[66] == 0xac && scr[1] == 0x04 {
		var pk secp256k1.XY
		if pk.ParsePubkey(scr[1:66]) && pk.IsValid() {
			return true, scr[1:66]
		}
		//println("invalid uncompressed pubkey")
	}
	return false, nil
}

func IsUnspendable(scr []byte) bool {
	if len(scr) > 0 && scr[0] == 0x6a {
		return true
	}
	if len(scr) > MAX_SCRIPT_SIZE {
		return true
	}
	if len(scr) == 67 && scr[0] == 65 && scr[66] == 0xac && scr[1] == 0x04 {
		var pk secp256k1.XY
		if !pk.ParsePubkey(scr[1:66]) || !pk.IsValid() {
			return true // pay to public key script with invalid (uncompressed) key
		}
	}
	return false
}

func IsValidSignatureEncoding(sig []byte) bool {
	// Minimum and maximum size constraints.
	if len(sig) < 9 {
		return false
	}
	if len(sig) > 73 {
		return false
	}

	// A signature is of type 0x30 (compound).
	if sig[0] != 0x30 {
		return false
	}

	// Make sure the length covers the entire signature.
	if int(sig[1]) != len(sig)-3 {
		return false
	}

	// Extract the length of the R element.
	lenR := uint(sig[3])

	// Make sure the length of the S element is still inside the signature.
	if 5+lenR >= uint(len(sig)) {
		return false
	}

	// Extract the length of the S element.
	lenS := uint(sig[5+lenR])

	// Verify that the length of the signature matches the sum of the length
	// of the elements.
	if lenR+lenS+7 != uint(len(sig)) {
		return false
	}

	// Check whether the R element is an integer.
	if sig[2] != 0x02 {
		return false
	}

	// Zero-length integers are not allowed for R.
	if lenR == 0 {
		return false
	}

	// Negative numbers are not allowed for R.
	if (sig[4] & 0x80) != 0 {
		return false
	}

	// Null bytes at the start of R are not allowed, unless R would
	// otherwise be interpreted as a negative number.
	if lenR > 1 && sig[4] == 0x00 && (sig[5]&0x80) == 0 {
		return false
	}

	// Check whether the S element is an integer.
	if sig[lenR+4] != 0x02 {
		return false
	}

	// Zero-length integers are not allowed for S.
	if lenS == 0 {
		return false
	}

	// Negative numbers are not allowed for S.
	if (sig[lenR+6] & 0x80) != 0 {
		return false
	}

	// Null bytes at the start of S are not allowed, unless S would otherwise be
	// interpreted as a negative number.
	if lenS > 1 && (sig[lenR+6] == 0x00) && (sig[lenR+7]&0x80) == 0 {
		return false
	}

	return true
}

func IsDefinedHashtypeSignature(sig []byte) bool {
	if len(sig) == 0 {
		return false
	}
	htype := sig[len(sig)-1] & (btc.SIGHASH_ANYONECANPAY ^ 0xff)
	if htype < btc.SIGHASH_ALL || htype > btc.SIGHASH_SINGLE {
		return false
	}
	return true
}

func IsLowS(sig []byte) bool {
	if !IsValidSignatureEncoding(sig) {
		return false
	}

	ss, e := btc.NewSignature(sig)
	if e != nil {
		return false
	}

	return ss.IsLowS()
}

func CheckSignatureEncoding(sig []byte, flags uint32) bool {
	if len(sig) == 0 {
		return true
	}
	if (flags&(VER_DERSIG|VER_STRICTENC)) != 0 && !IsValidSignatureEncoding(sig) {
		println("aaa")
		return false
	} else if (flags&VER_LOW_S) != 0 && !IsLowS(sig) {
		println("bbb")
		return false
	} else if (flags&VER_STRICTENC) != 0 && !IsDefinedHashtypeSignature(sig) {
		println("ccc")
		return false
	}
	return true
}

func IsCompressedOrUncompressedPubKey(pk []byte) bool {
	if len(pk) < 33 {
		return false
	}
	if pk[0] == 0x04 {
		if len(pk) != 65 {
			return false
		}
	} else if pk[0] == 0x02 || pk[0] == 0x03 {
		if len(pk) != 33 {
			return false
		}
	} else {
		return false
	}
	return true
}

func IsCompressedPubKey(pk []byte) bool {
	if len(pk) != 33 {
		return false
	}
	if pk[0] == 0x02 || pk[0] == 0x03 {
		return true
	}
	return false
}

func CheckPubKeyEncoding(pk []byte, flags uint32, sigversion int) bool {
	if (flags&VER_STRICTENC) != 0 && !IsCompressedOrUncompressedPubKey(pk) {
		return false
	}
	// Only compressed keys are accepted in segwit
	if (flags&VER_WITNESS_PUBKEY) != 0 && sigversion == SIGVERSION_WITNESS_V0 && !IsCompressedPubKey(pk) {
		return false
	}
	return true
}

// https://bitcointalk.org/index.php?topic=1240385.0
func checkMinimalPush(d []byte, opcode int) bool {
	if DBG_SCR {
		fmt.Printf("checkMinimalPush %02x %s\n", opcode, hex.EncodeToString(d))
	}
	if len(d) == 0 {
		// Could have used OP_0.
		if DBG_SCR {
			fmt.Println("Could have used OP_0.")
		}
		return opcode == 0x00
	} else if len(d) == 1 && d[0] >= 1 && d[0] <= 16 {
		// Could have used OP_1 .. OP_16.
		if DBG_SCR {
			fmt.Println("Could have used OP_1 .. OP_16.", 0x01+int(d[0]-1), 0x01, int(d[0]-1))
		}
		return opcode == 0x51+int(d[0])-1
	} else if len(d) == 1 && d[0] == 0x81 {
		// Could have used OP_1NEGATE.
		if DBG_SCR {
			fmt.Println("Could have used OP_1NEGATE.")
		}
		return opcode == 0x4f
	} else if len(d) <= 75 {
		// Could have used a direct push (opcode indicating number of bytes pushed + those bytes).
		if DBG_SCR {
			fmt.Println("Could have used a direct push (opcode indicating number of bytes pushed + those bytes).")
		}
		return opcode == len(d)
	} else if len(d) <= 255 {
		// Could have used OP_PUSHDATA.
		if DBG_SCR {
			fmt.Println("Could have used OP_PUSHDATA.")
		}
		return opcode == 0x4c
	} else if len(d) <= 65535 {
		// Could have used OP_PUSHDATA2.
		if DBG_SCR {
			fmt.Println("Could have used OP_PUSHDATA2.")
		}
		return opcode == 0x4d
	}
	fmt.Println("All checks passed")
	return true
}

func CheckSequence(tx *btc.Tx, inp int, seq int64) bool {
	if tx.Version < 2 {
		return false
	}

	toseq := int64(tx.TxIn[inp].Sequence)

	if (toseq & SEQUENCE_LOCKTIME_DISABLE_FLAG) != 0 {
		return false
	}

	// Mask off any bits that do not have consensus-enforced meaning
	// before doing the integer comparisons
	const nLockTimeMask = SEQUENCE_LOCKTIME_TYPE_FLAG | SEQUENCE_LOCKTIME_MASK
	txToSequenceMasked := toseq & nLockTimeMask
	nSequenceMasked := seq & nLockTimeMask

	if !((txToSequenceMasked < SEQUENCE_LOCKTIME_TYPE_FLAG && nSequenceMasked < SEQUENCE_LOCKTIME_TYPE_FLAG) ||
		(txToSequenceMasked >= SEQUENCE_LOCKTIME_TYPE_FLAG && nSequenceMasked >= SEQUENCE_LOCKTIME_TYPE_FLAG)) {
		return false
	}

	// Now that we know we're comparing apples-to-apples, the
	// comparison is a simple numeric one.
	if nSequenceMasked > txToSequenceMasked {
		return false
	}

	return true
}

func CompressScript(scr []byte) (out []byte) {
	if IsP2KH(scr) {
		out = make([]byte, 21)
		//out[0] = 0x00
		copy(out[1:], scr[3:23])
		return
	}
	if IsP2SH(scr) {
		out = make([]byte, 21)
		out[0] = 0x01
		copy(out[1:], scr[2:22])
		return
	}

	if ok, pk := IsP2PK(scr); ok {
		out = make([]byte, 33)
		copy(out[1:], pk[1:33])
		out[0] = pk[0]
		if pk[0] == 0x04 {
			out[0] |= pk[64] & 0x01
		}
	}
	return
}

func DecompressScript(data []byte) (script []byte) {
	switch data[0] {
	case 0x00:
		script = make([]byte, 25)
		script[0] = 0x76
		script[1] = 0xa9
		script[2] = 20
		copy(script[3:23], data[1:21])
		script[23] = 0x88
		script[24] = 0xac

	case 0x01:
		script = make([]byte, 23)
		script[0] = 0xa9
		script[1] = 20
		copy(script[2:22], data[1:])
		script[22] = 0x87

	case 0x02, 0x03:
		script = make([]byte, 35)
		script[0] = 33
		copy(script[1:34], data)
		script[34] = 0xac

	case 0x04, 0x05:
		var vch [33]byte
		var pk secp256k1.XY
		vch[0] = data[0] - 2
		copy(vch[1:], data[1:])
		pk.ParsePubkey(vch[:])
		script = make([]byte, 67)
		script[0] = 65
		pk.GetPublicKey(script[1:66])
		script[66] = 0xac
	}
	return
}
