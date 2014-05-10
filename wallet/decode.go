package main

import (
	"fmt"
	"bytes"
	"encoding/hex"
	"github.com/piotrnar/gocoin/btc"
)

// hex dump with max 32 bytes per line
func hex_dump(d []byte) (s string) {
	for {
		le := 32
		if len(d) < le {
			le = len(d)
		}
		s += "       " + hex.EncodeToString(d[:le]) + "\n"
		d = d[le:]
		if len(d)==0 {
			return
		}
	}
}


func dump_raw_sigscript(d []byte) bool {
	ss, er := btc.ScriptToText(d)
	if er != nil {
		println(er.Error())
		return false
	}

	p2sh := len(ss)>=2 && d[0]==0
	if p2sh {
		ms, er := btc.NewMultiSigFromScript(d)
		if er==nil {
			fmt.Println("       Multisig script", ms.SigsNeeded, "of", len(ms.PublicKeys))
			for i := range ms.PublicKeys {
				fmt.Printf("       pkey%d = %s\n", i+1, hex.EncodeToString(ms.PublicKeys[i]))
			}
			for i := range ms.Signatures {
				fmt.Printf("       R%d = %64s\n", i+1, hex.EncodeToString(ms.Signatures[i].R.Bytes()))
				fmt.Printf("       S%d = %64s\n", i+1, hex.EncodeToString(ms.Signatures[i].S.Bytes()))
				fmt.Printf("       HashType%d = %02x\n", i+1, ms.Signatures[i].HashType)
			}
			return len(ms.Signatures)>=int(ms.SigsNeeded)
		} else {
			println(er.Error())
		}
	}

	fmt.Println("       SigScript:", p2sh)
	for i := range ss {
		if p2sh && i==len(ss)-1 {
			// Print p2sh script
			d, _ = hex.DecodeString(ss[i])
			s2, er := btc.ScriptToText(d)
			if er != nil {
				println(er.Error())
				p2sh = false
				fmt.Println("       ", ss[i])
				continue
				//return
			}
			fmt.Println("        P2SH spend script:")
			for j := range s2 {
				fmt.Println("        ", s2[j])
			}
		} else {
			fmt.Println("       ", ss[i])
		}
	}
	return true
}


func dump_sigscript(d []byte) bool {
	if len(d) < 10 + 34 { // at least 10 bytes for sig and 34 bytes key
		fmt.Println("       WARNING: Short sigScript")
		fmt.Print(hex_dump(d))
		return false
	}
	rd := bytes.NewReader(d)

	// ECDSA Signature
	le, _ := rd.ReadByte()
	if le<0x40 {
		return dump_raw_sigscript(d)
	}
	sd := make([]byte, le)
	_, er := rd.Read(sd)
	if er != nil {
		return dump_raw_sigscript(d)
	}
	sig, er := btc.NewSignature(sd)
	if er != nil {
		return dump_raw_sigscript(d)
	}
	fmt.Printf("       R = %64s\n", hex.EncodeToString(sig.R.Bytes()))
	fmt.Printf("       S = %64s\n", hex.EncodeToString(sig.S.Bytes()))
	fmt.Printf("       HashType = %02x\n", sig.HashType)

	// Key
	le, er = rd.ReadByte()
	if er != nil {
		fmt.Println("       WARNING: PublicKey not present")
		fmt.Print(hex_dump(d))
		return false
	}

	sd = make([]byte, le)
	_, er = rd.Read(sd)
	if er != nil {
		fmt.Println("       WARNING: PublicKey too short", er.Error())
		fmt.Print(hex_dump(d))
		return false
	}

	fmt.Printf("       PublicKeyType = %02x\n", sd[0])
	key, er := btc.NewPublicKey(sd)
	if er != nil {
		fmt.Println("       WARNING: PublicKey broken", er.Error())
		fmt.Print(hex_dump(d))
		return false
	}
	fmt.Printf("       X = %64s\n", key.X.String())
	if le>=65 {
		fmt.Printf("       Y = %64s\n", key.Y.String())
	}

	if rd.Len() != 0 {
		fmt.Println("       WARNING: Extra bytes at the end of sigScript")
		fmt.Print(hex_dump(d[len(d)-rd.Len():]))
	}
	return true
}


// dump raw transaction
func dump_raw_tx() {
	tx := raw_tx_from_file(*dumptxfn)
	if tx == nil {
		fmt.Println("ERROR: Cannot decode the raw transaction")
		return
	}

	var unsigned int

	fmt.Println("ID:", tx.Hash.String())
	fmt.Println("Tx Version:", tx.Version)
	if tx.IsCoinBase() {
		if len(tx.TxIn[0].ScriptSig) >= 4 && tx.TxIn[0].ScriptSig[0]==3 {
			fmt.Println("Coinbase TX from block height", uint(tx.TxIn[0].ScriptSig[1]) |
				uint(tx.TxIn[0].ScriptSig[2])<<8 | uint(tx.TxIn[0].ScriptSig[3])<<16)
		} else {
			fmt.Println("Coinbase TX from an unknown block")
		}
		s := hex.EncodeToString(tx.TxIn[0].ScriptSig)
		for len(s)>0 {
			i := len(s)
			if i>64 {
				i = 64
			}
			fmt.Println("  ", s[:i])
			s = s[i:]
		}
		//fmt.Println()
	} else {
		fmt.Println("TX IN cnt:", len(tx.TxIn))
		for i := range tx.TxIn {
			fmt.Printf("%4d) %s sl=%d seq=%08x\n", i, tx.TxIn[i].Input.String(),
				len(tx.TxIn[i].ScriptSig), tx.TxIn[i].Sequence)

			if len(tx.TxIn[i].ScriptSig) > 0 {
				if !dump_sigscript(tx.TxIn[i].ScriptSig) {
					unsigned++
				}
			} else {
				unsigned++
			}
		}
	}
	fmt.Println("TX OUT cnt:", len(tx.TxOut))
	for i := range tx.TxOut {
		fmt.Printf("%4d) %20s BTC ", i, btc.UintToBtc(tx.TxOut[i].Value))
		addr := btc.NewAddrFromPkScript(tx.TxOut[i].Pk_script, testnet)
		if addr != nil {
			if addr.Version==btc.AddrVerScript(testnet) {
				fmt.Println("to scriptH", addr.String())
			} else {
				fmt.Println("to address", addr.String())
			}
		} else if len(tx.TxOut[i].Pk_script)==40 && tx.TxOut[i].Pk_script[0]==0x6a &&
			tx.TxOut[i].Pk_script[1]==0x26 && tx.TxOut[i].Pk_script[2]==0x06 {
			fmt.Println("Stealth", hex.EncodeToString(tx.TxOut[i].Pk_script[3:7]),
				hex.EncodeToString(tx.TxOut[i].Pk_script[7:]))
		} else {
			if tx.TxOut[i].Value > 0 {
				fmt.Println("WARNING!!! These coins go to non-standard Pk_script:")
			} else {
				fmt.Println("NULL output to Pk_script:")
			}
			ss, er := btc.ScriptToText(tx.TxOut[i].Pk_script)
			if er == nil {
				for i := range ss {
					fmt.Println("       ", ss[i])
				}
			} else {
				fmt.Println(hex.EncodeToString(tx.TxOut[i].Pk_script))
				fmt.Println(er.Error())
			}
		}
	}
	fmt.Println("Lock Time:", tx.Lock_time)

	if !tx.IsCoinBase() {
		if unsigned>0 {
			fmt.Println("WARNING:", unsigned, "out of", len(tx.TxIn), "inputs are not signed or signed only patially")
		} else {
			fmt.Println("All", len(tx.TxIn), "transaction inputs seem to be signed")
		}
	}
}
