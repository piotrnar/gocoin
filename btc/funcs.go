package btc


func allzeros(b []byte) bool {
	for i := range b {
		if b[i]!=0 {
			return false
		}
	}
	return true
}


func PutVlen(b []byte, vl int) uint32 {
	uvl := uint32(vl)
	if uvl<0xfd {
		b[0] = byte(uvl)
		return 1
	}
	if uvl<0x10000 {
		b[0] = 0xfd
		b[1] = byte(uvl)
		b[2] = byte(uvl>>8)
		return 3
	}
	b[0] = 0xfe
	b[1] = byte(uvl)
	b[2] = byte(uvl>>8)
	b[3] = byte(uvl>>16)
	b[4] = byte(uvl>>24)
	return 5
}


// Returns length and number of bytes that the var_int took
func VLen(b []byte) (len int, var_int_siz int) {
	c := b[0]
	if c < 0xfd {
		return int(c), 1
	}
	var_int_siz = 1 + (2 << (2-(0xff-c)))
	
	var res uint64
	for i:=1; i<var_int_siz; i++ {
		res |= (uint64(b[i]) << uint64(8*(i-1)))
	}
	
	if res>0x7fffffff {
		panic("This should never happen")
	}

	len = int(res)
	return
}


func getMerkel(txs []*Tx) ([]byte) {
	mtr := make([][]byte, len(txs))
	for i := range txs {
		mtr[i] = txs[i].Hash.Hash[:]
	}
	var j, i2 int
	for siz:=len(txs); siz>1; siz=(siz+1)/2 {
		for i := 0; i < siz; i += 2 {
			if i+1 < siz-1 {
				i2 = i+1
			} else {
				i2 = siz-1
			}
			h := Sha2Sum(append(mtr[j+i], mtr[j+i2]...))
			mtr = append(mtr, h[:])
		}
		j += siz
	}
	return mtr[len(mtr)-1]
}

