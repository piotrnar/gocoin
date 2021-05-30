package script

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"github.com/piotrnar/gocoin/lib/btc"
)

type witness_ctx struct {
	stack scrStack
}

func (w *witness_ctx) IsNull() bool {
	return w.stack.size() == 0
}

func (c *SigChecker) ExecuteWitnessScript(stack *scrStack, scriptPubKey []byte, flags uint32, sigversion int, execdata *btc.ScriptExecutionData) bool {
	if sigversion == SIGVERSION_TAPSCRIPT {
		var pc int
		for pc < len(scriptPubKey) {
			opcode, _, le, e := btc.GetOpcode(scriptPubKey[pc:])
			if e != nil {
				// Note how this condition would not be reached if an unknown OP_SUCCESSx was found
				if DBG_ERR {
					fmt.Println("SCRIPT_ERR_BAD_OPCODE")
				}
				return false
			}
			// New opcodes will be listed here. May use a different sigversion to modify existing opcodes.
			if IsOpSuccess(opcode) {
				if (flags & VER_DIS_SUCCESS) != 0 {
					if DBG_ERR {
						fmt.Println("SCRIPT_ERR_DISCOURAGE_OP_SUCCESS")
					}
					return false
				}
				if DBG_ERR {
					fmt.Println("IsOpSuccess - yes")
				}
				return true
			}
			pc += le
		}
		// Tapscript enforces initial stack size limits (altstack is empty here)
		if stack.size() > MAX_STACK_SIZE {
			if DBG_ERR {
				fmt.Println("SCRIPT_ERR_STACK_SIZE")
			}
			return false
		}
	}

	// Disallow stack item size > MAX_SCRIPT_ELEMENT_SIZE in witness stack
	for i := 0; i < stack.size(); i++ {
		if len(stack.at(i)) > btc.MAX_SCRIPT_ELEMENT_SIZE {
			if DBG_ERR {
				fmt.Println("SCRIPT_ERR_PUSH_SIZE")
			}
			return false
		}
	}

	// Run the script interpreter.
	//DBG_SCR = true
	if !evalScript(scriptPubKey, stack, c, flags, sigversion, execdata) {
		if DBG_ERR {
			fmt.Println("eval script failed")
		}
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

func (checker *SigChecker) VerifyWitnessProgram(witness *witness_ctx, witversion int, program []byte, flags uint32, is_p2sh bool) bool {
	var stack scrStack
	var scriptPubKey []byte
	var execdata btc.ScriptExecutionData

	if DBG_SCR {
		fmt.Println("*****************VerifyWitnessProgram", len(checker.Tx.SegWit), witversion, flags, witness.stack.size(), len(program))
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
			stack.copy_from(&witness.stack)
			scriptPubKey = stack.pop()
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
			return checker.ExecuteWitnessScript(&stack, scriptPubKey, flags, SIGVERSION_WITNESS_V0, &execdata)
		} else if len(program) == 20 {
			// Special case for pay-to-pubkeyhash; signature + pubkey in witness
			if witness.stack.size() != 2 {
				if DBG_ERR {
					fmt.Println("20-SCRIPT_ERR_WITNESS_PROGRAM_MISMATCH", checker.Tx.Hash.String())
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
			return checker.ExecuteWitnessScript(&stack, scriptPubKey, flags, SIGVERSION_WITNESS_V0, &execdata)
		} else {
			if DBG_ERR {
				fmt.Println("SCRIPT_ERR_WITNESS_PROGRAM_WRONG_LENGTH")
			}
			return false
		}
	} else if witversion == 1 && len(program) == 32 && !is_p2sh {
		// TAPROOT
		if (flags & VER_TAPROOT) == 0 {
			if DBG_ERR {
				fmt.Println("VER_TAPROOT not set - verify script OK")
			}
			return true
		}
		stack.copy_from(&witness.stack)

		if stack.size() == 0 {
			fmt.Println("SCRIPT_ERR_WITNESS_PROGRAM_WITNESS_EMPTY")
			return false
		}

		execdata.M_annex_hash = nil
		if stack.size() >= 2 {
			dat := stack.top(-1)
			if len(dat) > 0 && dat[0] == ANNEX_TAG {
				// Drop annex (this is non-standard; see IsWitnessStandard)
				annex := stack.pop()
				sha := sha256.New()
				btc.WriteVlen(sha, uint64(len(annex)))
				sha.Write(annex)
				execdata.M_annex_hash = sha.Sum(nil)
			}
		}
		// execdata.M_annex_init - it doesn't seem like we need this

		if stack.size() == 1 {
			// Key path spending (stack size is 1 after removing optional annex)
			if !checker.CheckSchnorrSignature(stack.top(-1), program, SIGVERSION_TAPROOT, &execdata) {
				return false // serror is set
			}
			return true
		} else {
			// Script path spending (stack size is >1 after removing optional annex)
			control := stack.pop()
			script_bytes := stack.pop()
			//exec_script = script_bytes
			if len(control) < TAPROOT_CONTROL_BASE_SIZE || len(control) > TAPROOT_CONTROL_MAX_SIZE || ((len(control)-TAPROOT_CONTROL_BASE_SIZE)%TAPROOT_CONTROL_NODE_SIZE) != 0 {
				if DBG_ERR {
					fmt.Println("SCRIPT_ERR_TAPROOT_WRONG_CONTROL_SIZE")
				}
				return false
			}

			if !VerifyTaprootCommitment(control, program, script_bytes, &execdata.M_tapleaf_hash) {
				if DBG_ERR {
					fmt.Println("VerifyTaprootCommitment: SCRIPT_ERR_WITNESS_PROGRAM_MISMATCH")
				}
				return false
			}

			if (control[0] & TAPROOT_LEAF_MASK) == TAPROOT_LEAF_TAPSCRIPT {
				// Tapscript (leaf version 0xc0)
				execdata.M_validation_weight_left = int64(witness.stack.GetSerializeSize() + VALIDATION_WEIGHT_OFFSET)
				execdata.M_validation_weight_left_init = true
				return checker.ExecuteWitnessScript(&stack, script_bytes, flags, SIGVERSION_TAPSCRIPT, &execdata)
			}
			if (flags & VER_DIS_TAPVER) != 0 {
				fmt.Println("SCRIPT_ERR_DISCOURAGE_UPGRADABLE_TAPROOT_VERSION")
				return false
			}
			return true
		}
	} else if (flags & VER_WITNESS_PROG) != 0 {
		if DBG_ERR {
			fmt.Println("SCRIPT_ERR_DISCOURAGE_UPGRADABLE_WITNESS_PROGRAM")
		}
		return false
	} else {
		// Higher version witness scripts return true for future softfork compatibility
		return true
	}
	// There is intentionally no return statement here, to be able to use "control reaches end of non-void function" warnings to detect gaps in the logic above.
}

func lexicographical_compare(d1, d2 []byte) bool {
	first1, first2 := 0, 0
	last1, last2 := len(d1), len(d2)
	for first1 != last1 {
		if first2 == last2 || d2[first2] < d1[first1] {
			return false
		}
		if d1[first1] < d2[first2] {
			return true
		}
		first1++
		first2++
	}
	return first2 != last2
}

// (control, program, script_bytes, execdata.M_tapleaf_hash)
func VerifyTaprootCommitment(control, program, script []byte, tapleaf_hash *[]byte) bool {
	path_len := (len(control) - TAPROOT_CONTROL_BASE_SIZE) / TAPROOT_CONTROL_NODE_SIZE
	//! The internal pubkey (x-only, so no Y coordinate parity).
	p := control[1:TAPROOT_CONTROL_BASE_SIZE]
	//! The output pubkey (taken from the scriptPubKey).
	q := program //const XOnlyPubKey q{uint256(program)};
	// Compute the tapleaf hash.

	sha := btc.Hasher(btc.HASHER_TAPLEAF)
	sha.Write([]byte{control[0] & TAPROOT_LEAF_MASK})
	btc.WriteVlen(sha, uint64(len(script)))
	sha.Write(script)
	*tapleaf_hash = sha.Sum(nil)

	// Compute the Merkle root from the leaf and the provided path.
	k := *tapleaf_hash
	for i := 0; i < path_len; i++ {
		ss_branch := btc.Hasher(btc.HASHER_TAPBRANCH)
		tmp := TAPROOT_CONTROL_BASE_SIZE + TAPROOT_CONTROL_NODE_SIZE*i
		node := control[tmp : tmp+TAPROOT_CONTROL_NODE_SIZE]

		if lexicographical_compare(k, node) {
			ss_branch.Write(k)
			ss_branch.Write(node)
		} else {
			ss_branch.Write(node)
			ss_branch.Write(k)
		}
		k = ss_branch.Sum(nil)
	}

	// Compute the tweak from the Merkle root and the internal pubkey.
	sha = btc.Hasher(btc.HASHER_TAPTWEAK)
	sha.Write(p)
	sha.Write(k)
	k = sha.Sum(nil)
	// Verify that the output pubkey matches the tweaked internal pubkey, after correcting for parity.
	return btc.CheckPayToContract(q, p, k, (control[0]&1) != 0)
}
