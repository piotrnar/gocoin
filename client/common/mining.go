package common

import (
	"bytes"
)

var MinerIds = [][2]string{
	{"BTC_Guild", "BTC Guild"},
	{"ASICMiner", "ASICMiner"},
	{"50BTC", "50BTC.com"},
	{"Slush", "/slush/"},
	// Dont know how to do Deepbit
	{"EclipseMC", "EMC "},
	{"Eligius", "Eligius"},
	{"BitMinter", "BitMinter"},
	{"Bitparking", "bitparking"},
	{"CoinLab", "CoinLab"},
	{"Triplemin", "Triplemining.com"},
	{"Ozcoin", "ozcoin"},
	{"SatoshiSys", "Satoshi Systems"},
	{"ST_Mining", "st mining corp"},
	{"GHash.IO", "\x80\xad\x90\xd4\x03\x58\x1f\xa3\xbf\x46\x08\x6a\x91\xb2\xd9\xd4\x12\x5d\xb6\xc1"}, // 1CjPR7Z5ZSyWk6WtXvSFgkptmpoi4UM9BC
	{"Discus Fish", "Mined by user"},
}

func MinedBy(bl []byte, id string) bool {
	max2search := 0x200
	if len(bl)<max2search {
		max2search = len(bl)
	}
	return bytes.Index(bl[0x51:max2search], []byte(id))!=-1
}


func MinedByUs(bl []byte) bool {
	LockCfg()
	minid := CFG.Beeps.MinerID
	UnlockCfg()
	if minid=="" {
		return false
	}
	return MinedBy(bl, minid)
}

func BlocksMiner(bl []byte) (string, int) {
	for i := range MinerIds {
		if MinedBy(bl, MinerIds[i][1]) {
			return MinerIds[i][0], i
		}
	}
	return "", -1
}
