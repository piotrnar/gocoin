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

func (c *SigChecker) ExecuteWitnessScript(stack *scrStack, scriptPubKey []byte, flags uint32, sigversion int, execdata *btc.ScriptExecutionData) bool {
    /*
	if (sigversion == SigVersion::TAPSCRIPT) {
        // OP_SUCCESSx processing overrides everything, including stack element size limits
        CScript::const_iterator pc = scriptPubKey.begin();
        while (pc < scriptPubKey.end()) {
            opcodetype opcode;
            if (!scriptPubKey.GetOp(pc, opcode)) {
                // Note how this condition would not be reached if an unknown OP_SUCCESSx was found
                return set_error(serror, SCRIPT_ERR_BAD_OPCODE);
            }
            // New opcodes will be listed here. May use a different sigversion to modify existing opcodes.
            if (IsOpSuccess(opcode)) {
                if (flags & SCRIPT_VERIFY_DISCOURAGE_OP_SUCCESS) {
                    return set_error(serror, SCRIPT_ERR_DISCOURAGE_OP_SUCCESS);
                }
                return set_success(serror);
            }
        }

        // Tapscript enforces initial stack size limits (altstack is empty here)
        if (stack.size() > MAX_STACK_SIZE) return set_error(serror, SCRIPT_ERR_STACK_SIZE);
    }
	*/

	// Disallow stack item size > MAX_SCRIPT_ELEMENT_SIZE in witness stack
	for i:=0; i<stack.size(); i++ {
		if len(stack.at(i)) > btc.MAX_SCRIPT_ELEMENT_SIZE {
			if DBG_ERR {
				fmt.Println("SCRIPT_ERR_PUSH_SIZE")
			}
			return false
		}
	}

    // Run the script interpreter.
	if !evalScript(scriptPubKey, stack, c, flags, SIGVERSION_WITNESS_V0, execdata) {
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
	
func VerifyWitnessProgram(witness *witness_ctx, checker *SigChecker, witversion int, program []byte, flags uint32, is_p2sh bool) bool {
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
			if (witness.stack.size() != 2) {
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
				fmt.Println("VER_TAPROOT not set")
			}
			return false
		}
		stack.copy_from(&witness.stack)
		
		if stack.size() == 0 {
			fmt.Println("SCRIPT_ERR_WITNESS_PROGRAM_WITNESS_EMPTY")
			return false
		}

        execdata.M_annex_present = false
		if stack.size() >= 2 {
			dat := stack.top(-1)
			if len(dat) > 0 && dat[0] == ANNEX_TAG {
            	// Drop annex (this is non-standard; see IsWitnessStandard)
				annex := stack.pop()
				sha := sha256.New()
				sha.Write(annex)
				execdata.M_annex_hash = sha.Sum(nil)
				execdata.M_annex_present = true
			}
        }
		
		execdata.M_annex_init = true

        if stack.size() == 1 {
            // Key path spending (stack size is 1 after removing optional annex)
            if (!checker.CheckSchnorrSignature(stack.top(-1), program, SIGVERSION_TAPROOT, &execdata)) {
				println("schnorr sig Bad")
                return false; // serror is set
            }
			println("schnorr sig OK")
			return true
        } else {
            // Script path spending (stack size is >1 after removing optional annex)
			println("==== TAP TAP not implemented =====")
			/*
            const valtype& control = SpanPopBack(stack);
            const valtype& script_bytes = SpanPopBack(stack);
            exec_script = CScript(script_bytes.begin(), script_bytes.end());
            if (control.size() < TAPROOT_CONTROL_BASE_SIZE || control.size() > TAPROOT_CONTROL_MAX_SIZE || ((control.size() - TAPROOT_CONTROL_BASE_SIZE) % TAPROOT_CONTROL_NODE_SIZE) != 0) {
                return set_error(serror, SCRIPT_ERR_TAPROOT_WRONG_CONTROL_SIZE);
            }
            if (!VerifyTaprootCommitment(control, program, exec_script, execdata.m_tapleaf_hash)) {
                return set_error(serror, SCRIPT_ERR_WITNESS_PROGRAM_MISMATCH);
            }
            execdata.m_tapleaf_hash_init = true;
            if ((control[0] & TAPROOT_LEAF_MASK) == TAPROOT_LEAF_TAPSCRIPT) {
                // Tapscript (leaf version 0xc0)
                execdata.m_validation_weight_left = ::GetSerializeSize(witness.stack, PROTOCOL_VERSION) + VALIDATION_WEIGHT_OFFSET;
                execdata.m_validation_weight_left_init = true;
                return ExecuteWitnessScript(stack, exec_script, flags, SigVersion::TAPSCRIPT, checker, execdata, serror);
            }
            if (flags & SCRIPT_VERIFY_DISCOURAGE_UPGRADABLE_TAPROOT_VERSION) {
                return set_error(serror, SCRIPT_ERR_DISCOURAGE_UPGRADABLE_TAPROOT_VERSION);
            }
            return set_success(serror);
			*/
			return false
		}
	} else if (flags&VER_WITNESS_PROG) != 0 {
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
