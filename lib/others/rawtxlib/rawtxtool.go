package rawtxlib

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"

	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/ltc"
)

// hex_dump returns the hex dump with max 32 bytes per line.
func hex_dump(d []byte) (s string) {
	for {
		le := 32
		if len(d) < le {
			le = len(d)
		}
		s += "       " + hex.EncodeToString(d[:le]) + "\n"
		d = d[le:]
		if len(d) == 0 {
			return
		}
	}
}

func dump_raw_sigscript(o io.Writer, d []byte) bool {
	ss, er := btc.ScriptToText(d)
	if er != nil {
		println(er.Error())
		return false
	}

	p2sh := len(ss) >= 2 && d[0] == 0
	if p2sh {
		ms, er := btc.NewMultiSigFromScript(d)
		if er == nil {
			fmt.Fprintln(o, "      Multisig script", ms.SigsNeeded, "of", len(ms.PublicKeys))
			for i := range ms.PublicKeys {
				fmt.Fprintf(o, "       pkey%d = %s\n", i+1, hex.EncodeToString(ms.PublicKeys[i]))
			}
			for i := range ms.Signatures {
				fmt.Fprintf(o, "       R%d = %64s\n", i+1, hex.EncodeToString(ms.Signatures[i].R.Bytes()))
				fmt.Fprintf(o, "       S%d = %64s\n", i+1, hex.EncodeToString(ms.Signatures[i].S.Bytes()))
				fmt.Fprintf(o, "       HashType%d = %02x\n", i+1, ms.Signatures[i].HashType)
			}
			return len(ms.Signatures) >= int(ms.SigsNeeded)
		} else {
			println(er.Error())
		}
	}

	fmt.Fprintln(o, "      SigScript:")
	for i := range ss {
		if p2sh && i == len(ss)-1 {
			// Print p2sh script
			d, _ = hex.DecodeString(ss[i])
			s2, er := btc.ScriptToText(d)
			if er != nil {
				println(er.Error())
				p2sh = false
				fmt.Fprintln(o, "       ", ss[i])
				continue
				//return
			}
			fmt.Fprintln(o, "        P2SH spend script:")
			for j := range s2 {
				fmt.Fprintln(o, "        ", s2[j])
			}
		} else {
			fmt.Fprintln(o, "       ", ss[i])
		}
	}
	return true
}

func dump_sigscript(o io.Writer, d []byte) bool {
	if len(d) == 0 {
		fmt.Fprintln(o, "       WARNING: Empty sigScript")
		return false
	}
	rd := bytes.NewReader(d)

	// ECDSA Signature
	le, _ := rd.ReadByte()
	if le < 0x40 {
		return dump_raw_sigscript(o, d)
	}
	sd := make([]byte, le)
	_, er := rd.Read(sd)
	if er != nil {
		return dump_raw_sigscript(o, d)
	}
	sig, er := btc.NewSignature(sd)
	if er != nil {
		return dump_raw_sigscript(o, d)
	}
	fmt.Fprintf(o, "       R = %64s\n", hex.EncodeToString(sig.R.Bytes()))
	fmt.Fprintf(o, "       S = %64s\n", hex.EncodeToString(sig.S.Bytes()))
	fmt.Fprintf(o, "       HashType = %02x\n", sig.HashType)

	// Key
	le, er = rd.ReadByte()
	if er != nil {
		fmt.Fprintln(o, "       WARNING: PublicKey not present")
		fmt.Fprint(o, hex_dump(d))
		return false
	}

	sd = make([]byte, le)
	_, er = rd.Read(sd)
	if er != nil {
		fmt.Fprintln(o, "       WARNING: PublicKey too short", er.Error())
		fmt.Fprint(o, hex_dump(d))
		return false
	}

	fmt.Fprintf(o, "       PublicKeyType = %02x\n", sd[0])
	key, er := btc.NewPublicKey(sd)
	if er != nil {
		fmt.Fprintln(o, "       WARNING: PublicKey broken", er.Error())
		fmt.Fprint(o, hex_dump(d))
		return false
	}
	fmt.Fprintf(o, "       X = %64s\n", key.X.String())
	if le >= 65 {
		fmt.Fprintf(o, "       Y = %64s\n", key.Y.String())
	}

	if rd.Len() != 0 {
		fmt.Fprintln(o, "       WARNING: Extra bytes at the end of sigScript")
		fmt.Fprint(o, hex_dump(d[len(d)-rd.Len():]))
	}
	return true
}

// addr_from_pkscr gets the BtcAddr from pk_script.
func addr_from_pkscr(scr []byte, testnet, litecoin bool) *btc.BtcAddr {
	if litecoin {
		return ltc.NewAddrFromPkScript(scr, testnet)
	} else {
		return btc.NewAddrFromPkScript(scr, testnet)
	}
}

func ver_script(testnet, litecoin bool) byte {
	if litecoin {
		return ltc.AddrVerScript(testnet)
	} else {
		return btc.AddrVerScript(testnet)
	}
}

func Decode(o io.Writer, tx *btc.Tx, getpo func(*btc.TxPrevOut) *btc.TxOut, tstnet, ltc bool) (totin, totout, noins uint64) {
	var unsigned int
	fmt.Fprintln(o, "ID:", tx.Hash.String())
	fmt.Fprintln(o, "WTxID:", tx.WTxID().String())
	fmt.Fprintln(o, "Tx Version:", tx.Version)
	if tx.SegWit != nil {
		fmt.Fprintln(o, "Segregated Witness transaction", len(tx.SegWit))
	} else {
		fmt.Fprintln(o, "Regular (non-SegWit) transaction", len(tx.SegWit))
	}
	if tx.IsCoinBase() {
		if len(tx.TxIn[0].ScriptSig) >= 4 && tx.TxIn[0].ScriptSig[0] == 3 {
			fmt.Fprintln(o, "Coinbase TX from block height", uint(tx.TxIn[0].ScriptSig[1])|
				uint(tx.TxIn[0].ScriptSig[2])<<8|uint(tx.TxIn[0].ScriptSig[3])<<16)
		} else {
			fmt.Fprintln(o, "Coinbase TX from an unknown block")
		}
		s := hex.EncodeToString(tx.TxIn[0].ScriptSig)
		for len(s) > 0 {
			i := len(s)
			if i > 64 {
				i = 64
			}
			fmt.Fprintln(o, "  ", s[:i])
			s = s[i:]
		}
		for wia := range tx.SegWit {
			for wib, ww := range tx.SegWit[wia] {
				fmt.Fprintln(o, "  Witness", wia, wib, hex.EncodeToString(ww))
			}
		}
		//fmt.Fprintln(o, )
	} else {
		fmt.Fprintln(o, "TX IN cnt:", len(tx.TxIn))
		for i := range tx.TxIn {
			var po *btc.TxOut
			fmt.Fprintf(o, "%4d) %s sl=%d seq=%08x\n", i, tx.TxIn[i].Input.String(),
				len(tx.TxIn[i].ScriptSig), tx.TxIn[i].Sequence)
			if getpo != nil {
				po = getpo(&tx.TxIn[i].Input)
			}
			if po != nil {
				val := po.Value
				totin += val
				fmt.Fprintf(o, "%15s BTC from address %s\n", btc.UintToBtc(val), btc.NewAddrFromPkScript(po.Pk_script, tstnet))
			} else {
				noins++
			}

			if len(tx.TxIn[i].ScriptSig) > 0 {
				if !dump_sigscript(o, tx.TxIn[i].ScriptSig) {
					unsigned++
				}
			} else {
				if tx.SegWit == nil || len(tx.SegWit[i]) < 2 {
					if i < len(tx.SegWit) && len(tx.SegWit[i]) == 1 && (len(tx.SegWit[i][0])|1) == 65 {
						fmt.Fprintln(o, "      Schnorr signature:")
						fmt.Fprintln(o, "       ", hex.EncodeToString(tx.SegWit[i][0][:32]))
						fmt.Fprintln(o, "       ", hex.EncodeToString(tx.SegWit[i][0][32:]))
						if len(tx.SegWit[i][0]) == 65 {
							fmt.Fprintf(o, "        Hash Type 0x%02x\n", tx.SegWit[i][0][64])
						}
						goto skip_wintesses
					} else {
						unsigned++
					}
				}
			}
			if tx.SegWit != nil {
				fmt.Fprintln(o, "      Witness data:")
				for _, ww := range tx.SegWit[i] {
					if len(ww) == 0 {
						fmt.Fprintln(o, "       ", "OP_0")
					} else {
						fmt.Fprintln(o, "       ", hex.EncodeToString(ww))
					}
				}
			}
		skip_wintesses:
		}
	}
	fmt.Fprintln(o, "TX OUT cnt:", len(tx.TxOut))
	for i := range tx.TxOut {
		totout += tx.TxOut[i].Value
		fmt.Fprintf(o, "%4d) %15s BTC ", i, btc.UintToBtc(tx.TxOut[i].Value))
		addr := addr_from_pkscr(tx.TxOut[i].Pk_script, tstnet, ltc)
		if addr != nil {
			if addr.Version == ver_script(tstnet, ltc) {
				fmt.Fprintln(o, "to scriptH", addr.String())
			} else {
				fmt.Fprintln(o, "to address", addr.String())
			}
		} else {
			if tx.TxOut[i].Value > 0 {
				fmt.Fprintln(o, "WARNING!!! These coins go to non-standard Pk_script:")
			} else {
				fmt.Fprintln(o, "NULL output to Pk_script:")
			}
			ss, er := btc.ScriptToText(tx.TxOut[i].Pk_script)
			if er == nil {
				for i := range ss {
					fmt.Fprintln(o, "       ", ss[i])
				}
			} else {
				fmt.Fprintln(o, hex.EncodeToString(tx.TxOut[i].Pk_script))
				fmt.Fprintln(o, er.Error())
			}
		}
	}
	fmt.Fprintln(o, "Lock Time:", tx.Lock_time)
	fmt.Fprintln(o, "Transaction Size:", tx.Size, "   NoWitSize:", tx.NoWitSize, "   Weight:", tx.Weight(), "   VSize:", tx.VSize())

	fmt.Fprintln(o, "Output volume:", btc.UintToBtc(totout), "BTC")
	if noins == 0 {
		fmt.Fprintln(o, "Input volume :", btc.UintToBtc(totin), "BTC")
		fmt.Fprintln(o, "Transact. fee:", btc.UintToBtc(totin-totout), "BTC ->",
			fmt.Sprintf("%.3f", float64(totin-totout)/float64(tx.VSize())), "SPB")
	} else {
		fmt.Fprintln(o, "WARNING: Missing inputs.")
		fmt.Fprintln(o, "Unable to decode BTC input amount and the fee")
	}

	if !tx.IsCoinBase() {
		if unsigned > 0 {
			fmt.Fprintln(o, "WARNING:", unsigned, "out of", len(tx.TxIn), "inputs are not signed or signed only patially")
		} else {
			fmt.Fprintln(o, "All", len(tx.TxIn), "transaction inputs seem to be signed")
		}
	}
	return
}
