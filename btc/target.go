package btc

import (
	"math/big"
)

var MaxPOWValue *big.Int // it's var but used as a constant

const (
	POWRetargetSpam = 14 * 24 * 60 * 60 // two weeks
	TargetSpacing = 10 * 60
	targetInterval = POWRetargetSpam / TargetSpacing
	MaxPOWBits = 0x1d00ffff
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


func (ch *Chain) GetNextWorkRequired(lst *BlockTreeNode, ts uint32) (res uint32) {
	// Genesis block
	if lst.Parent == nil {
		return MaxPOWBits
	}

	if ((lst.Height+1) % targetInterval) != 0 {
		// Special difficulty rule for testnet:
		if ch.testnet() {
			// If the new block's timestamp is more than 2* 10 minutes
			// then allow mining of a min-difficulty block.
			if ts > lst.Timestamp() + TargetSpacing*2 {
				return MaxPOWBits;
			} else {
				// Return the last non-special-min-difficulty-rules-block
				prv := lst
				for prv.Parent!=nil && (prv.Height%targetInterval)!=0 && prv.Bits()==MaxPOWBits {
					prv = prv.Parent
				}
				return prv.Bits()
			}
		}
		return lst.Bits()
	}

	prv := lst
	for i:=0; i<targetInterval-1; i++ {
		prv = prv.Parent
	}

	actualTimespan := int64(lst.Timestamp() - prv.Timestamp())

	if actualTimespan < POWRetargetSpam/4 {
		actualTimespan = POWRetargetSpam/4
	}
	if actualTimespan > POWRetargetSpam*4 {
		actualTimespan = POWRetargetSpam*4
	}

	// Retarget
	bnewbn := SetCompact(prv.Bits())
	bnewbn.Mul(bnewbn, big.NewInt(actualTimespan))
	bnewbn.Div(bnewbn, big.NewInt(POWRetargetSpam))

	if bnewbn.Cmp(MaxPOWValue) > 0 {
		bnewbn = MaxPOWValue
	}

	res = GetCompact(bnewbn)

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

func init() {
	MaxPOWValue, _ = new(big.Int).SetString("00000000FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF", 16)
}
