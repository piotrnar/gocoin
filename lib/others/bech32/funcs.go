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

func MyDecode(hrp, addr string) (ver int, prog []byte, er error) {
	var pri []int
	ver, pri, er = SegwitAddrDecode(hrp, addr)
	if er == nil {
		prog = make([]byte, len(pri))
		for i := range pri {
			prog[i] = byte(pri[i])
		}
	}
	return
}
