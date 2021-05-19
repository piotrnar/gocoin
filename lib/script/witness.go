package script

import (
	"fmt"
	"bytes"
	"encoding/hex"
	"crypto/sha256"
	"github.com/piotrnar/gocoin/lib/btc"
)

type witness_ctx struct {
	stack scrStack
}

func (w *witness_ctx) IsNull() bool {
	return w.stack.size()==0
}

func VerifyWitnessProgram(witness *witness_ctx, amount uint64, tx *btc.Tx, inp int, witversion int, program []byte, flags uint32, is_p2sh bool) bool {
	var stack scrStack
	var scriptPubKey []byte

	if DBG_SCR {
		fmt.Println("*****************VerifyWitnessProgram", len(tx.SegWit), witversion, flags, witness.stack.size(), len(program))
	}

	if witversion == 0 {
		if len(program) == 32 {
			// Version 0 segregated witness program: SHA256(CScript) inside the program, CScript + inputs in witness
			if witness.stack.size() == 0 {
				if DBG_ERR {
					fmt.Println("SCRIPT_ERR_WITNESS_PROGRAM_WITNESS_EMPTY")
				}
				return false
			}
			scriptPubKey = witness.stack.pop()
			sha := sha256.New()
			sha.Write(scriptPubKey)
			sum := sha.Sum(nil)
			if !bytes.Equal(program, sum) {
				if DBG_ERR {
					fmt.Println("32-SCRIPT_ERR_WITNESS_PROGRAM_MISMATCH")
					fmt.Println(hex.EncodeToString(program))
					fmt.Println(hex.EncodeToString(sum))
					fmt.Println(hex.EncodeToString(scriptPubKey))
				}
				return false
			}
			stack.copy_from(&witness.stack)
			witness.stack.push(scriptPubKey)
		} else if len(program) == 20 {
			// Special case for pay-to-pubkeyhash; signature + pubkey in witness
			if (witness.stack.size() != 2) {
				if DBG_ERR {
					fmt.Println("20-SCRIPT_ERR_WITNESS_PROGRAM_MISMATCH", tx.Hash.String())
				}
				return false
			}

			scriptPubKey = make([]byte, 25)
			scriptPubKey[0] = 0x76
			scriptPubKey[1] = 0xa9
			scriptPubKey[2] = 0x14
			copy(scriptPubKey[3:23], program)
			scriptPubKey[23] = 0x88
			scriptPubKey[24] = 0xac
			stack.copy_from(&witness.stack)
		} else {
			if DBG_ERR {
				fmt.Println("SCRIPT_ERR_WITNESS_PROGRAM_WRONG_LENGTH")
			}
			return false
		}
	} else if witversion == 1 && len(program) == 32 && !is_p2sh {
		if (flags & VER_TAPROOT) == 0 {
			if DBG_ERR {
				fmt.Println("VER_TAPROOT not set")
			}
			return false
		}
		println("==== TAP TAP =====")
		return false
	} else if (flags&VER_WITNESS_PROG) != 0 {
		if DBG_ERR {
			fmt.Println("SCRIPT_ERR_DISCOURAGE_UPGRADABLE_WITNESS_PROGRAM")
		}
		return false
	} else {
		// Higher version witness scripts return true for future softfork compatibility
		return true
	}

	if DBG_SCR {
		fmt.Println("*****************", stack.size())
	}
	// Disallow stack item size > MAX_SCRIPT_ELEMENT_SIZE in witness stack
	for i:=0; i<stack.size(); i++ {
		if len(stack.at(i)) > btc.MAX_SCRIPT_ELEMENT_SIZE {
			if DBG_ERR {
				fmt.Println("SCRIPT_ERR_PUSH_SIZE")
			}
			return false
		}
	}

	if !evalScript(scriptPubKey, amount, &stack, tx, inp, flags, SIGVERSION_WITNESS_V0) {
		return false
	}

	// Scripts inside witness implicitly require cleanstack behaviour
	if stack.size() != 1 {
		if DBG_ERR {
			fmt.Println("SCRIPT_ERR_EVAL_FALSE")
		}
		return false
	}

	if !stack.topBool(-1) {
		if DBG_ERR {
			fmt.Println("SCRIPT_ERR_EVAL_FALSE")
		}
		return false
	}
	return true
}
