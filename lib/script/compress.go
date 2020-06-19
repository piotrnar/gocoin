package script

import (
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
	var pk secp256k1.XY
	if len(scr) == 35 && scr[0] == 33 && scr[34] == 0xac && (scr[1] == 0x02 || scr[1] == 0x03) {
		return true, scr[1:34]
	}
	if len(scr) == 67 && scr[0] == 65 && scr[66] == 0xac && scr[1] == 0x04 {
		if pk.ParsePubkey(scr[1:66]) && pk.IsValid() {
			return true, scr[1:66]
		}
		//println("invalid uncompressed pubkey")
	}
	return false, nil
}

func IsUnspendable(scr []byte) bool {
	return len(scr) > 0 && scr[0] == 0x6a
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
	switch(data[0]) {
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
