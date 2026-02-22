package sipadll

/*
  EC_Verify / Schnorr_Verify / CheckPayToContract speedup for Windows.
  Calls libsecp256k1-5.dll directly (built with --enable-shared --enable-module-schnorrsig).
  No custom wrapper DLL needed.
*/

import (
	"encoding/hex"
	"os"
	"syscall"
	"unsafe"
)

var (
	dll = syscall.NewLazyDLL("libsecp256k1-5.dll")

	procContextCreate              = dll.NewProc("secp256k1_context_create")
	procEcPubkeyParse              = dll.NewProc("secp256k1_ec_pubkey_parse")
	procEcdsaSignatureParseCompact = dll.NewProc("secp256k1_ecdsa_signature_parse_compact")
	procEcdsaSignatureNormalize    = dll.NewProc("secp256k1_ecdsa_signature_normalize")
	procEcdsaVerify                = dll.NewProc("secp256k1_ecdsa_verify")
	procXonlyPubkeyParse           = dll.NewProc("secp256k1_xonly_pubkey_parse")
	procSchnorrsigVerify           = dll.NewProc("secp256k1_schnorrsig_verify")
	procXonlyPubkeyTweakAddCheck   = dll.NewProc("secp256k1_xonly_pubkey_tweak_add_check")

	ctx uintptr
)

const SECP256K1_CONTEXT_VERIFY = 0x0101

// ecdsaSignatureParseDerLax is a Go port of the lax DER parser from Bitcoin Core.
func ecdsaSignatureParseDerLax(input []byte) (sig [64]byte, ok bool) {
	var tmpsig [64]byte

	syscall.SyscallN(procEcdsaSignatureParseCompact.Addr(),
		ctx, uintptr(unsafe.Pointer(&sig[0])), uintptr(unsafe.Pointer(&tmpsig[0])))

	inputlen := len(input)
	pos := 0

	if pos == inputlen || input[pos] != 0x30 {
		return sig, false
	}
	pos++

	if pos == inputlen {
		return sig, false
	}
	lenbyte := int(input[pos])
	pos++
	if lenbyte&0x80 != 0 {
		lenbyte -= 0x80
		if pos+lenbyte > inputlen {
			return sig, false
		}
		pos += lenbyte
	}

	// R
	if pos == inputlen || input[pos] != 0x02 {
		return sig, false
	}
	pos++
	if pos == inputlen {
		return sig, false
	}

	var rlen int
	lenbyte = int(input[pos])
	pos++
	if lenbyte&0x80 != 0 {
		lenbyte -= 0x80
		if pos+lenbyte > inputlen {
			return sig, false
		}
		for lenbyte > 0 && input[pos] == 0 {
			pos++
			lenbyte--
		}
		if lenbyte >= 8 {
			return sig, false
		}
		rlen = 0
		for lenbyte > 0 {
			rlen = (rlen << 8) + int(input[pos])
			pos++
			lenbyte--
		}
	} else {
		rlen = lenbyte
	}
	if rlen > inputlen-pos {
		return sig, false
	}
	rpos := pos
	pos += rlen

	// S
	if pos == inputlen || input[pos] != 0x02 {
		return sig, false
	}
	pos++
	if pos == inputlen {
		return sig, false
	}

	var slen int
	lenbyte = int(input[pos])
	pos++
	if lenbyte&0x80 != 0 {
		lenbyte -= 0x80
		if pos+lenbyte > inputlen {
			return sig, false
		}
		for lenbyte > 0 && input[pos] == 0 {
			pos++
			lenbyte--
		}
		if lenbyte >= 8 {
			return sig, false
		}
		slen = 0
		for lenbyte > 0 {
			slen = (slen << 8) + int(input[pos])
			pos++
			lenbyte--
		}
	} else {
		slen = lenbyte
	}
	if slen > inputlen-pos {
		return sig, false
	}
	spos := pos

	// Copy R
	overflow := false
	for rlen > 0 && input[rpos] == 0 {
		rlen--
		rpos++
	}
	if rlen > 32 {
		overflow = true
	} else {
		copy(tmpsig[32-rlen:32], input[rpos:rpos+rlen])
	}

	// Copy S
	for slen > 0 && input[spos] == 0 {
		slen--
		spos++
	}
	if slen > 32 {
		overflow = true
	} else {
		copy(tmpsig[64-slen:64], input[spos:spos+slen])
	}

	if !overflow {
		r, _, _ := syscall.SyscallN(procEcdsaSignatureParseCompact.Addr(),
			ctx, uintptr(unsafe.Pointer(&sig[0])), uintptr(unsafe.Pointer(&tmpsig[0])))
		if r == 0 {
			overflow = true
		}
	}
	if overflow {
		var zeros [64]byte
		syscall.SyscallN(procEcdsaSignatureParseCompact.Addr(),
			ctx, uintptr(unsafe.Pointer(&sig[0])), uintptr(unsafe.Pointer(&zeros[0])))
	}
	return sig, true
}

func EC_Verify(pkey, sign, hash []byte) bool {
	var pubkey [64]byte

	r, _, _ := syscall.SyscallN(procEcPubkeyParse.Addr(),
		ctx,
		uintptr(unsafe.Pointer(&pubkey[0])),
		uintptr(unsafe.Pointer(&pkey[0])),
		uintptr(len(pkey)))
	if r == 0 {
		return false
	}

	sig, ok := ecdsaSignatureParseDerLax(sign)
	if !ok {
		return false
	}

	syscall.SyscallN(procEcdsaSignatureNormalize.Addr(),
		ctx,
		uintptr(unsafe.Pointer(&sig[0])),
		uintptr(unsafe.Pointer(&sig[0])))

	result, _, _ := syscall.SyscallN(procEcdsaVerify.Addr(),
		ctx,
		uintptr(unsafe.Pointer(&sig[0])),
		uintptr(unsafe.Pointer(&hash[0])),
		uintptr(unsafe.Pointer(&pubkey[0])))

	return result == 1
}

func Schnorr_Verify(pkey, sign, msg []byte) bool {
	var pubkey [64]byte

	r, _, _ := syscall.SyscallN(procXonlyPubkeyParse.Addr(),
		ctx,
		uintptr(unsafe.Pointer(&pubkey[0])),
		uintptr(unsafe.Pointer(&pkey[0])))
	if r == 0 {
		return false
	}

	result, _, _ := syscall.SyscallN(procSchnorrsigVerify.Addr(),
		ctx,
		uintptr(unsafe.Pointer(&sign[0])),
		uintptr(unsafe.Pointer(&msg[0])),
		32,
		uintptr(unsafe.Pointer(&pubkey[0])))

	return result == 1
}

func CheckPayToContract(kd, base, hash []byte, parity bool) bool {
	var basePoint [64]byte

	r, _, _ := syscall.SyscallN(procXonlyPubkeyParse.Addr(),
		ctx,
		uintptr(unsafe.Pointer(&basePoint[0])),
		uintptr(unsafe.Pointer(&base[0])))
	if r == 0 {
		return false
	}

	var par uintptr
	if parity {
		par = 1
	}

	result, _, _ := syscall.SyscallN(procXonlyPubkeyTweakAddCheck.Addr(),
		ctx,
		uintptr(unsafe.Pointer(&kd[0])),
		par,
		uintptr(unsafe.Pointer(&basePoint[0])),
		uintptr(unsafe.Pointer(&hash[0])))

	return result == 1
}

func ec_verify() bool {
	key, _ := hex.DecodeString("020eaebcd1df2df853d66ce0e1b0fda07f67d1cabefde98514aad795b86a6ea66d")
	sig, _ := hex.DecodeString("3045022100fe00e013c244062847045ae7eb73b03fca583e9aa5dbd030a8fd1c6dfcf11b1002207d0d04fed8fa1e93007468d5a9e134b0a7023b6d31db4e50942d43a250f4d07c01")
	has, _ := hex.DecodeString("3382219555ddbb5b00e0090f469e590ba1eae03c7f28ab937de330aa60294ed6")
	return EC_Verify(key, sig, has)
}

func schnorr_verify() bool {
	key, _ := hex.DecodeString("DFF1D77F2A671C5F36183726DB2341BE58FEAE1DA2DECED843240F7B502BA659")
	sig, _ := hex.DecodeString("6896BD60EEAE296DB48A229FF71DFE071BDE413E6D43F917DC8DCF8C78DE33418906D11AC976ABCCB20B091292BFF4EA897EFCB639EA871CFA95F6DE339E4B0A")
	msg, _ := hex.DecodeString("243F6A8885A308D313198A2E03707344A4093822299F31D0082EFA98EC4E6C89")
	return Schnorr_Verify(key, sig, msg)
}

func p2scr_verify() bool {
	kd, _ := hex.DecodeString("afaf8a67be00186668f74740e34ffce748139c2b73c9fbd2c1f33e48a612a75d")
	base, _ := hex.DecodeString("f1cbd3f2430910916144d5d2bf63d48a6281e5b8e6ade31413adccff3d8839d4")
	hash, _ := hex.DecodeString("93a760e87123883022cbd462ac40571176cf09d9d2c6168759fee6c2b079fdd8")
	return CheckPayToContract(kd, base, hash, true)
}

func init() {
	r, _, _ := syscall.SyscallN(procContextCreate.Addr(), SECP256K1_CONTEXT_VERIFY)
	ctx = r
	if ctx == 0 {
		println("ERROR: secp256k1_context_create failed")
		os.Exit(1)
	}

	if !ec_verify() {
		println("ERROR: Could not initiate libsecp256k1-5.dll (EC_Verify failed)")
		os.Exit(1)
	}
	if !schnorr_verify() {
		println("ERROR: Could not initiate libsecp256k1-5.dll (Schnorr_Verify failed)")
		os.Exit(1)
	}
	if !p2scr_verify() {
		println("ERROR: Could not initiate libsecp256k1-5.dll (CheckPayToContract failed)")
		os.Exit(1)
	}
}
