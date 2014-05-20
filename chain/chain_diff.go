package chain

import (
	"math/big"
	"github.com/piotrnar/gocoin/btc"
)

var MaxPOWValue *big.Int // it's var but used as a constant

const (
	POWRetargetSpam = 14 * 24 * 60 * 60 // two weeks
	TargetSpacing = 10 * 60
	targetInterval = POWRetargetSpam / TargetSpacing
	MaxPOWBits = 0x1d00ffff
)

func init() {
	MaxPOWValue, _ = new(big.Int).SetString("00000000FFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFFF", 16)
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
	bnewbn := btc.SetCompact(prv.Bits())
	bnewbn.Mul(bnewbn, big.NewInt(actualTimespan))
	bnewbn.Div(bnewbn, big.NewInt(POWRetargetSpam))

	if bnewbn.Cmp(MaxPOWValue) > 0 {
		bnewbn = MaxPOWValue
	}

	res = btc.GetCompact(bnewbn)

	return
}
