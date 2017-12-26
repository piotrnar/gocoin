package bech32

func GetSegwitHRP(testnet bool) string {
	if testnet {
		return "tb"
	} else {
		return "bc"
	}
}

func MyEncode(hrc string, ver int, prog []byte) (string, error) {
	p := make([]int, len(prog))
	for i := range prog {
		p[i] = int(prog[i])
	}
	return SegwitAddrEncode(hrc, ver, p)
}
