package main

import (
	"encoding/hex"
	"fmt"

	"github.com/piotrnar/gocoin/lib/btc"
)

// Use this tool with "Prnt" type of public master keys

func main() {
	hdw, er := btc.StringWallet("xpub6CmoCa2ASDeVgaXeVkWXaJ5ZmeEAvYoZFnt3EXc2dhZEMVDW8noe3wMhWyQ4kPFUFGJdVAduv5JaugCndZNghtJicQqu76UywCWDKGTz5us")
	if er != nil {
		println(er.Error())
		return
	}

	hdw.Prefix = btc.PublicZ // convert to bech32 format
	fmt.Println("Parent master public key:")
	fmt.Println("", hdw.String())

	fmt.Println("1st deposit address (e.g. m/84'/0'/0'/0/0):")
	fmt.Println("", hdw.Child(0).Child(0).PubAddr().String())
	fmt.Println("", hex.EncodeToString(hdw.Child(0).Child(0).Pub().Key))

	fmt.Println("1st change address (e.g. m/84'/0'/0'/1/0):")
	fmt.Println("", hdw.Child(1).Child(0).PubAddr().String())
	fmt.Println("", hex.EncodeToString(hdw.Child(1).Child(0).Pub().Key))
}
