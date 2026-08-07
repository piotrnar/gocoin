package chain

import (
	"errors"
	"fmt"

	"github.com/piotrnar/gocoin/lib/script"
)

// SEQUENCE_LOCKTIME_GRANULARITY - time based relative lock-times are
// measured in units of 512 seconds (BIP68).
const SEQUENCE_LOCKTIME_GRANULARITY = 9

type timeLock struct {
	coinHeight uint32 // height of the block that the spent output came from
	secs       uint32 // how old (in seconds) the output has to be
}

// seqLocks collects BIP68 relative lock-time requirements of all the inputs
// of all the transactions from one block.
// Since violating any single one of them invalidates the entire block,
// we only need to remember the strongest requirement of each type.
type seqLocks struct {
	height uint32     // the lowest height at which the block can be mined
	times  []timeLock // resolving these needs the median time of old blocks, so do it later
}

// add takes the relative lock-time (if any) of one input into account.
// coinHeight is the height of the block containing the output being spent
// (for outputs created by the very same block - the height of this block).
func (l *seqLocks) add(sequence, coinHeight uint32) {
	if (sequence & script.SEQUENCE_LOCKTIME_DISABLE_FLAG) != 0 {
		return
	}
	span := sequence & script.SEQUENCE_LOCKTIME_MASK
	if (sequence & script.SEQUENCE_LOCKTIME_TYPE_FLAG) != 0 {
		if span != 0 { // a zero span is always satisfied, so don't bother
			l.times = append(l.times, timeLock{coinHeight: coinHeight,
				secs: span << SEQUENCE_LOCKTIME_GRANULARITY})
		}
		return
	}
	if coinHeight+span > l.height {
		l.height = coinHeight + span
	}
}

// checkSequenceLocks verifies the collected BIP68 requirements against
// the block that is being connected at the given height.
// It is the equivalent of EvaluateSequenceLocks() from Bitcoin Core.
func (ch *Chain) checkSequenceLocks(l *seqLocks, height uint32) error {
	if l.height == 0 && len(l.times) == 0 {
		return nil // no relative lock-times in this block
	}

	// The block is being connected on top of the current head of the chain.
	prev := ch.LastBlock()
	if prev == nil || prev.Height+1 != height {
		return errors.New("checkSequenceLocks(): unexpected chain head - RPC_Result:bad-txns-nonfinal")
	}

	if l.height > height {
		return errors.New(fmt.Sprint("checkSequenceLocks(): output not mature enough (needs height ",
			l.height, ") - RPC_Result:bad-txns-nonfinal"))
	}

	if len(l.times) == 0 {
		return nil
	}

	// Time based locks are measured against the median time past of the block
	// preceding the one that had created the output being spent.
	// Fetch all of them with a single walk down the chain.
	mtp := make(map[uint32]uint32, len(l.times))
	lowest := height
	for _, t := range l.times {
		h := heightOfMTP(t.coinHeight)
		mtp[h] = 0
		if h < lowest {
			lowest = h
		}
	}

	ch.BlockIndexAccess.Lock()
	blockTime := prev.GetMedianTimePast()
	for n := prev; n != nil; n = n.Parent {
		if _, ok := mtp[n.Height]; ok {
			mtp[n.Height] = n.GetMedianTimePast()
		}
		if n.Height <= lowest {
			break
		}
	}
	ch.BlockIndexAccess.Unlock()

	for _, t := range l.times {
		coinTime := mtp[heightOfMTP(t.coinHeight)]
		if coinTime == 0 {
			return errors.New(fmt.Sprint("checkSequenceLocks(): no median time for height ",
				t.coinHeight, " - RPC_Result:bad-txns-nonfinal"))
		}
		if coinTime+t.secs > blockTime {
			return errors.New(fmt.Sprint("checkSequenceLocks(): output not old enough (needs ",
				coinTime+t.secs, ", got ", blockTime, ") - RPC_Result:bad-txns-nonfinal"))
		}
	}

	return nil
}

func heightOfMTP(coinHeight uint32) uint32 {
	if coinHeight > 0 {
		return coinHeight - 1
	}
	return 0
}
