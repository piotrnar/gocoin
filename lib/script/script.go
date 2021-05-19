package script

import (
	"fmt"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"crypto/sha256"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/ripemd160"
	"runtime/debug"
)


type VerifyConsensusFunction func(pkScr []byte, amount uint64, i int, tx *btc.Tx, ver_flags uint32, result bool)

var (
	DBG_SCR = false
	DBG_ERR = true

	VerifyConsensus VerifyConsensusFunction
)

const (
	MAX_SCRIPT_SIZE = 10000

	VER_P2SH = 1<<0
	VER_STRICTENC = 1<<1
	VER_DERSIG = 1<<2
	VER_LOW_S = 1<<3
	VER_NULLDUMMY = 1<<4
	VER_SIGPUSHONLY = 1<<5
	VER_MINDATA = 1<<6
	VER_BLOCK_OPS = 1<<7 // othewise known as DISCOURAGE_UPGRADABLE_NOPS
	VER_CLEANSTACK = 1<<8
	VER_CLTV = 1<<9
	VER_CSV = 1<<10
	VER_WITNESS = 1<<11
	VER_WITNESS_PROG = 1<<12 // DISCOURAGE_UPGRADABLE_WITNESS_PROGRAM
	VER_MINIMALIF = 1<<13
	VER_NULLFAIL = 1<<14
	VER_WITNESS_PUBKEY = 1 << 15 // WITNESS_PUBKEYTYPE
	VER_CONST_SCRIPTCODE = 1 << 16
	VER_TAPROOT = 1 << 17

	STANDARD_VERIFY_FLAGS = VER_P2SH | VER_STRICTENC | VER_DERSIG | VER_LOW_S |
		VER_NULLDUMMY | VER_MINDATA | VER_BLOCK_OPS | VER_CLEANSTACK | VER_CLTV | VER_CSV |
		VER_WITNESS | VER_WITNESS_PROG | VER_MINIMALIF | VER_NULLFAIL | VER_WITNESS_PUBKEY |
		VER_CONST_SCRIPTCODE

	LOCKTIME_THRESHOLD = 500000000
	SEQUENCE_LOCKTIME_DISABLE_FLAG = 1<<31

	SEQUENCE_LOCKTIME_TYPE_FLAG = 1 << 22
	SEQUENCE_LOCKTIME_MASK = 0x0000ffff

	SIGVERSION_BASE = 0
	SIGVERSION_WITNESS_V0 = 1
)


func VerifyTxScript(pkScr []byte, amount uint64, i int, tx *btc.Tx, ver_flags uint32) (result bool) {
	if VerifyConsensus!=nil {
		defer func() {
			// We call CompareToConsensus inside another function to wait for final "result"
			VerifyConsensus(pkScr, amount, i, tx, ver_flags, result)
		}()
	}

	sigScr := tx.TxIn[i].ScriptSig

	if (ver_flags & VER_SIGPUSHONLY) != 0 && !btc.IsPushOnly(sigScr) {
		if DBG_ERR {
			fmt.Println("Not push only")
		}
		return false
	}

	if DBG_SCR {
		fmt.Println("VerifyTxScript", tx.Hash.String(), i+1, "/", len(tx.TxIn))
		fmt.Println("sigScript:", hex.EncodeToString(sigScr[:]))
		fmt.Println("_pkScript:", hex.EncodeToString(pkScr))
		fmt.Printf("flagz:%x\n", ver_flags)
	}

	var stack, stackCopy scrStack
	if !evalScript(sigScr, amount, &stack, tx, i, ver_flags, SIGVERSION_BASE) {
		if DBG_ERR {
			if tx != nil {
				fmt.Println("VerifyTxScript", tx.Hash.String(), i+1, "/", len(tx.TxIn))
			}
			fmt.Println("sigScript failed :", hex.EncodeToString(sigScr[:]))
			fmt.Println("pkScript:", hex.EncodeToString(pkScr[:]))
		}
		return
	}
	if DBG_SCR {
		fmt.Println("\nsigScr verified OK")
		//stack.print()
		fmt.Println()
	}

	if (ver_flags&VER_P2SH)!=0 && stack.size()>0 {
		// copy the stack content to stackCopy
		stackCopy.copy_from(&stack)
	}

	if !evalScript(pkScr, amount, &stack, tx, i, ver_flags, SIGVERSION_BASE) {
		if DBG_SCR {
			fmt.Println("* pkScript failed :", hex.EncodeToString(pkScr[:]))
			fmt.Println("* VerifyTxScript", tx.Hash.String(), i+1, "/", len(tx.TxIn))
			fmt.Println("* sigScript:", hex.EncodeToString(sigScr[:]))
		}
		return
	}

	if stack.size()==0 {
		if DBG_SCR {
			fmt.Println("* stack empty after executing scripts:", hex.EncodeToString(pkScr[:]))
		}
		return
	}

	if !stack.topBool(-1) {
		if DBG_SCR {
			fmt.Println("* FALSE on stack after executing scripts:", hex.EncodeToString(pkScr[:]))
		}
		return
	}

    // Bare witness programs
	var witnessversion int
	var witnessprogram []byte
	var hadWitness bool
	var witness witness_ctx

	if (ver_flags&VER_WITNESS) != 0 {
		if tx.SegWit!=nil {
			for _, wd := range tx.SegWit[i] {
				witness.stack.push(wd)
			}
		}

		witnessversion, witnessprogram = btc.IsWitnessProgram(pkScr)
		if DBG_SCR {
			fmt.Println("------------witnessversion:", witnessversion, "   witnessprogram:", hex.EncodeToString(witnessprogram))
		}
		if witnessprogram!=nil {
			hadWitness = true
			if len(sigScr) != 0 {
				if DBG_ERR {
					fmt.Println("SCRIPT_ERR_WITNESS_MALLEATED")
				}
				return
			}
			if !VerifyWitnessProgram(&witness, amount, tx, i, witnessversion, witnessprogram, ver_flags) {
				if DBG_ERR {
					fmt.Println("VerifyWitnessProgram failed A")
				}
				return false
			}
			// Bypass the cleanstack check at the end. The actual stack is obviously not clean
			// for witness programs.
			stack.resize(1);
		} else {
			if DBG_SCR {
				fmt.Println("No witness program")
			}
		}
	} else {
		if DBG_SCR {
			fmt.Println("Witness flag off")
		}
	}

	// Additional validation for spend-to-script-hash transactions:
	if (ver_flags&VER_P2SH)!=0 && btc.IsPayToScript(pkScr) {
		if DBG_SCR {
			fmt.Println()
			fmt.Println()
			fmt.Println(" ******************* Looks like P2SH script ********************* ")
			stack.print()
		}

		if DBG_SCR {
			fmt.Println("sigScr len", len(sigScr), hex.EncodeToString(sigScr))
		}
		if !btc.IsPushOnly(sigScr) {
			if DBG_ERR {
				fmt.Println("P2SH is not push only")
			}
			return
		}

		// Restore stack.
		stack = stackCopy

		pubKey2 := stack.pop()
		if DBG_SCR {
			fmt.Println("pubKey2:", hex.EncodeToString(pubKey2))
		}

		if !evalScript(pubKey2, amount, &stack, tx, i, ver_flags, SIGVERSION_BASE) {
			if DBG_ERR {
				fmt.Println("P2SH extra verification failed")
			}
			return
		}

		if stack.size()==0 {
			if DBG_SCR {
				fmt.Println("* P2SH stack empty after executing script:", hex.EncodeToString(pubKey2))
			}
			return
		}

		if !stack.topBool(-1) {
			if DBG_SCR {
				fmt.Println("* FALSE on stack after executing P2SH script:", hex.EncodeToString(pubKey2))
			}
			return
		}

		if (ver_flags & VER_WITNESS)!=0 {
			witnessversion, witnessprogram = btc.IsWitnessProgram(pubKey2)
			if DBG_SCR {
				fmt.Println("============witnessversion:", witnessversion, "   witnessprogram:", hex.EncodeToString(witnessprogram))
			}
			if witnessprogram!=nil {
				hadWitness = true
				bt := new(bytes.Buffer)
				btc.WritePutLen(bt, uint32(len(pubKey2)))
				bt.Write(pubKey2)
				if !bytes.Equal(sigScr, bt.Bytes()) {
					if DBG_ERR {
						fmt.Println(hex.EncodeToString(sigScr))
						fmt.Println(hex.EncodeToString(bt.Bytes()))
						fmt.Println("SCRIPT_ERR_WITNESS_MALLEATED_P2SH")
					}
					return
				}
				if !VerifyWitnessProgram(&witness, amount, tx, i, witnessversion, witnessprogram, ver_flags) {
					if DBG_ERR {
						fmt.Println("VerifyWitnessProgram failed B")
					}
					return false
				}
				// Bypass the cleanstack check at the end. The actual stack is obviously not clean
				// for witness programs.
				stack.resize(1);
			}
		}
	}

	if (ver_flags & VER_CLEANSTACK) != 0 {
		if (ver_flags & VER_P2SH) == 0 {
			panic("VER_CLEANSTACK without VER_P2SH")
		}
		if DBG_SCR {
			fmt.Println("stack size", stack.size())
		}
		if stack.size()!=1 {
			if DBG_ERR {
				fmt.Println("Stack not clean")
			}
			return
		}
	}

	if (ver_flags&VER_WITNESS)!=0 {
		// We can't check for correct unexpected witness data if P2SH was off, so require
		// that WITNESS implies P2SH. Otherwise, going from WITNESS->P2SH+WITNESS would be
		// possible, which is not a softfork.
		if (ver_flags&VER_P2SH) == 0 {
			panic("VER_WITNESS must be used with P2SH")
		}
		if !hadWitness && !witness.IsNull() {
			if DBG_ERR {
				fmt.Println("SCRIPT_ERR_WITNESS_UNEXPECTED", len(tx.SegWit))
			}
			return
		}
	}

	result = true
	return true
}

func b2i(b bool) int64 {
	if b {
		return 1
	} else {
		return 0
	}
}

func evalScript(p []byte, amount uint64, stack *scrStack, tx *btc.Tx, inp int, ver_flags uint32, sigversion int) bool {
	if DBG_SCR {
		fmt.Println("evalScript len", len(p), "amount", amount, "inp", inp, "flagz", ver_flags, "sigver", sigversion)
		stack.print()
	}


	if len(p) > MAX_SCRIPT_SIZE {
		if DBG_ERR {
			fmt.Println("script too long", len(p))
		}
		return false
	}

	defer func() {
		if r := recover(); r != nil {
			if DBG_ERR {
				err, ok := r.(error)
				if !ok {
					err = fmt.Errorf("pkg: %v", r)
				}
				fmt.Println("evalScript panic:", err.Error())
				fmt.Println(string(debug.Stack()))
			}
		}
	}()

	var exestack scrStack
	var altstack scrStack
	sta, idx, opcnt := 0, 0, 0
	checkMinVals := (ver_flags&VER_MINDATA)!=0
	for idx < len(p) {
		inexec := exestack.nofalse()

		// Read instruction
		opcode, pushval, n, e := btc.GetOpcode(p[idx:])
		if e!=nil {
			//fmt.Println(e.Error())
			//fmt.Println("A", idx, hex.EncodeToString(p))
			return false
		}
		idx+= n

		if DBG_SCR {
			fmt.Printf("\nExecuting opcode 0x%02x  n=%d  inexec:%t  push:%s..\n",
				opcode, n, inexec, hex.EncodeToString(pushval))
			stack.print()
		}

		if pushval!=nil && len(pushval) > btc.MAX_SCRIPT_ELEMENT_SIZE {
			if DBG_ERR {
				fmt.Println("pushval too long", len(pushval))
			}
			return false
		}

		if opcode > 0x60 {
			opcnt++
			if opcnt > 201 {
				if DBG_ERR {
					fmt.Println("evalScript: too many opcodes A")
				}
				return false
			}
		}

		if opcode == 0x7e/*OP_CAT*/ ||
			opcode == 0x7f/*OP_SUBSTR*/ ||
			opcode == 0x80/*OP_LEFT*/ ||
			opcode == 0x81/*OP_RIGHT*/ ||
			opcode == 0x83/*OP_INVERT*/ ||
			opcode == 0x84/*OP_AND*/ ||
			opcode == 0x85/*OP_OR*/ ||
			opcode == 0x86/*OP_XOR*/ ||
			opcode == 0x8d/*OP_2MUL*/ ||
			opcode == 0x8e/*OP_2DIV*/ ||
			opcode == 0x95/*OP_MUL*/ ||
			opcode == 0x96/*OP_DIV*/ ||
			opcode == 0x97/*OP_MOD*/ ||
			opcode == 0x98/*OP_LSHIFT*/ ||
			opcode == 0x99/*OP_RSHIFT*/ {
			if DBG_ERR {
				fmt.Println("Unsupported opcode", opcode)
			}
			return false
		}

		if opcode == 0xab/*OP_CODESEPARATOR*/ && sigversion == SIGVERSION_BASE && (ver_flags&VER_CONST_SCRIPTCODE) != 0 {
			if DBG_ERR {
				fmt.Println("evalScript: SCRIPT_ERR_OP_CODESEPARATOR")
			}
			return false
		}

		if inexec && 0<=opcode && opcode<=btc.OP_PUSHDATA4 {
			if checkMinVals && !checkMinimalPush(pushval, opcode) {
				if DBG_ERR {
					fmt.Println("Push value not in a minimal format", hex.EncodeToString(pushval))
				}
				return false
			}
			stack.push(pushval)
			if DBG_SCR {
				fmt.Println("pushed", len(pushval), "bytes")
			}
		} else if inexec || (0x63/*OP_IF*/ <= opcode && opcode <= 0x68/*OP_ENDIF*/) {
			switch {
				case opcode==0x4f: // OP_1NEGATE
					stack.pushInt(-1)

				case opcode>=0x51 && opcode<=0x60: // OP_1-OP_16
					stack.pushInt(int64(opcode-0x50))

				case opcode==0x61: // OP_NOP
					// Do nothing

				/* - not handled
					OP_VER = 0x62
				*/

				case opcode==0x63 || opcode==0x64: //OP_IF || OP_NOTIF
					// <expression> if [statements] [else [statements]] endif
					val := false
					if inexec {
						if (stack.size() < 1) {
							if DBG_ERR {
								fmt.Println("Stack too short for", opcode)
							}
							return false
						}
						vch := stack.pop()
						if sigversion==SIGVERSION_WITNESS_V0 && (ver_flags&VER_MINIMALIF)!=0 {
							if len(vch)>1 {
								if DBG_ERR {
									fmt.Println("SCRIPT_ERR_MINIMALIF-1")
								}
								return false
							}
							if len(vch)==1 && vch[0]!=1 {
								if DBG_ERR {
									fmt.Println("SCRIPT_ERR_MINIMALIF-2")
								}
								return false
							}
						}
						val = bts2bool(vch)
						if opcode == 0x64/*OP_NOTIF*/ {
							val = !val
						}
					}
					if DBG_SCR {
						fmt.Println(inexec, "if pushing", val, "...")
					}
					exestack.pushBool(val)

				/* - not handled
				    OP_VERIF = 0x65,
				    OP_VERNOTIF = 0x66,
				*/
				case opcode==0x67: //OP_ELSE
					if exestack.size()==0 {
						if DBG_ERR {
							fmt.Println("exestack empty in OP_ELSE")
						}
					}
					exestack.pushBool(!exestack.popBool())

				case opcode==0x68: //OP_ENDIF
					if exestack.size()==0 {
						if DBG_ERR {
							fmt.Println("exestack empty in OP_ENDIF")
						}
					}
					exestack.pop()

				case opcode==0x69: //OP_VERIFY
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					if !stack.topBool(-1) {
						return false
					}
					stack.pop()

				case opcode==0x6b: //OP_TOALTSTACK
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					altstack.push(stack.pop())

				case opcode==0x6c: //OP_FROMALTSTACK
					if altstack.size()<1 {
						if DBG_ERR {
							fmt.Println("AltStack too short for opcode", opcode)
						}
						return false
					}
					stack.push(altstack.pop())

				case opcode==0x6d: //OP_2DROP
					if stack.size()<2 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					stack.pop()
					stack.pop()

				case opcode==0x6e: //OP_2DUP
					if stack.size()<2 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					x1 := stack.top(-1)
					x2 := stack.top(-2)
					stack.push(x2)
					stack.push(x1)

				case opcode==0x6f: //OP_3DUP
					if stack.size()<3 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					x1 := stack.top(-3)
					x2 := stack.top(-2)
					x3 := stack.top(-1)
					stack.push(x1)
					stack.push(x2)
					stack.push(x3)

				case opcode==0x70: //OP_2OVER
					if stack.size()<4 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					x1 := stack.top(-4)
					x2 := stack.top(-3)
					stack.push(x1)
					stack.push(x2)

				case opcode==0x71: //OP_2ROT
					// (x1 x2 x3 x4 x5 x6 -- x3 x4 x5 x6 x1 x2)
					if stack.size()<6 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					x6 := stack.pop()
					x5 := stack.pop()
					x4 := stack.pop()
					x3 := stack.pop()
					x2 := stack.pop()
					x1 := stack.pop()
					stack.push(x3)
					stack.push(x4)
					stack.push(x5)
					stack.push(x6)
					stack.push(x1)
					stack.push(x2)

				case opcode==0x72: //OP_2SWAP
					// (x1 x2 x3 x4 -- x3 x4 x1 x2)
					if stack.size()<4 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					x4 := stack.pop()
					x3 := stack.pop()
					x2 := stack.pop()
					x1 := stack.pop()
					stack.push(x3)
					stack.push(x4)
					stack.push(x1)
					stack.push(x2)

				case opcode==0x73: //OP_IFDUP
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					if stack.topBool(-1) {
						stack.push(stack.top(-1))
					}

				case opcode==0x74: //OP_DEPTH
					stack.pushInt(int64(stack.size()))

				case opcode==0x75: //OP_DROP
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					stack.pop()

				case opcode==0x76: //OP_DUP
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					el := stack.pop()
					stack.push(el)
					stack.push(el)

				case opcode==0x77: //OP_NIP
					if stack.size()<2 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					x := stack.pop()
					stack.pop()
					stack.push(x)

				case opcode==0x78: //OP_OVER
					if stack.size()<2 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					stack.push(stack.top(-2))

				case opcode==0x79 || opcode==0x7a: //OP_PICK || OP_ROLL
					if stack.size()<2 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					n := stack.popInt(checkMinVals)
					if n < 0 || n >= int64(stack.size()) {
						if DBG_ERR {
							fmt.Println("Wrong n for opcode", opcode)
						}
						return false
					}
					if opcode==0x79/*OP_PICK*/ {
						stack.push(stack.top(int(-1-n)))
					} else if n > 0 {
						tmp := make([][]byte, n)
						for i := range tmp {
							tmp[i] = stack.pop()
						}
						xn := stack.pop()
						for i := len(tmp)-1; i>=0; i-- {
							stack.push(tmp[i])
						}
						stack.push(xn)
					}

				case opcode==0x7b: //OP_ROT
					if stack.size()<3 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					x3 := stack.pop()
					x2 := stack.pop()
					x1 := stack.pop()
					stack.push(x2)
					stack.push(x3)
					stack.push(x1)

				case opcode==0x7c: //OP_SWAP
					if stack.size()<2 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					x1 := stack.pop()
					x2 := stack.pop()
					stack.push(x1)
					stack.push(x2)

				case opcode==0x7d: //OP_TUCK
					if stack.size()<2 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					x1 := stack.pop()
					x2 := stack.pop()
					stack.push(x1)
					stack.push(x2)
					stack.push(x1)

				case opcode==0x82: //OP_SIZE
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					stack.pushInt(int64(len(stack.top(-1))))

				case opcode==0x87 || opcode==0x88: //OP_EQUAL || OP_EQUALVERIFY
					if stack.size()<2 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					a := stack.pop()
					b := stack.pop()
					if opcode==0x88 { //OP_EQUALVERIFY
						if !bytes.Equal(a, b) {
							return false
						}
					} else {
						stack.pushBool(bytes.Equal(a, b))
					}

				/* - not handled
					OP_RESERVED1 = 0x89,
					OP_RESERVED2 = 0x8a,
				*/

				case opcode==0x8b: //OP_1ADD
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					stack.pushInt(stack.popInt(checkMinVals)+1)

				case opcode==0x8c: //OP_1SUB
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					stack.pushInt(stack.popInt(checkMinVals)-1)

				case opcode==0x8f: //OP_NEGATE
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					stack.pushInt(-stack.popInt(checkMinVals))

				case opcode==0x90: //OP_ABS
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					a := stack.popInt(checkMinVals)
					if a<0 {
						stack.pushInt(-a)
					} else {
						stack.pushInt(a)
					}

				case opcode==0x91: //OP_NOT
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					stack.pushBool(stack.popInt(checkMinVals)==0)

				case opcode==0x92: //OP_0NOTEQUAL
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					d := stack.pop()
					if checkMinVals && len(d)>1 {
						if DBG_ERR {
							fmt.Println("Not minimal bool value", hex.EncodeToString(d))
						}
						return false
					}
					stack.pushBool(bts2bool(d))

				case opcode==0x93 || //OP_ADD
					opcode==0x94 || //OP_SUB
					opcode==0x9a || //OP_BOOLAND
					opcode==0x9b || //OP_BOOLOR
					opcode==0x9c || opcode==0x9d || //OP_NUMEQUAL || OP_NUMEQUALVERIFY
					opcode==0x9e || //OP_NUMNOTEQUAL
					opcode==0x9f || //OP_LESSTHAN
					opcode==0xa0 || //OP_GREATERTHAN
					opcode==0xa1 || //OP_LESSTHANOREQUAL
					opcode==0xa2 || //OP_GREATERTHANOREQUAL
					opcode==0xa3 || //OP_MIN
					opcode==0xa4: //OP_MAX
					if stack.size()<2 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					bn2 := stack.popInt(checkMinVals)
					bn1 := stack.popInt(checkMinVals)
					var bn int64
					switch opcode {
						case 0x93: bn = bn1 + bn2 // OP_ADD
						case 0x94: bn = bn1 - bn2 // OP_SUB
						case 0x9a: bn = b2i(bn1 != 0 && bn2 != 0) // OP_BOOLAND
						case 0x9b: bn = b2i(bn1 != 0 || bn2 != 0) // OP_BOOLOR
						case 0x9c: bn = b2i(bn1 == bn2) // OP_NUMEQUAL
						case 0x9d: bn = b2i(bn1 == bn2) // OP_NUMEQUALVERIFY
						case 0x9e: bn = b2i(bn1 != bn2) // OP_NUMNOTEQUAL
						case 0x9f: bn = b2i(bn1 < bn2) // OP_LESSTHAN
						case 0xa0: bn = b2i(bn1 > bn2) // OP_GREATERTHAN
						case 0xa1: bn = b2i(bn1 <= bn2) // OP_LESSTHANOREQUAL
						case 0xa2: bn = b2i(bn1 >= bn2) // OP_GREATERTHANOREQUAL
						case 0xa3: // OP_MIN
							if bn1 < bn2 {
								bn = bn1
							} else {
								bn = bn2
							}
						case 0xa4: // OP_MAX
							if bn1 > bn2 {
								bn = bn1
							} else {
								bn = bn2
							}
						default: panic("invalid opcode")
					}
					if opcode == 0x9d { //OP_NUMEQUALVERIFY
						if bn==0 {
							return false
						}
					} else {
						stack.pushInt(bn)
					}

				case opcode==0xa5: //OP_WITHIN
					if stack.size()<3 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					bn3 := stack.popInt(checkMinVals)
					bn2 := stack.popInt(checkMinVals)
					bn1 := stack.popInt(checkMinVals)
					stack.pushBool(bn2 <= bn1 && bn1 < bn3)

				case opcode==0xa6: //OP_RIPEMD160
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					rim := ripemd160.New()
					rim.Write(stack.pop()[:])
					stack.push(rim.Sum(nil)[:])

				case opcode==0xa7: //OP_SHA1
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					sha := sha1.New()
					sha.Write(stack.pop()[:])
					stack.push(sha.Sum(nil)[:])

				case opcode==0xa8: //OP_SHA256
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					sha := sha256.New()
					sha.Write(stack.pop()[:])
					stack.push(sha.Sum(nil)[:])

				case opcode==0xa9: //OP_HASH160
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					rim160 := btc.Rimp160AfterSha256(stack.pop())
					stack.push(rim160[:])

				case opcode==0xaa: //OP_HASH256
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					h := btc.Sha2Sum(stack.pop())
					stack.push(h[:])

				case opcode==0xab: // OP_CODESEPARATOR
					sta = idx

				case opcode==0xac || opcode==0xad: // OP_CHECKSIG || OP_CHECKSIGVERIFY

					if stack.size()<2 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					var fSuccess bool
					vchSig := stack.top(-2)
					vchPubKey := stack.top(-1)

					scriptCode := p[sta:]

					// Drop the signature in pre-segwit scripts but not segwit scripts
					if sigversion == SIGVERSION_BASE {
						var found int
						scriptCode, found = delSig(scriptCode, vchSig)
						if found > 0 && (ver_flags&VER_CONST_SCRIPTCODE) != 0 {
							if DBG_ERR {
								fmt.Println("SCRIPT_ERR_SIG_FINDANDDELETE SIN")
							}
							return false
						}
					}

					// BIP-0066
					if !CheckSignatureEncoding(vchSig, ver_flags) || !CheckPubKeyEncoding(vchPubKey, ver_flags, sigversion) {
						if DBG_ERR {
							fmt.Println("Invalid Signature Encoding A")
						}
						return false
					}

					if len(vchSig) > 0 {
						var sh []byte
						if sigversion == SIGVERSION_WITNESS_V0 {
							if DBG_SCR {
								fmt.Println("getting WitnessSigHash for inp", inp, "and htype", int32(vchSig[len(vchSig)-1]))
							}
							sh = tx.WitnessSigHash(scriptCode, amount, inp, int32(vchSig[len(vchSig)-1]))
						} else {
							sh = tx.SignatureHash(scriptCode, inp, int32(vchSig[len(vchSig)-1]))
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
						fmt.Println("EcdsaVerify fail 1", tx.Hash.String())
					}

					if !fSuccess && (ver_flags&VER_NULLFAIL)!=0 && len(vchSig)>0 {
						if DBG_ERR {
							fmt.Println("SCRIPT_ERR_SIG_NULLFAIL-1")
						}
						return false
					}

					stack.pop()
					stack.pop()

					if DBG_SCR {
						fmt.Println("ver:", fSuccess)
					}
					if opcode==0xad {
						if !fSuccess { // OP_CHECKSIGVERIFY
							return false
						}
					} else { // OP_CHECKSIG
						stack.pushBool(fSuccess)
					}

				case opcode==0xae || opcode==0xaf: //OP_CHECKMULTISIG || OP_CHECKMULTISIGVERIFY
					//fmt.Println("OP_CHECKMULTISIG ...")
					//stack.print()
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("OP_CHECKMULTISIG: Stack too short A")
						}
						return false
					}
					i := 1
					keyscnt := stack.topInt(-i, checkMinVals)
					if keyscnt < 0 || keyscnt > 20 {
						if DBG_ERR {
							fmt.Println("OP_CHECKMULTISIG: Wrong number of keys")
						}
						return false
					}
					opcnt += int(keyscnt)
					if opcnt > 201 {
						if DBG_ERR {
							fmt.Println("evalScript: too many opcodes B")
						}
						return false
					}
					i++
					ikey := i
					// ikey2 is the position of last non-signature item in the stack. Top stack item = 1.
					// With SCRIPT_VERIFY_NULLFAIL, this is used for cleanup if operation fails.
					ikey2 := keyscnt + 2
					i += int(keyscnt)
					if stack.size()<i {
						if DBG_ERR {
							fmt.Println("OP_CHECKMULTISIG: Stack too short B")
						}
						return false
					}
					sigscnt := stack.topInt(-i, checkMinVals)
					if sigscnt < 0 || sigscnt > keyscnt {
						if DBG_ERR {
							fmt.Println("OP_CHECKMULTISIG: sigscnt error")
						}
						return false
					}
					i++
					isig := i
					i += int(sigscnt)
					if stack.size()<i {
						if DBG_ERR {
							fmt.Println("OP_CHECKMULTISIG: Stack too short C")
						}
						return false
					}

					xxx := p[sta:]
					// Drop the signature in pre-segwit scripts but not segwit scripts
					if sigversion == SIGVERSION_BASE {
						for k := 0; k < int(sigscnt); k++ {
							var found int
							xxx, found = delSig(xxx, stack.top(-isig-k))
							if found > 0 && (ver_flags&VER_CONST_SCRIPTCODE) != 0 {
								if DBG_ERR {
									fmt.Println("SCRIPT_ERR_SIG_FINDANDDELETE MUL")
								}
								return false
							}
						}
					}

					success := true
					for sigscnt > 0 {
						vchPubKey := stack.top(-ikey)
						vchSig := stack.top(-isig)

						// BIP-0066
						if !CheckSignatureEncoding(vchSig, ver_flags) || !CheckPubKeyEncoding(vchPubKey, ver_flags, sigversion) {
							if DBG_ERR {
								fmt.Println("Invalid Signature Encoding B")
							}
							return false
						}

						if len(vchSig) > 0 {
							var sh []byte

							if sigversion==SIGVERSION_WITNESS_V0 {
								sh = tx.WitnessSigHash(xxx, amount, inp, int32(vchSig[len(vchSig)-1]))
							} else {
								sh = tx.SignatureHash(xxx, inp, int32(vchSig[len(vchSig)-1]))
							}
							if btc.EcdsaVerify(vchPubKey, vchSig, sh) {
								isig++
								sigscnt--
							}
						}

						ikey++
						keyscnt--

						// If there are more signatures left than keys left,
						// then too many signatures have failed
						if sigscnt > keyscnt {
							success = false
							break
						}
					}

					// Clean up stack of actual arguments
					for i > 1 {
						i--

						if !success && (ver_flags&VER_NULLFAIL)!=0 && ikey2==0 && len(stack.top(-1))>0 {
							if DBG_ERR {
								fmt.Println("SCRIPT_ERR_SIG_NULLFAIL-2")
							}
							return false
						}
						if ikey2>0 {
							ikey2--
						}

						stack.pop()
					}

					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("OP_CHECKMULTISIG: Dummy element missing")
						}
						return false
					}
					if (ver_flags & VER_NULLDUMMY)!=0 && len(stack.top(-1))!=0 {
						if DBG_ERR {
							fmt.Println("OP_CHECKMULTISIG: NULLDUMMY verification failed")
						}
						return false
					}
					stack.pop()

					if opcode==0xaf {
						if !success { // OP_CHECKMULTISIGVERIFY
							return false
						}
					} else {
						stack.pushBool(success)
					}

				case opcode==0xb1: //OP_NOP2 or OP_CHECKLOCKTIMEVERIFY
					if (ver_flags&VER_CLTV) == 0 {
						if (ver_flags&VER_BLOCK_OPS)!=0 {
							return false
						}
						break // Just do NOP2
					}

					if DBG_SCR {
						fmt.Println("OP_CHECKLOCKTIMEVERIFY...")
					}

					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("OP_CHECKLOCKTIMEVERIFY: Stack too short")
						}
						return false
					}

					d := stack.top(-1)
					if len(d)>5 {
						if DBG_ERR {
							fmt.Println("OP_CHECKLOCKTIMEVERIFY: locktime field too long", len(d))
						}
						return false
					}

					if DBG_SCR {
						fmt.Println("val from stack", hex.EncodeToString(d))
					}

					locktime := bts2int_ext(d, 5, checkMinVals)
					if locktime < 0 {
						if DBG_ERR {
							fmt.Println("OP_CHECKLOCKTIMEVERIFY: negative locktime")
						}
						return false
					}

					if (! ((tx.Lock_time <  LOCKTIME_THRESHOLD && locktime <  LOCKTIME_THRESHOLD) ||
						(tx.Lock_time >= LOCKTIME_THRESHOLD && locktime >= LOCKTIME_THRESHOLD) )) {
						if DBG_ERR {
							fmt.Println("OP_CHECKLOCKTIMEVERIFY: broken lock value")
						}
						return false
					}

					if DBG_SCR {
						fmt.Println("locktime > int64(tx.Lock_time)", locktime, int64(tx.Lock_time))
						fmt.Println(" ... seq", len(tx.TxIn), inp, tx.TxIn[inp].Sequence)
					}

					// Actually compare the specified lock time with the transaction.
					if locktime > int64(tx.Lock_time) {
						if DBG_ERR {
							fmt.Println("OP_CHECKLOCKTIMEVERIFY: Locktime requirement not satisfied")
						}
						return false
					}

					if tx.TxIn[inp].Sequence == 0xffffffff  {
						if DBG_ERR {
							fmt.Println("OP_CHECKLOCKTIMEVERIFY: TxIn final")
						}
						return false
					}

					// OP_CHECKLOCKTIMEVERIFY passed successfully

				case opcode==0xb2: //OP_NOP3 or OP_CHECKSEQUENCEVERIFY
					if (ver_flags&VER_CSV) == 0 {
						if (ver_flags&VER_BLOCK_OPS)!=0 {
							return false
						}
						break // Just do NOP3
					}

					if DBG_SCR {
						fmt.Println("OP_CHECKSEQUENCEVERIFY...")
					}

					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("OP_CHECKSEQUENCEVERIFY: Stack too short")
						}
						return false
					}

					d := stack.top(-1)
					if len(d)>5 {
						if DBG_ERR {
							fmt.Println("OP_CHECKSEQUENCEVERIFY: sequence field too long", len(d))
						}
						return false
					}

					if DBG_SCR {
						fmt.Println("seq from stack", hex.EncodeToString(d))
					}

					sequence := bts2int_ext(d, 5, checkMinVals)
					if sequence < 0 {
						if DBG_ERR {
							fmt.Println("OP_CHECKSEQUENCEVERIFY: negative sequence")
						}
						return false
					}

					if (sequence & SEQUENCE_LOCKTIME_DISABLE_FLAG) != 0 {
						break
					}

					if !CheckSequence(tx, inp, sequence) {
						if DBG_ERR {
							fmt.Println("OP_CHECKSEQUENCEVERIFY: CheckSequence failed")
						}
						return false
					}

				case opcode==0xb0 || opcode>=0xb3 && opcode<=0xb9: //OP_NOP1 || OP_NOP4..OP_NOP10
					if (ver_flags&VER_BLOCK_OPS)!=0 {
						return false
					}
					// just do nothing

				default:
					if DBG_ERR {
						fmt.Printf("Unhandled opcode 0x%02x - a handler must be implemented\n", opcode)
						stack.print()
						fmt.Println("Rest of the script:", hex.EncodeToString(p[idx:]))
					}
					return false
			}
		}

		if DBG_SCR {
			fmt.Printf("Finished Executing opcode 0x%02x\n", opcode)
			stack.print()
		}
		if (stack.size() + altstack.size() > 1000) {
			if DBG_ERR {
				fmt.Println("Stack too big")
			}
			return false
		}
	}

	if DBG_SCR {
		fmt.Println("END OF SCRIPT")
		stack.print()
	}

	if exestack.size()>0 {
		if DBG_ERR {
			fmt.Println("Unfinished if..")
		}
		return false
	}

	return true
}


func delSig(where, sig []byte) (res []byte, cnt int) {
	// place the push opcode in front of the signature
	push_sig_scr := make([]byte, len(sig)+5)
	n := int(btc.PutVlen(push_sig_scr, len(sig)))
	copy(push_sig_scr[n:], sig)
	sig = push_sig_scr[:n+len(sig)]

	// set the cap to the maximum possible size, to speed up further append-s
	res = make([]byte, 0, len(where))

	var idx int
	for idx < len(where) {
		_, _, n, e := btc.GetOpcode(where[idx:])
		if e != nil {
			fmt.Println(e.Error())
			fmt.Println("B", idx, hex.EncodeToString(where))
			return
		}
		if !bytes.Equal(where[idx:idx+n], sig) {
			res = append(res, where[idx:idx+n]...)
		} else {
			cnt++
		}
		idx+= n
	}
	return
}


