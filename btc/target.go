package btc

import (
	"math/big"
)

var bnProofOfWorkLimit *big.Int // it's var but used as a constant

const (
	nTargetTimespan = 14 * 24 * 60 * 60 // two weeks
	nTargetSpacing = 10 * 60
	nInterval = nTargetTimespan / nTargetSpacing
	nProofOfWorkLimit = 0x1d00ffff
)


func SetCompact(nCompact uint32) (res *big.Int) {
	nSize := nCompact>>24
	fNegative := (nCompact&0x00800000)!=0
	nWord := nCompact & 0x007fffff
	if nSize <= 3 {
		nWord >>= 8*(3-nSize);
		res = big.NewInt(int64(nWord))
	} else {
		res = big.NewInt(int64(nWord))
		res.Lsh(res, uint(8*(nSize-3)))
	}
	if fNegative {
		res.Neg(res)
	}
	return res
}


func GetDifficulty(nBits uint32) (dDiff float64) {
	nShift := int(nBits >> 24) & 0xff
	dDiff = float64(0x0000ffff) / float64(nBits & 0x00ffffff)
	for nShift < 29 {
		dDiff *= 256.0
		nShift++
	}
	for nShift > 29 {
		dDiff /= 256.0
		nShift--
	}
	return
}


func (ch *Chain) GetNextWorkRequired(lst *BlockTreeNode, ts uint32) (res uint32) {
	// Genesis block
	if lst.Parent == nil {
		return nProofOfWorkLimit
	}

	if ((lst.Height+1) % nInterval) != 0 {
		// Special difficulty rule for testnet:
		if ch.testnet() {
			// If the new block's timestamp is more than 2* 10 minutes
			// then allow mining of a min-difficulty block.
			if ts > lst.Timestamp() + nTargetSpacing*2 {
				return nProofOfWorkLimit;
			} else {
				// Return the last non-special-min-difficulty-rules-block
				prv := lst
				for prv.Parent!=nil && (prv.Height%nInterval)!=0 && prv.Bits()==nProofOfWorkLimit {
					prv = prv.Parent
				}
				return prv.Bits()
			}
		}
		return lst.Bits()
	}

	prv := lst
	for i:=0; i<nInterval-1; i++ {
		prv = prv.Parent
	}

	nActualTimespan := int64(lst.Timestamp() - prv.Timestamp())

	if nActualTimespan < nTargetTimespan/4 {
		nActualTimespan = nTargetTimespan/4
	}
	if nActualTimespan > nTargetTimespan*4 {
		nActualTimespan = nTargetTimespan*4
	}

	// Retarget
	bnNew := SetCompact(prv.Bits())
	bnNew.Mul(bnNew, big.NewInt(nActualTimespan))
	bnNew.Div(bnNew, big.NewInt(nTargetTimespan))

	if bnNew.Cmp(bnProofOfWorkLimit) > 0 {
		bnNew = bnProofOfWorkLimit
	}

	res = GetCompact(bnNew)

	return
}


func GetCompact(b *big.Int) uint32 {

	nSize := uint32(len(b.Bytes()))
	var nCompact uint32

	if nSize <= 3 {
		nCompact = uint32(b.Int64() << uint(8*(3-nSize)))
	} else {
		b = new(big.Int).Rsh(b, uint(8*(nSize-3)))
		nCompact = uint32(b.Int64())
	}

	// The 0x00800000 bit denotes the sign.
	// Thus, if it is already set, divide the mantissa by 256 and increase the exponent.
	if (nCompact & 0x00800000) != 0 {
		nCompact >>= 8
		nSize++
	}
	nCompact |= nSize << 24
	if b.Cmp(big.NewInt(0)) < 0 {
		nCompact |= 0x00800000
	}
	return nCompact
}

func init() {
	bnProofOfWorkLimit, _ = new(big.Int).SetString("00000000FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF", 16)
}
