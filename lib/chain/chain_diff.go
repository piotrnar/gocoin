package chain

import (
	"math/big"

	"github.com/piotrnar/gocoin/lib/btc"
)

const (
	POWRetargetSpam = 14 * 24 * 60 * 60 // two weeks
	TargetSpacing   = 10 * 60
	targetInterval  = POWRetargetSpam / TargetSpacing
)

func (ch *Chain) GetNextWorkRequired(lst *BlockTreeNode, ts uint32) (res uint32) {
	// Genesis block
	if lst.Parent == nil {
		return ch.Consensus.MaxPOWBits
	}

	if ((lst.Height + 1) % targetInterval) != 0 {
		// Special difficulty rule for testnet:
		if ch.testnet() {
			// If the new block's timestamp is more than 2* 10 minutes
			// then allow mining of a min-difficulty block.
			if ts > lst.Timestamp()+TargetSpacing*2 {
				return ch.Consensus.MaxPOWBits
			} else {
				// Return the last non-special-min-difficulty-rules-block
				prv := lst
				for prv.Parent != nil && (prv.Height%targetInterval) != 0 && prv.Bits() == ch.Consensus.MaxPOWBits {
					prv = prv.Parent
				}
				return prv.Bits()
			}
		}
		return lst.Bits()
	}

	prv := lst
	for i := 0; i < targetInterval-1; i++ {
		prv = prv.Parent
	}

	actualTimespan := int64(lst.Timestamp() - prv.Timestamp())

	if actualTimespan < POWRetargetSpam/4 {
		actualTimespan = POWRetargetSpam / 4
	}
	if actualTimespan > POWRetargetSpam*4 {
		actualTimespan = POWRetargetSpam * 4
	}

	// Retarget
	var bnewbn *big.Int
	if ch.testnet4() {
		// Use the last non-special-min-difficulty-rules-block
		prv = lst
		for prv.Parent != nil && (prv.Height%targetInterval) != 0 && prv.Bits() == ch.Consensus.MaxPOWBits {
			prv = prv.Parent
		}
		bnewbn = btc.SetCompact(prv.Bits())
	} else {
		bnewbn = btc.SetCompact(lst.Bits())
	}
	bnewbn.Mul(bnewbn, big.NewInt(actualTimespan))
	bnewbn.Div(bnewbn, big.NewInt(POWRetargetSpam))

	if bnewbn.Cmp(ch.Consensus.MaxPOWValue) > 0 {
		bnewbn = ch.Consensus.MaxPOWValue
	}

	res = btc.GetCompact(bnewbn)

	return
}

// MorePOW returns true if b1 has more POW than b2.
func (b1 *BlockTreeNode) MorePOW(b2 *BlockTreeNode) bool {
	var b1sum, b2sum float64
	for b1.Height > b2.Height {
		b1sum += btc.GetDifficulty(b1.Bits())
		b1 = b1.Parent
	}
	for b2.Height > b1.Height {
		b2sum += btc.GetDifficulty(b2.Bits())
		b2 = b2.Parent
	}
	for b1 != b2 {
		b1sum += btc.GetDifficulty(b1.Bits())
		b2sum += btc.GetDifficulty(b2.Bits())
		b1 = b1.Parent
		b2 = b2.Parent
	}
	return b1sum > b2sum
}
