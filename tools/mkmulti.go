package main

import (
	"os"
	"fmt"
	"bytes"
	"strconv"
	"io/ioutil"
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
)

/*
{
"address" : "2NAHUDSC1EmbTBwQQp4VQ2FNzWDqHtmk1i6",
"redeemScript" : "512102cdc4fff0ad031ea5f2d0d4337e2bf976b84334f8f80b08fe3f69886d58bc5a8a2102ebf54926d3edaae51bde71f2976948559a8d43fce52f5e7ed9ed85dbaa449d7f52ae"
}
*/
func main() {
	var testnet bool
	if len(os.Args)<3 {
		fmt.Println("Specify one integer and at least one public key.")
		fmt.Println("For Testent, make the integer negative.")
		return
	}
	cnt, er := strconv.ParseInt(os.Args[1], 10, 32)
	if er!=nil {
		println("Count value:", er.Error())
		return
	}
	if cnt<0 {
		testnet = true
		cnt = -cnt
	}
	if cnt<1 || cnt>16 {
		println("The integer (required number of keys) must be between 1 and 16")
		return
	}
	buf := new(bytes.Buffer)
	buf.WriteByte(byte(0x50+cnt))
	fmt.Println("Trying to prepare multisig address for", cnt, "out of", len(os.Args)-2, "public keys ...")
	var pkeys byte
	var ads string
	for i:=2; i<len(os.Args); i++ {
		if pkeys==16 {
			println("Oh, give me a break. You don't need more than 16 public keys - stopping here!")
			break
		}
		d, er := hex.DecodeString(os.Args[i])
		if er != nil {
			println("pubkey", i, er.Error())
		}
		_, er = btc.NewPublicKey(d)
		if er != nil {
			println("pubkey", i, er.Error())
			return
		}
		pkeys++
		buf.WriteByte(byte(len(d)))
		buf.Write(d)
		if ads!="" {
			ads += ", "
		}
		ads += "\"" + btc.NewAddrFromPubkey(d, btc.AddrVerPubkey(testnet)).String() + "\""
	}
	buf.WriteByte(0x50+pkeys)
	buf.WriteByte(0xae)

	p2sh := buf.Bytes()
	addr := btc.NewAddrFromPubkey(p2sh, btc.AddrVerScript(testnet))

	rec := "{\n"
	rec += fmt.Sprintf("\t\"multiAddress\" : \"%s\",\n", addr.String())
	rec += fmt.Sprintf("\t\"scriptPubKey\" : \"a914%s87\",\n", hex.EncodeToString(addr.Hash160[:]))
	rec += fmt.Sprintf("\t\"keysRequired\" : %d,\n", cnt)
	rec += fmt.Sprintf("\t\"keysProvided\" : %d,\n", pkeys)
	rec += fmt.Sprintf("\t\"redeemScript\" : \"%s\",\n", hex.EncodeToString(p2sh))
	rec += fmt.Sprintf("\t\"listOfAddres\" : [%s]\n", ads)
	rec += "}\n"
	fname := addr.String()+".json"
	ioutil.WriteFile(fname, []byte(rec), 0666)
	fmt.Println("The address record stored in", fname)
}
