package btc

import (
	"fmt"
	"os"
)

func VerifyTxScript(sig []byte, in *TxOut) bool {
	return true
	fmt.Println("VerifyTxScript")
	fmt.Println("sigScript:", bin2hex(sig[:]))
	fmt.Println("btcValue :", in.Value)
	fmt.Println("pkScript :", bin2hex(in.Pk_script[:]))

	var st scrStack
	evalScript(sig[:], &st)
	evalScript(in.Pk_script[:], &st)

	return false
}

func evalScript(p []byte, stack *scrStack) bool {
	idx := 0
	for idx < len(p) {
		opcode := int(p[idx])
		idx++

		if opcode == OP_CAT ||
			opcode == OP_SUBSTR ||
			opcode == OP_LEFT ||
			opcode == OP_RIGHT ||
			opcode == OP_INVERT ||
			opcode == OP_AND ||
			opcode == OP_OR ||
			opcode == OP_XOR ||
			opcode == OP_2MUL ||
			opcode == OP_2DIV ||
			opcode == OP_MUL ||
			opcode == OP_DIV ||
			opcode == OP_MOD ||
			opcode == OP_LSHIFT ||
			opcode == OP_RSHIFT {
			return false
		}

		if opcode <= OP_PUSHDATA4 {
			stack.push(p[idx:idx+opcode])
			fmt.Printf("evalScript: %d bytes pushed\n", opcode)
			idx += opcode
		} else if opcode==OP_CHECKSIG || opcode==OP_CHECKSIGVERIFY {
			
			println("doing OP_CHECKSIG")
			
			if stack.size() < 2 {
				return false
			}

			vchPubKey := stack.pop()
			vchSig := stack.pop()
			//vchSig    := stack.top(-2);
			//vchPubKey := stack.top(-1);

			scriptCode := p[:]
			println("sig", bin2hex(vchSig[:]))
			println("pub", bin2hex(vchPubKey[:]))
			println("cod", bin2hex(scriptCode[:]))

			os.Exit(1)
/*
            // Subset of script starting at the most recent codeseparator
            CScript scriptCode(pbegincodehash, pend);

            // Drop the signature, since there's no way for a signature to sign itself
            scriptCode.FindAndDelete(CScript(vchSig));

            bool fSuccess = (!fStrictEncodings || (IsCanonicalSignature(vchSig) && IsCanonicalPubKey(vchPubKey)));
            if (fSuccess)
                fSuccess = CheckSig(vchSig, vchPubKey, scriptCode, txTo, nIn, nHashType, flags);

            popstack(stack);
            popstack(stack);
            stack.push_back(fSuccess ? vchTrue : vchFalse);
            if (opcode == OP_CHECKSIGVERIFY)
            {
                if (fSuccess)
                    popstack(stack);
                else
                    return false;
            }
*/
		} else {
			fmt.Printf("evalScript: Unexpected script command 0x%02X\n", opcode)
			os.Exit(1)
		}
	}
	return true
}
