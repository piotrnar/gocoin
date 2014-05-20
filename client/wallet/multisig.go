package wallet

import (
	"os"
	"io/ioutil"
	"encoding/json"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/client/common"
)

type MultisigAddr struct {
	MultiAddress string
	ScriptPubKey string
	KeysRequired, KeysProvided uint
	RedeemScript string
	ListOfAddres []string
}

func IsMultisig(ad *btc.BtcAddr) (yes bool, rec *MultisigAddr) {
	yes = ad.Version==btc.AddrVerScript(common.Testnet)
	if !yes {
		return
	}

	fn := common.GocoinHomeDir + "wallet" +
		string(os.PathSeparator) + "multisig" +
		string(os.PathSeparator) + ad.String() + ".json"

	d, er := ioutil.ReadFile(fn)
	if er != nil {
		//println("fn", fn, er.Error())
		return
	}

	var msa MultisigAddr
	er = json.Unmarshal(d, &msa)
	if er == nil {
		rec = &msa
	} else {
		println(fn, er.Error())
	}

	return
}
