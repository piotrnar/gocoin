package script

import (
	"fmt"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/secp256k1"
)


type SigChecker struct {
	Tx *btc.Tx
	Idx int
	Amount uint64
}

func (c *SigChecker) evalChecksig(vchSig, vchPubKey, p []byte, pbegincodehash int, execdata *btc.ScriptExecutionData, ver_flags uint32, sigversion int) (ok, fSuccess bool) {
	scriptCode := p[pbegincodehash:]

	// Drop the signature in pre-segwit scripts but not segwit scripts
	if sigversion == SIGVERSION_BASE {
		var found int
		scriptCode, found = delSig(scriptCode, vchSig)
		if found > 0 && (ver_flags&VER_CONST_SCRIPTCODE) != 0 {
			if DBG_ERR {
				fmt.Println("SCRIPT_ERR_SIG_FINDANDDELETE SIN")
			}
			return
		}
	}

	// BIP-0066
	if !CheckSignatureEncoding(vchSig, ver_flags) || !CheckPubKeyEncoding(vchPubKey, ver_flags, sigversion) {
		if DBG_ERR {
			fmt.Println("Invalid Signature Encoding A")
		}
		return
	}

	if len(vchSig) > 0 {
		var sh []byte
		if sigversion == SIGVERSION_WITNESS_V0 {
			if DBG_SCR {
				fmt.Println("getting WitnessSigHash error")
			}
			sh = c.Tx.WitnessSigHash(scriptCode, c.Amount, c.Idx, int32(vchSig[len(vchSig)-1]))
		} else {
			sh = c.Tx.SignatureHash(scriptCode, c.Idx, int32(vchSig[len(vchSig)-1]))
		}
		if DBG_SCR {
			fmt.Println("EcdsaVerify", hex.EncodeToString(sh))
			fmt.Println(" key:", hex.EncodeToString(vchPubKey))
			fmt.Println(" sig:", hex.EncodeToString(vchSig))
		}
		fSuccess = btc.EcdsaVerify(vchPubKey, vchSig, sh)
		if DBG_SCR {
			fmt.Println(" ->", fSuccess)
		}
	}
	if !fSuccess && DBG_SCR {
		fmt.Println("EcdsaVerify fail 1", c.Tx.Hash.String())
	}

	if !fSuccess && (ver_flags&VER_NULLFAIL)!=0 && len(vchSig)>0 {
		if DBG_ERR {
			fmt.Println("SCRIPT_ERR_SIG_NULLFAIL-1")
		}
		return
	}
	ok = true
	return
}


func (c *SigChecker) verifyECDSA(data, sig, pubkey []byte, sigversion int) bool {
	if len(sig) == 0 {
		return false
	}
	
	nHashType := int32(sig[len(sig)-1])
	
	var sh []byte

	if sigversion==SIGVERSION_WITNESS_V0 {
		sh = c.Tx.WitnessSigHash(data, c.Amount, c.Idx, nHashType)
	} else {
		sh = c.Tx.SignatureHash(data, c.Idx, nHashType)
	}
	return btc.EcdsaVerify(pubkey, sig, sh)
}

func (c *SigChecker) CheckSchnorrSignature(sig, pubkey []byte, sigversion int, execdata *btc.ScriptExecutionData) bool {
	if len(sig) != 64 && len(sig) != 65 {
		fmt.Println("SCRIPT_ERR_SCHNORR_SIG_SIZE")
		return false
	}
	hashtype := byte(btc.SIGHASH_DEFAULT)
	if len(sig) == 65 {
		hashtype = sig[64]
		if hashtype == btc.SIGHASH_DEFAULT {
			fmt.Println("SCRIPT_ERR_SCHNORR_SIG_HASHTYPE")
			return false
		}
		sig = sig[:64]
	}
	sh := c.Tx.TaprootSigHash(execdata, c.Idx, hashtype, sigversion == SIGVERSION_TAPSCRIPT)
	return secp256k1.SchnorrVerify(pubkey, sig, sh)
}
