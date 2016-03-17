package btc

import (
	"math/big"
)

func SetCompact(nCompact uint32) (res *big.Int) {
	size := nCompact>>24
	neg := (nCompact&0x00800000)!=0
	word := nCompact & 0x007fffff
	if size <= 3 {
		word >>= 8*(3-size);
		res = big.NewInt(int64(word))
	} else {
		res = big.NewInt(int64(word))
		res.Lsh(res, uint(8*(size-3)))
	}
	if neg {
		res.Neg(res)
	}
	return res
}


func GetDifficulty(bits uint32) (diff float64) {
	shift := int(bits >> 24) & 0xff
	diff = float64(0x0000ffff) / float64(bits & 0x00ffffff)
	for shift < 29 {
		diff *= 256.0
		shift++
	}
	for shift > 29 {
		diff /= 256.0
		shift--
	}
	return
}


func GetCompact(b *big.Int) uint32 {

	size := uint32(len(b.Bytes()))
	var compact uint32

	if size <= 3 {
		compact = uint32(b.Int64() << uint(8*(3-size)))
	} else {
		b = new(big.Int).Rsh(b, uint(8*(size-3)))
		compact = uint32(b.Int64())
	}

	// The 0x00800000 bit denotes the sign.
	// Thus, if it is already set, divide the mantissa by 256 and increase the exponent.
	if (compact & 0x00800000) != 0 {
		compact >>= 8
		size++
	}
	compact |= size << 24
	if b.Cmp(big.NewInt(0)) < 0 {
		compact |= 0x00800000
	}
	return compact
}


func CheckProofOfWork(hash *Uint256, bits uint32) bool {
	return hash.BigInt().Cmp(SetCompact(bits)) <= 0
}
