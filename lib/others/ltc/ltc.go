package ltc

import (
	"bytes"
	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/others/utils"
	"github.com/piotrnar/gocoin/lib/utxo"
)

const LTC_ADDR_VERSION = 48

// LTC signing uses different seed string
func HashFromMessage(msg []byte, out []byte) {
	const MessageMagic = "Litecoin Signed Message:\n"
	b := new(bytes.Buffer)
	btc.WriteVlen(b, uint64(len(MessageMagic)))
	b.Write([]byte(MessageMagic))
	btc.WriteVlen(b, uint64(len(msg)))
	b.Write(msg)
	btc.ShaHash(b.Bytes(), out)
}

func AddrVerPubkey(testnet bool) byte {
	if !testnet {
		return LTC_ADDR_VERSION
	}
	return btc.AddrVerPubkey(testnet)
}

func NewAddrFromPkScript(scr []byte, testnet bool) (ad *btc.BtcAddr) {
	ad = btc.NewAddrFromPkScript(scr, testnet)
	if ad != nil && ad.Version == btc.AddrVerPubkey(false) {
		ad.Version = LTC_ADDR_VERSION
	}
	return
}

func GetUnspent(addr *btc.BtcAddr) (res utxo.AllUnspentTx) {
	var er error

	res, er = utils.GetUnspentFromBlockcypher(addr, "ltc")
	if er == nil {
		return
	}
	println("GetUnspentFromBlockcypher:", er.Error())

	return
}

// Download testnet's raw transaction from a web server
func GetTxFromWeb(txid *btc.Uint256) (raw []byte) {
	raw = utils.GetTxFromBlockcypher(txid, "ltc")
	if raw != nil && txid.Equal(btc.NewSha2Hash(raw)) {
		//println("GetTxFromBlockcypher - OK")
		return
	}

	return
}
