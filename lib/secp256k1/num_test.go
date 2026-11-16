package secp256k1

import (
	"math/big"
	"math/rand"
	"testing"
)

// The lower part must be below 2^bits and both the parts must sum up back to the input.
// This also needs to work on the platforms where big.Word is 32 bits long.
func TestNumSplit(t *testing.T) {
	rnd := rand.New(rand.NewSource(5))
	var rl, rh Number
	var sum, lim big.Int
	for _, nbits := range []uint{1, 31, 32, 33, 63, 64, 65, 96, 100, 127, 128, 129, 160, 192, 255, 256} {
		lim.Lsh(BigInt1, nbits)
		for i := 0; i < 500; i++ {
			a := rand_number(rnd, 1+rnd.Intn(40))
			if i == 0 {
				a.SetInt64(0)
			}
			org := new(big.Int).Set(&a.Int)
			a.split(&rl, &rh, nbits)
			if a.Cmp(org) != 0 {
				t.Fatal("split modified its input", org.Text(16), nbits)
			}
			if rl.Sign() < 0 || rl.Cmp(&lim) >= 0 {
				t.Fatal("low part out of range", org.Text(16), nbits, rl.Text(16))
			}
			sum.Lsh(&rh.Int, nbits)
			sum.Add(&sum, &rl.Int)
			if sum.Cmp(org) != 0 {
				t.Fatal("split parts do not sum up", org.Text(16), nbits, rl.Text(16), rh.Text(16))
			}
		}
	}
}
