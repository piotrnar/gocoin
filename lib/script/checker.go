package script

import (
	"encoding/hex"
	"fmt"
	"github.com/piotrnar/gocoin/lib/btc"
)

type SigChecker struct {
	Tx     *btc.Tx
	Idx    int
	Amount uint64
}

func (c *SigChecker) evalChecksig(vchSig, vchPubKey, p []byte, pbegincodehash int, execdata *btc.ScriptExecutionData, ver_flags uint32, sigversion int) (ok, fSuccess bool) {
	if sigversion == SIGVERSION_BASE || sigversion == SIGVERSION_WITNESS_V0 {
		return c.evalChecksigPreTapscript(vchSig, vchPubKey, p, pbegincodehash, execdata, ver_flags, sigversion)
	}
	if sigversion == SIGVERSION_TAPSCRIPT {
		return c.evalChecksigTapscript(vchSig, vchPubKey, execdata, ver_flags, sigversion)
	}
	panic("should not get here")
	return
}

func (c *SigChecker) evalChecksigTapscript(sig, pubkey []byte, execdata *btc.ScriptExecutionData, flags uint32, sigversion int) (ok, success bool) {
	/*
	 *  The following validation sequence is consensus critical. Please note how --
	 *    upgradable public key versions precede other rules;
	 *    the script execution fails when using empty signature with invalid public key;
	 *    the script execution fails when using non-empty invalid signature.
	 */
	success = len(sig) > 0
	if success {
		// Implement the sigops/witnesssize ratio test.
		// Passing with an upgradable public key version is also counted.
		execdata.M_validation_weight_left -= VALIDATION_WEIGHT_PER_SIGOP_PASSED
		if execdata.M_validation_weight_left < 0 {
			if DBG_ERR {
				fmt.Println("SCRIPT_ERR_TAPSCRIPT_VALIDATION_WEIGHT")
			}
			return
		}
	}
	if len(pubkey) == 0 {
		if DBG_ERR {
			fmt.Println("SCRIPT_ERR_PUBKEYTYPE")
		}
		return
	} else if len(pubkey) == 32 {
		if success && !c.CheckSchnorrSignature(sig, pubkey, sigversion, execdata) {
			if DBG_ERR {
				fmt.Println("evalChecksigTapscript: CheckSchnorrSignature failed")
			}
			return
		}
	} else {
		/*
		 *  New public key version softforks should be defined before this `else` block.
		 *  Generally, the new code should not do anything but failing the script execution. To avoid
		 *  consensus bugs, it should not modify any existing values (including `success`).
		 */
		if (flags & VER_DIS_PUBKEYTYPE) != 0 {
			fmt.Println("SCRIPT_ERR_DISCOURAGE_UPGRADABLE_PUBKEYTYPE")
			return
		}
	}

	ok = true
	return
}

func (c *SigChecker) evalChecksigPreTapscript(vchSig, vchPubKey, p []byte, pbegincodehash int, execdata *btc.ScriptExecutionData, ver_flags uint32, sigversion int) (ok, fSuccess bool) {
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

	if !fSuccess && (ver_flags&VER_NULLFAIL) != 0 && len(vchSig) > 0 {
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

	if sigversion == SIGVERSION_WITNESS_V0 {
		sh = c.Tx.WitnessSigHash(data, c.Amount, c.Idx, nHashType)
	} else {
		sh = c.Tx.SignatureHash(data, c.Idx, nHashType)
	}
	return btc.EcdsaVerify(pubkey, sig, sh)
}

func (c *SigChecker) CheckSchnorrSignature(sig, pubkey []byte, sigversion int, execdata *btc.ScriptExecutionData) bool {
	if len(sig) != 64 && len(sig) != 65 {
		if DBG_ERR {
			fmt.Println("SCRIPT_ERR_SCHNORR_SIG_SIZE")
		}
		return false
	}
	hashtype := byte(btc.SIGHASH_DEFAULT)
	if len(sig) == 65 {
		hashtype = sig[64]
		if hashtype == btc.SIGHASH_DEFAULT {
			if DBG_ERR {
				fmt.Println("SCRIPT_ERR_SCHNORR_SIG_HASHTYPE")
			}
			return false
		}
		sig = sig[:64]
	}
	sh := c.Tx.TaprootSigHash(execdata, c.Idx, hashtype, sigversion == SIGVERSION_TAPSCRIPT)
	return btc.SchnorrVerify(pubkey, sig, sh)
}
