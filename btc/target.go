package btc

import (
//	"fmt"
	"errors"
	"math/big"
)


const (
	nTargetTimespan = 14 * 24 * 60 * 60 // two weeks
	nTargetSpacing = 10 * 60
	nInterval = nTargetTimespan / nTargetSpacing
	nProofOfWorkLimit = 0x1d00ffff
)

var (
	bnProofOfWorkLimit *big.Int
	testnet bool
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


func CheckProofOfWork(hash *Uint256, nBits uint32) error {
	bnTarget := SetCompact(nBits)

	if bnTarget.Cmp(big.NewInt(0)) <= 0 || bnTarget.Cmp(bnProofOfWorkLimit) > 0 {
		return errors.New("CheckProofOfWork() : nBits below minimum work")
	}

	// Check proof of work matches claimed amount
	if bnTarget.Cmp(hash.BigInt()) < 0 {
		return errors.New("CheckProofOfWork() : hash doesn't match nBits")
	}

	return nil
}

func GetNextWorkRequired(lst *BlockTreeNode, ts uint32) (res uint32) {

	// Genesis block
	if lst.Parent == nil {
		return nProofOfWorkLimit
	}

	if ((lst.Height+1) % nInterval) != 0 {
		//println("*newdiff")
		// Special difficulty rule for testnet:
		if testnet {
			// If the new block's timestamp is more than 2* 10 minutes
			// then allow mining of a min-difficulty block.
			if ts > lst.Timestamp + nTargetSpacing*2 {
				return nProofOfWorkLimit;
			} else {
				// Return the last non-special-min-difficulty-rules-block
				prv := lst
				for prv.Parent!=nil && (prv.Height%nInterval)!=0 && prv.Bits==nProofOfWorkLimit {
					prv = prv.Parent
				}
				return prv.Bits
			}
		}
		return lst.Bits
	}

	prv := lst
	for i:=0; i<nInterval-1; i++ {
		prv = prv.Parent
	}

	nActualTimespan := int64(lst.Timestamp - prv.Timestamp)
	//println("  nActualTimespan =", nActualTimespan, " before bounds")

	if nActualTimespan < nTargetTimespan/4 {
		nActualTimespan = nTargetTimespan/4
	}
	if nActualTimespan > nTargetTimespan*4 {
		nActualTimespan = nTargetTimespan*4
	}

	// Retarget
	bnNew := SetCompact(prv.Bits)
	bnNew.Mul(bnNew, big.NewInt(nActualTimespan))
	bnNew.Div(bnNew, big.NewInt(nTargetTimespan))

	if bnNew.Cmp(bnProofOfWorkLimit) > 0 {
		bnNew = bnProofOfWorkLimit
	}

	res = GetCompact(bnNew)
	/*
	fmt.Printf("Block %d: diff %.2f => %.2f\n", lst.Height+1,
		GetDifficulty(lst.Bits), GetDifficulty(res))*/

	/*
	fmt.Printf("GetNextWorkRequired RETARGET\n");
	fmt.Printf("GetNextWorkRequired RETARGET\n");
	fmt.Printf("nTargetTimespan = %d    nActualTimespan = %d\n", nTargetTimespan, nActualTimespan)
	fmt.Printf("Before: %08x\n", lst.Parent.Bits)
	fmt.Printf("After:  %08x\n", GetCompact(bnNew))
	*/

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
