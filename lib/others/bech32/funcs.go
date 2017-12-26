package bech32

func GetSegwitHRP(testnet bool) string {
	if testnet {
		return "tb"
	} else {
		return "bc"
	}
}

