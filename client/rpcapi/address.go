package rpcapi

import (
	"encoding/hex"
	"github.com/piotrnar/gocoin/lib/btc"
	//"github.com/piotrnar/gocoin/client/common"
)

/*

{"result":
	{"isvalid":true,
	"address":"mqzwxBkSH1UKqEAjGwvkj6aV5Gc6BtBCSs",
	"scriptPubKey":"76a91472fc9e6b1bbbd40a66653989a758098bfbf1b54788ac",
	"ismine":false,
	"iswatchonly":false,
	"isscript":false
}
*/

type ValidAddressResponse struct {
	Address      string `json:"address"`
	ScriptPubKey string `json:"scriptPubKey"`
	IsValid      bool   `json:"isvalid"`
	IsMine       bool   `json:"ismine"`
	IsWatchOnly  bool   `json:"iswatchonly"`
	IsScript     bool   `json:"isscript"`
}

type InvalidAddressResponse struct {
	IsValid bool `json:"isvalid"`
}

func ValidateAddress(addr string) interface{} {
	a, e := btc.NewAddrFromString(addr)
	if e != nil {
		return new(InvalidAddressResponse)
	}
	res := new(ValidAddressResponse)
	res.IsValid = true
	res.Address = addr
	res.ScriptPubKey = hex.EncodeToString(a.OutScript())
	return res
	//res.IsMine = false
	//res.IsWatchOnly = false
	//res.IsScript = false
}
