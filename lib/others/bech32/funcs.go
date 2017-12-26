package bech32

func GetSegwitHRP(testnet bool) string {
	if testnet {
		return "tb"
	} else {
		return "bc"
	}
}

func MyEncode(hrc string, ver byte, prog []byte) (string, error) {
	p := make([]int, len(prog))
	for i := range prog {
		p[i] = int(prog[i])
	}
	return SegwitAddrEncode(hrc, int(ver), p)
}
