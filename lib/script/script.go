package script

import (
	"fmt"
	"bytes"
	"crypto/sha1"
	"encoding/hex"
	"crypto/sha256"
	"encoding/binary"
	"github.com/piotrnar/gocoin/lib/btc"
	"code.google.com/p/go.crypto/ripemd160"
	"runtime/debug"
)

var (
	DBG_SCR = false
	DBG_ERR = true
)

const (
	VER_P2SH = 1<<0
	VER_DERSIG = 1<<2
)


func VerifyTxScript(sigScr []byte, pkScr []byte, i int, tx *btc.Tx, ver_flags uint32) bool {
	if DBG_SCR {
		fmt.Println("VerifyTxScript", tx.Hash.String(), i+1, "/", len(tx.TxIn))
		fmt.Println("sigScript:", hex.EncodeToString(sigScr[:]))
		fmt.Println("_pkScript:", hex.EncodeToString(pkScr))
	}

	var st, stP2SH scrStack
	if !evalScript(sigScr, &st, tx, i, ver_flags) {
		if DBG_ERR {
			if tx != nil {
				fmt.Println("VerifyTxScript", tx.Hash.String(), i+1, "/", len(tx.TxIn))
			}
			fmt.Println("sigScript failed :", hex.EncodeToString(sigScr[:]))
			fmt.Println("pkScript:", hex.EncodeToString(pkScr[:]))
		}
		return false
	}
	if DBG_SCR {
		fmt.Println("\nsigScr verified OK")
		//st.print()
		fmt.Println()
	}

	// copy the stack content to stP2SH
	if st.size()>0 {
		idx := -st.size()
		for i:=0; i<st.size(); i++ {
			x := st.top(idx)
			stP2SH.push(x)
			idx++
		}
	}

	if !evalScript(pkScr, &st, tx, i, ver_flags) {
		if DBG_SCR {
			fmt.Println("* pkScript failed :", hex.EncodeToString(pkScr[:]))
			fmt.Println("* VerifyTxScript", tx.Hash.String(), i+1, "/", len(tx.TxIn))
			fmt.Println("* sigScript:", hex.EncodeToString(sigScr[:]))
		}
		return false
	}

	if st.size()==0 {
		if DBG_SCR {
			fmt.Println("* stack empty after executing scripts:", hex.EncodeToString(pkScr[:]))
		}
		return false
	}

	if !st.popBool() {
		if DBG_SCR {
			fmt.Println("* FALSE on stack after executing scripts:", hex.EncodeToString(pkScr[:]))
		}
		return false
	}

	// Additional validation for spend-to-script-hash transactions:
	if (ver_flags&VER_P2SH)!=0 && btc.IsPayToScript(pkScr) {
		if DBG_SCR {
			fmt.Println()
			fmt.Println()
			fmt.Println(" ******************* Looks like P2SH script ********************* ")
			stP2SH.print()
		}

		if DBG_SCR {
			fmt.Println("sigScr len", len(sigScr), hex.EncodeToString(sigScr))
		}
		if !IsPushOnly(sigScr) {
			if DBG_ERR {
				fmt.Println("P2SH is not push only")
			}
			return false
		}

		pubKey2 := stP2SH.pop()
		if DBG_SCR {
			fmt.Println("pubKey2:", hex.EncodeToString(pubKey2))
		}

		if !evalScript(pubKey2, &stP2SH, tx, i, ver_flags) {
			if DBG_ERR {
				fmt.Println("P2SH extra verification failed")
			}
			return false
		}

		if stP2SH.size()==0 {
			if DBG_SCR {
				fmt.Println("* P2SH stack empty after executing script:", hex.EncodeToString(pubKey2))
			}
			return false
		}

		if !stP2SH.popBool() {
			if DBG_SCR {
				fmt.Println("* FALSE on stack after executing P2SH script:", hex.EncodeToString(pubKey2))
			}
			return false
		}
	}

	return true
}

func b2i(b bool) int64 {
	if b {
		return 1
	} else {
		return 0
	}
}

func evalScript(p []byte, stack *scrStack, tx *btc.Tx, inp int, ver_flags uint32) bool {
	if DBG_SCR {
		fmt.Println("script len", len(p))
	}


	if len(p) > 10000 {
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

		if inexec && 0<=opcode && opcode<=btc.OP_PUSHDATA4 {
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
						if opcode == 0x63/*OP_IF*/ {
							val = stack.popBool()
						} else {
							val = !stack.popBool()
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
					n := stack.popInt()
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
					stack.pushInt(stack.popInt()+1)

				case opcode==0x8c: //OP_1SUB
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					stack.pushInt(stack.popInt()-1)

				case opcode==0x8f: //OP_NEGATE
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					stack.pushInt(-stack.popInt())

				case opcode==0x90: //OP_ABS
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					a := stack.popInt()
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
					stack.pushBool(stack.popInt()==0)

				case opcode==0x92: //OP_0NOTEQUAL
					if stack.size()<1 {
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
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
						if DBG_ERR {
							fmt.Println("Stack too short for opcode", opcode)
						}
						return false
					}
					bn2 := stack.popInt()
					bn1 := stack.popInt()
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
					bn3 := stack.popInt()
					bn2 := stack.popInt()
					bn1 := stack.popInt()
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
					var ok bool
					pk := stack.pop()
					si := stack.pop()

					// BIP-0066
					if !CheckSignatureEncoding(si, ver_flags) {
						if DBG_ERR {
							fmt.Println("Invalid Signature Encoding A")
						}
						return false
					}

					if len(si)>0 {
						sh := tx.SignatureHash(delSig(p[sta:], si), inp, int32(si[len(si)-1]))
						ok = btc.EcdsaVerify(pk, si, sh)
					}
					if !ok && DBG_ERR {
						if DBG_ERR {
							fmt.Println("EcdsaVerify fail 1")
						}
					}

					if DBG_SCR {
						fmt.Println("ver:", ok)
					}
					if opcode==0xad {
						if !ok { // OP_CHECKSIGVERIFY
							return false
						}
					} else { // OP_CHECKSIG
						stack.pushBool(ok)
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
					keyscnt := stack.topInt(-i)
					if keyscnt < 0 || keyscnt > 20 {
						fmt.Println("OP_CHECKMULTISIG: Wrong number of keys")
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
					i += int(keyscnt)
					if stack.size()<i {
						if DBG_ERR {
							fmt.Println("OP_CHECKMULTISIG: Stack too short B")
						}
						return false
					}
					sigscnt := stack.topInt(-i)
					if sigscnt < 0 || sigscnt > keyscnt {
						fmt.Println("OP_CHECKMULTISIG: sigscnt error")
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
					for k:=0; k<int(sigscnt); k++ {
						xxx = delSig(xxx, stack.top(-isig-k))
					}

					success := true
					for sigscnt > 0 {
						pk := stack.top(-ikey)
						si := stack.top(-isig)

						// BIP-0066
						if !CheckSignatureEncoding(si, ver_flags) {
							if DBG_ERR {
								fmt.Println("Invalid Signature Encoding B")
							}
							return false
						}

						if len(si) > 0 {
							sh := tx.SignatureHash(xxx, inp, int32(si[len(si)-1]))
							if btc.EcdsaVerify(pk, si, sh) {
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


func delSig(where, sig []byte) (res []byte) {
	// recover the standard length
	bb := new(bytes.Buffer)
	if len(sig) < btc.OP_PUSHDATA1 {
		bb.Write([]byte{byte(len(sig))})
	} else if len(sig) <= 0xff {
		bb.Write([]byte{btc.OP_PUSHDATA1})
		bb.Write([]byte{byte(len(sig))})
	} else if len(sig) <= 0xffff {
		bb.Write([]byte{btc.OP_PUSHDATA2})
		binary.Write(bb, binary.LittleEndian, uint16(len(sig)))
	} else {
		bb.Write([]byte{btc.OP_PUSHDATA4})
		binary.Write(bb, binary.LittleEndian, uint32(len(sig)))
	}
	bb.Write(sig)
	sig = bb.Bytes()
	var idx int
	for idx < len(where) {
		_, _, n, e := btc.GetOpcode(where[idx:])
		if e!=nil {
			fmt.Println(e.Error())
			fmt.Println("B", idx, hex.EncodeToString(where))
			return
		}
		if !bytes.Equal(where[idx:idx+n], sig) {
			res = append(res, where[idx:idx+n]...)
		}
		idx+= n
	}
	return
}


func IsPushOnly(scr []byte) bool {
	idx := 0
	for idx<len(scr) {
		op, _, n, e := btc.GetOpcode(scr[idx:])
		if e != nil {
			return false
		}
		if op > btc.OP_16 {
			return false
		}
		idx += n
	}
	return true
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
	if sig[2]!=0x02 {
		return false
	}

	// Zero-length integers are not allowed for R.
	if lenR == 0 {
		return false
	}

	// Negative numbers are not allowed for R.
	if (sig[4]&0x80)!=0 {
		return false
	}

	// Null bytes at the start of R are not allowed, unless R would
	// otherwise be interpreted as a negative number.
	if lenR>1 && sig[4]==0x00 && (sig[5]&0x80)==0 {
		return false
	}

	// Check whether the S element is an integer.
	if sig[lenR+4]!=0x02 {
		return false
	}

	// Zero-length integers are not allowed for S.
	if lenS==0 {
		return false
	}

	// Negative numbers are not allowed for S.
	if (sig[lenR+6]&0x80)!=0 {
		return false
	}

	// Null bytes at the start of S are not allowed, unless S would otherwise be
	// interpreted as a negative number.
	if lenS>1 && (sig[lenR+6]==0x00) && (sig[lenR+7]&0x80)==0 {
		return false
	}

	return true
}

// We only check for VER_DERSIG from BIP66. BIP62 has not been implemented
func CheckSignatureEncoding(sig []byte, flags uint32) bool {
	// Empty signature. Not strictly DER encoded, but allowed to provide a
	// compact way to provide an invalid signature for use with CHECK(MULTI)SIG
	if len(sig) == 0 {
		return true
	}
	if (flags&VER_DERSIG)!=0 && !IsValidSignatureEncoding(sig) {
		return false
	}
	return true
}


