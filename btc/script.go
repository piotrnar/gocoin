package btc

import (
	"os"
	"fmt"
	"bytes"
	"errors"
	"math/big"
	"encoding/hex"
	"encoding/binary"
	"crypto/sha256"
	"code.google.com/p/go.crypto/ripemd160"
)

const MAX_SCRIPT_ELEMENT_SIZE = 520


func VerifyTxScript(sigScr []byte, pkScr []byte, i int, tx *Tx) bool {
	if don(DBG_SCRIPT) {
		fmt.Println("VerifyTxScript", tx.Hash.String(), i+1, "/", len(tx.TxIn))
		fmt.Println("sigScript:", hex.EncodeToString(sigScr[:]))
	}

	var st scrStack
	if !evalScript(sigScr[:], &st, tx, i) {
		fmt.Println("VerifyTxScript", tx.Hash.String(), i+1, "/", len(tx.TxIn))
		fmt.Println("sigScript failed :", hex.EncodeToString(sigScr[:]))
		fmt.Println("pkScript:", hex.EncodeToString(pkScr[:]))
		return false
	}
	if don(DBG_SCRIPT) {
		fmt.Println("\nsigScr verified OK")
		st.print()
		fmt.Println("\npkScript:", hex.EncodeToString(pkScr))
	}

	if !evalScript(pkScr[:], &st, tx, i) {
		if don(DBG_SCRIPT) {
			fmt.Println("* pkScript failed :", hex.EncodeToString(pkScr[:]))
			fmt.Println("* VerifyTxScript", tx.Hash.String(), i+1, "/", len(tx.TxIn))
			fmt.Println("* sigScript:", hex.EncodeToString(sigScr[:]))
		}
		return false
	}

	if st.size()==0 {
		if don(DBG_SCRIPT) {
			fmt.Println("* stack empty after executing scripts:", hex.EncodeToString(pkScr[:]))
		}
		return false
	}

	if !st.popBool() {
		if don(DBG_SCRIPT) {
			fmt.Println("* FALSE on stack after executing scripts:", hex.EncodeToString(pkScr[:]))
		}
		return false
	}

	return true
}


func b2i(b bool) *big.Int {
	if b {
		return big.NewInt(1)
	} else {
		return new(big.Int)
	}
}

func evalScript(p []byte, stack *scrStack, tx *Tx, inp int) bool {
	var vfExec scrStack
	var altstack scrStack
	sta, idx, opcnt := 0, 0, 0
	for idx < len(p) {
		fExec := vfExec.empties()==0

		// Read instruction
		opcode, vchPushValue, n, e := getOpcode(p[idx:])
		if e!=nil {
			println(e.Error())
			return false
		}
		idx+= n

		if don(DBG_SCRIPT) {
			fmt.Printf("\nExecuting opcode 0x%02x..\n", opcode)
			stack.print()
		}

		if vchPushValue!=nil && len(vchPushValue) > MAX_SCRIPT_ELEMENT_SIZE {
			println("vchPushValue too long", len(vchPushValue))
			return false
		}

		if opcode > 0x60 {
			opcnt++
			if opcnt > 201 {
				println("evalScript: too many opcodes A")
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
			println("Unsupported opcode")
			return false
		}

		if fExec && 0<=opcode && opcode<=0x4e/*OP_PUSHDATA4*/ {
			stack.push(vchPushValue[:])
		} else if fExec || (0x63/*OP_IF*/ <= opcode && opcode <= 0x68/*OP_ENDIF*/) {
			switch {
				case opcode==0x4f: // OP_1NEGATE
					stack.pushInt(big.NewInt(-1))

				case opcode>=0x51 && opcode<=0x60: // OP_1-OP_16
					stack.pushInt(big.NewInt(int64(opcode-0x50)))

				case opcode==0x61: // OP_NOP
					// Do nothing

				case opcode==0x62: // OP_VER
					// I think, do nothing...

				case opcode==0x63 || opcode==0x64: //OP_IF || OP_NOTIF
					// <expression> if [statements] [else [statements]] endif
					fValue := false
					if fExec {
						if (stack.size() < 1) {
							println("Stack too short for", opcode)
							return false
						}
						if opcode == 0x63/*OP_IF*/ {
							fValue = stack.popBool()
						} else {
							fValue = !stack.popBool()
						}
					}
					vfExec.pushBool(fValue)

				case opcode==0x67: //OP_ELSE
					if vfExec.size()==0 {
						println("vfExec empty in OP_ELSE")
					}
					vfExec.pushBool(!vfExec.popBool())

				case opcode==0x68: //OP_ENDIF
					if vfExec.size()==0 {
						println("vfExec empty in OP_ENDIF")
					}
					vfExec.pop()

				case opcode==0x69: //OP_VERIFY
					if stack.size()<1 {
						println("Stack too short for opcode", opcode)
						return false
					}
					if !stack.topBool(-1) {
						return false
					}
					stack.pop()

				case opcode==0x6b: //OP_TOALTSTACK
					if stack.size()<1 {
						println("Stack too short for opcode", opcode)
						return false
					}
					altstack.push(stack.pop())

				case opcode==0x6c: //OP_FROMALTSTACK
					if altstack.size()<1 {
						println("AltStack too short for opcode", opcode)
						return false
					}
					stack.push(altstack.pop())

				case opcode==0x6d: //OP_2DROP
					if stack.size()<2 {
						println("Stack too short for opcode", opcode)
						return false
					}
					stack.pop()
					stack.pop()

				case opcode==0x6e: //OP_2DUP
					if stack.size()<2 {
						println("Stack too short for opcode", opcode)
						return false
					}
					x1 := stack.top(-1)
					x2 := stack.top(-2)
					stack.push(x2)
					stack.push(x1)

				case opcode==0x6f: //OP_3DUP
					if stack.size()<3 {
						println("Stack too short for opcode", opcode)
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
						println("Stack too short for opcode", opcode)
						return false
					}
					x1 := stack.top(-4)
					x2 := stack.top(-3)
					stack.push(x1)
					stack.push(x2)

				case opcode==0x73: //OP_IFDUP
					if stack.size()<1 {
						println("Stack too short for opcode", opcode)
						return false
					}
					if stack.topBool(-1) {
						stack.push(stack.top(-1))
					}

				case opcode==0x74: //OP_DEPTH
					stack.pushInt(big.NewInt(int64(stack.size())))

				case opcode==0x75: //OP_DROP
					if stack.size()<1 {
						println("Stack too short for opcode", opcode)
						return false
					}
					stack.pop()

				case opcode==0x76: //OP_DUP
					if stack.size()<1 {
						println("Stack too short for opcode", opcode)
						return false
					}
					el := stack.pop()
					stack.push(el)
					stack.push(el)

				case opcode==0x77: //OP_NIP
					if stack.size()<2 {
						println("Stack too short for opcode", opcode)
						return false
					}
					x := stack.pop()
					stack.pop()
					stack.push(x)

				case opcode==0x78: //OP_OVER
					if stack.size()<2 {
						println("Stack too short for opcode", opcode)
						return false
					}
					stack.push(stack.top(-2))

				case opcode==0x79 || opcode==0x7a: //OP_PICK || OP_ROLL
					if stack.size()<2 {
						println("Stack too short for opcode", opcode)
						return false
					}
					n := stack.popInt()
					//if n < 0 || n >= int64(stack.size()) {
					if n.Sign()<0 || n.Cmp(big.NewInt(int64(stack.size()))) >= 0 {
						println("Wrong n for opcode", opcode)
						return false
					}
					ni := n.Int64()
					if opcode==0x79/*OP_PICK*/ {
						stack.push(stack.top(int(-1-ni)))
					} else if ni > 0 {
						tmp := make([][]byte, ni)
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
						println("Stack too short for opcode", opcode)
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
						println("Stack too short for opcode", opcode)
						return false
					}
					x1 := stack.pop()
					x2 := stack.pop()
					stack.push(x1)
					stack.push(x2)

				case opcode==0x7d: //OP_TUCK
					if stack.size()<2 {
						println("Stack too short for opcode", opcode)
						return false
					}
					x1 := stack.pop()
					x2 := stack.pop()
					stack.push(x1)
					stack.push(x2)
					stack.push(x1)

				case opcode==0x82: //OP_SIZE
					if stack.size()<1 {
						println("Stack too short for opcode", opcode)
						return false
					}
					stack.pushInt(big.NewInt(int64(len(stack.top(-1)))))

				case opcode==0x87 || opcode==0x88: //OP_EQUAL || OP_EQUALVERIFY
					if stack.size()<2 {
						println("Stack too short for opcode", opcode)
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

				case opcode==0x8c: //OP_1SUB
					if stack.size()<1 {
						println("Stack too short for opcode", opcode)
						return false
					}
					n := stack.popInt()
					stack.pushInt(n.Sub(n, big.NewInt(1)))

				case opcode==0x8f: //OP_NEGATE
					if stack.size()<1 {
						println("Stack too short for opcode", opcode)
						return false
					}
					a := stack.popInt()
					stack.pushInt(a.Neg(a))

				case opcode==0x90: //OP_ABS
					if stack.size()<1 {
						println("Stack too short for opcode", opcode)
						return false
					}
					a := stack.popInt()
					if a.Sign()<0 {
						stack.pushInt(a.Neg(a))
					} else {
						stack.pushInt(a)
					}

				case opcode==0x91: //OP_NOT
					if stack.size()<1 {
						println("Stack too short for opcode", opcode)
						return false
					}
					stack.pushBool(!stack.popBool())

				case opcode==0x92: //OP_0NOTEQUAL
					if stack.size()<1 {
						println("Stack too short for opcode", opcode)
						return false
					}
					stack.pushBool(stack.popBool())

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
						println("Stack too short for opcode", opcode)
						return false
					}
					bn2 := stack.popInt()
					bn1 := stack.popInt()
					var bn *big.Int
					switch opcode {
						case 0x93: bn = new(big.Int).Add(bn1, bn2) // OP_ADD
						case 0x94: bn = new(big.Int).Sub(bn1, bn2) // OP_SUB
						case 0x9a: bn = b2i(bn1.Sign()!=0 && bn2.Sign()!=0) // OP_BOOLAND
						case 0x9b: bn = b2i(bn1.Sign()!=0 || bn2.Sign()!=0) // OP_BOOLOR
						case 0x9c: bn = b2i(bn1.Cmp(bn2)==0) // OP_NUMEQUAL
						case 0x9d: bn = b2i(bn1.Cmp(bn2)==0) // OP_NUMEQUALVERIFY
						case 0x9e: bn = b2i(bn1.Cmp(bn2)!=0) // OP_NUMNOTEQUAL
						case 0x9f: bn = b2i(bn1.Cmp(bn2)<0) // OP_LESSTHAN
						case 0xa0: bn = b2i(bn1.Cmp(bn2)>0) // OP_GREATERTHAN
						case 0xa1: bn = b2i(bn1.Cmp(bn2)<=0) // OP_LESSTHANOREQUAL
						case 0xa2: bn = b2i(bn1.Cmp(bn2)>=0) // OP_GREATERTHANOREQUAL
						case 0xa3: // OP_MIN
							if bn1.Cmp(bn2)<0 {
								bn = bn1
							} else {
								bn = bn2
							}
						case 0xa4: // OP_MAX
							if bn1.Cmp(bn2)>0 {
								bn = bn1
							} else {
								bn = bn2
							}
						default: panic("invalid opcode")
					}
					if opcode == 0x9d { //OP_NUMEQUALVERIFY
						if bn.Sign()==0 {
							return false
						}
					} else {
						stack.pushInt(bn)
					}

				case opcode==0xa5: //OP_WITHIN
					if stack.size()<3 {
						println("Stack too short for opcode", opcode)
						return false
					}
					bn3 := stack.popInt()
					bn2 := stack.popInt()
					bn1 := stack.popInt()
					stack.pushBool(bn2.Cmp(bn1)<=0 && bn1.Cmp(bn3)<0)

				case opcode==0xa6: //OP_RIPEMD160
					if stack.size()<1 {
						println("Stack too short for opcode", opcode)
						return false
					}
					rim := ripemd160.New()
					rim.Write(stack.pop()[:])
					stack.push(rim.Sum(nil)[:])

				case opcode==0xa8: //OP_SHA256
					if stack.size()<1 {
						println("Stack too short for opcode", opcode)
						return false
					}
					sha := sha256.New()
					sha.Write(stack.pop()[:])
					stack.push(sha.Sum(nil)[:])

				case opcode==0xa9: //OP_HASH160
					if stack.size()<1 {
						println("Stack too short for opcode", opcode)
						return false
					}
					rim160 := Rimp160AfterSha256(stack.pop())
					stack.push(rim160[:])

				case opcode==0xaa: //OP_HASH256
					if stack.size()<1 {
						println("Stack too short for opcode", opcode)
						return false
					}
					h := Sha2Sum(stack.pop())
					stack.push(h[:])

				case opcode==0xab: // OP_CODESEPARATOR
					sta = idx

				case opcode==0xac || opcode==0xad: // OP_CHECKSIG || OP_CHECKSIGVERIFY
					if stack.size()<2 {
						println("Stack too short for opcode", opcode)
						return false
					}
					var ok bool
					pk := stack.pop()
					si := stack.pop()
					if len(si) > 9 {
						ok = EcdsaVerify(pk, si, tx.SignatureHash(p[sta:], inp, si[len(si)-1]))
						if !ok {
							println("EcdsaVerify fail 1")
						}
					}
					if don(DBG_SCRIPT) {
						println("ver:", ok)
					}
					if opcode==0xad {
						if !ok { // OP_CHECKSIGVERIFY
							return false
						}
					} else { // OP_CHECKSIG
						stack.pushBool(ok)
					}

				case opcode==0xae || opcode==0xaf: //OP_CHECKMULTISIG || OP_CHECKMULTISIGVERIFY
					//println("OP_CHECKMULTISIG ...")
					//stack.print()
					if stack.size()<1 {
						println("OP_CHECKMULTISIG: Stack too short A")
						return false
					}
					i := 1
					bnkc := stack.topInt(-i)
					if bnkc.Sign()<0 || bnkc.Cmp(big.NewInt(20)) > 0 {
						println("OP_CHECKMULTISIG: Wrong number of keys")
						return false
					}
					keyscnt := bnkc.Int64()
					opcnt += int(keyscnt)
					if opcnt > 201 {
						println("evalScript: too many opcodes B")
						return false
					}
					i++
					ikey := i
					i += int(keyscnt)
					if stack.size()<i {
						println("OP_CHECKMULTISIG: Stack too short B")
						return false
					}
					bnsc := stack.topInt(-i)
					if bnsc.Sign()<0 || bnsc.Cmp(bnkc)>0 {
						println("OP_CHECKMULTISIG: sigscnt error")
						return false
					}
					sigscnt := bnsc.Int64()
					i++
					isig := i
					i += int(sigscnt)
					if stack.size()<i {
						println("OP_CHECKMULTISIG: Stack too short C")
						return false
					}
					success := true
					for sigscnt > 0 {
						pk := stack.top(-ikey)
						si := stack.top(-isig)
						if len(si)>9 && ((len(pk)==65 && pk[0]==4) || (len(pk)==33 && (pk[0]|1)==3)) {
							if EcdsaVerify(pk, si, tx.SignatureHash(p[sta:], inp, si[len(si)-1])) {
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
					for i > 0 {
						i--
						stack.pop()
					}
					if opcode==0xaf {
						if !success { // OP_CHECKMULTISIGVERIFY
							return false
						}
					} else {
						stack.pushBool(success)
					}

				case opcode>=0xb0 && opcode<=0xb9: //OP_NOP
					// just do nothing

				case opcode>=0xba: // inivalid opcode = invaid script
					println("fail tx because it contains invalid opcode", opcode)
					return false

				default:
					fmt.Printf("Unhandled opcode 0x%02x - a handler must be implemented\n", opcode)
					stack.print()
					fmt.Println("Rest of the script:", hex.EncodeToString(p[idx:]))
					os.Exit(0)
			}
		}

		if don(DBG_SCRIPT) {
			fmt.Printf("Finished Executing opcode 0x%02x\n", opcode)
			stack.print()
		}
		if (stack.size() + altstack.size() > 1000) {
			println("Stack too big")
			return false
		}
	}

	if don(DBG_SCRIPT) {
		fmt.Println("END OF SCRIPT")
		stack.print()
	}

	if vfExec.size()>0 {
		println("Unfinished if..")
		return false
	}

	return true
}

func getOpcode(b []byte) (opcode int, pvchRet []byte, pc int, e error) {
	// Read instruction
	if pc+1 > len(b) {
		e = errors.New("getOpcode error 1")
		return
	}
	opcode = int(b[pc])
	pc++

	if opcode <= 0x4e/*OP_PUSHDATA4*/ {
		nSize := 0
		if opcode < 0x4c/*OP_PUSHDATA1*/ {
			nSize = opcode
		}
		if opcode == 0x4c/*OP_PUSHDATA1*/ {
			if pc+1 > len(b) {
				e = errors.New("getOpcode error 2")
				return
			}
			nSize = int(b[pc])
			pc++
		} else if opcode == 0x4d/*OP_PUSHDATA2*/ {
			if pc+2 > len(b) {
				e = errors.New("getOpcode error 3")
				return
			}
			nSize = int(binary.LittleEndian.Uint16(b[pc:pc+2]))
			pc += 2
		} else if opcode == 0x4d/*OP_PUSHDATA2*/ {
			if pc+4 > len(b) {
				e = errors.New("getOpcode error 4")
				return
			}
			nSize = int(binary.LittleEndian.Uint16(b[pc:pc+4]))
			pc += 4
		}
		pvchRet = b[pc:pc+nSize]
		pc += nSize
	}

	return
}
