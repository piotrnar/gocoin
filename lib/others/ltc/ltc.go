package ltc

import (
	"bytes"
	"github.com/piotrnar/gocoin/lib/btc"
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
	if ad!=nil && ad.Version==btc.AddrVerPubkey(false) {
		ad.Version = LTC_ADDR_VERSION
	}
	return
}
