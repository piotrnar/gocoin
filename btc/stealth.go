package btc

import (
	"bytes"
	"errors"
	"math/big"
	"crypto/sha256"
)

type xyz_t struct {
	x, y, z *big.Int
}

var (
	BigInt3 *big.Int = new(big.Int).SetInt64(3)
)


func (a *xyz_t) equal(b *xyz_t) bool {
	return a.x.Cmp(b.x)==0 && a.y.Cmp(b.y)==0  && a.z.Cmp(b.z)==0
}


func (a *xyz_t) twice(r *xyz_t) {
	x1 := a.x
	y1 := a.y

	y1z1 := new(big.Int).Mul(y1, a.z)

	y1sqz1 := new(big.Int).Mul(y1z1, y1)
	y1sqz1.Mod(y1sqz1, secp256k1.P)

	w := new(big.Int).Mul(x1, x1)
	w.Mul(w, BigInt3)
	w.Mod(w, secp256k1.P)

    w2 := new(big.Int).Mul(w, w)

    // x3 = 2 * y1 * z1 * (w^2 - 8 * x1 * y1^2 * z1)
	x3 := new(big.Int).Mul(new(big.Int).Lsh(x1, 3), y1sqz1) // 8 * x1 * y1^2 * z1
	x3.Sub(w2, x3) // w^2 - ...
	x3 = x3.Lsh(x3, 1) // ... *2
	x3 = x3.Mul(x3, y1z1) // * y1 * z1
	x3.Mod(x3, secp256k1.P) // mod

    // y3 = 4 * y1^2 * z1 * (3 * w * x1 - 2 * y1^2 * z1) - w^3
    y3 := new(big.Int).Lsh(y1sqz1, 1)   // 2 * y1^2 * z1
    y3.Sub(new(big.Int).Mul(new(big.Int).Mul(w, x1), BigInt3), y3) // (3 * w * x1) - ...
    y3.Mul(y3, y1sqz1) // * y1^2 * z1
    y3.Lsh(y3, 2) // * 4
    w3 := new(big.Int).Mul(w2, w)
    y3.Sub(y3, w3)
	y3.Mod(y3, secp256k1.P) // mod

    // z3 = 8 * (y1 * z1)^3
    z3 := new(big.Int).Mul(y1z1, y1z1)
    z3.Mul(z3, y1z1)
    z3.Lsh(z3, 3)
	z3.Mod(z3, secp256k1.P) // mod

	r.x = x3
	r.y = y3
	r.z = z3
}



func (a *xyz_t) add(r, b *xyz_t) {
	x1 := a.x
	x2 := b.x
	y1 := a.y
	y2 := b.y
	z1 := a.z
	z2 := b.z

	// u = Y2 * Z1 - Y1 * Z2
	u := new(big.Int).Sub(new(big.Int).Mul(y2, z1), new(big.Int).Mul(y1, z2))
	u.Mod(u, secp256k1.P)

    // v = X2 * Z1 - X1 * Z2
	v := new(big.Int).Sub(new(big.Int).Mul(x2, z1), new(big.Int).Mul(x1, z2))
	v.Mod(v, secp256k1.P)

	if v.Sign()==0 {
		if u.Sign()==0 {
			a.twice(r)
			return
		}
		println("ERROR:  FpADD- this should not happen")
		return
	}

	v2 := new(big.Int).Mul(v, v)
	v3 := new(big.Int).Mul(v2, v)
	x1v2 := new(big.Int).Mul(x1, v2)
	zu2 := new(big.Int).Mul(u, u)
	zu2.Mul(zu2, z1)

    // x3 = v * (z2 * (z1 * u^2 - 2 * x1 * v^2) - v^3)
	x3 := new(big.Int).Sub(zu2, new(big.Int).Lsh(x1v2, 1)) // (z1 * u^2 - 2 * x1 * v^2)
	x3.Mul(x3, z2)
	x3.Sub(x3, v3)
	x3.Mul(x3, v)
	x3.Mod(x3, secp256k1.P)

    // y3 = z2 * (3 * x1 * u * v^2 - y1 * v^3 - z1 * u^3) + u * v^3
	y3 := new(big.Int).Mul(x1v2, u) // x1 * u * v^2
	y3.Mul(y3, BigInt3) // .. *3
	tmp := new(big.Int).Mul(y1, v3) // ... - y1 * v^3
	y3.Sub(y3, tmp)
	tmp.Mul(zu2, u)
	y3.Sub(y3, tmp) // ... - z1 * u^3
	y3.Mul(y3, z2) // .. * z2
	tmp.Mul(u, v3)
	y3.Add(y3, tmp) // ... + u * v^3
	y3.Mod(y3, secp256k1.P)

	// z3 = v^3 * z1 * z2
	z3 := new(big.Int).Mul(v3, z1) // x1 * u * v^2
	z3.Mul(z3, z2)
	z3.Mod(z3, secp256k1.P)

	r.x = x3
	r.y = y3
	r.z = z3
}




// Simple NAF (Non-Adjacent Form) multiplication algorithm
// (whatever that is).
func (a *xyz_t) mul(r *xyz_t, k *big.Int) {
	var neg xyz_t

	*r = *a
	h := new(big.Int).Mul(k, BigInt3)
	neg.x = new(big.Int).Set(a.x)
	neg.y = new(big.Int).Neg(a.y)
	neg.z = new(big.Int).Set(a.z)
	for i:=h.BitLen()-2; i>0; i-- {
		r.twice(r)
		hb := h.Bit(i)
		if hb != k.Bit(i) {
			if hb!=0 {
				r.add(r, a)
			} else {
				r.add(r, &neg)
			}
		}
	}
	return
}


func StealthAddressVersion(testnet bool) byte {
	if testnet {
		return 43
	} else {
		return 42
	}
}


type StealthAddr struct {
	Version byte
	Options byte
	ScanKey [33]byte
	SpendKeys [][33]byte
	Sigs byte
	Prefix []byte
	Checksum []byte
}


func NewStealthAddrFromString(hs string) (a *StealthAddr, e error) {
	var tmp byte

	dec := Decodeb58(hs)
	if dec == nil {
		e = errors.New("StealthAddr: Cannot decode b58 string *"+hs+"*")
		return
	}
	if (len(dec)<2+33+33+1+1+4) {
		e = errors.New("StealthAddr: data too short")
		return
	}

	sh := Sha2Sum(dec[0:len(dec)-4])
	if !bytes.Equal(sh[:4], dec[len(dec)-4:len(dec)]) {
		e = errors.New("StealthAddr: Checksum error")
		return
	}

	a = new(StealthAddr)
	a.Checksum = sh[:4]

	b := bytes.NewBuffer(dec[0:len(dec)-4])

	if a.Version, e = b.ReadByte(); e != nil {
		a = nil
		return
	}
	if a.Options, e = b.ReadByte(); e != nil {
		a = nil
		return
	}
	if _, e = b.Read(a.ScanKey[:]); e != nil {
		a = nil
		return
	}
	if tmp, e = b.ReadByte(); e != nil {
		a = nil
		return
	}
	a.SpendKeys = make([][33]byte, int(tmp))
	for i := range a.SpendKeys {
		if _, e = b.Read(a.SpendKeys[i][:]); e != nil {
			a = nil
			return
		}
	}
	if a.Sigs, e = b.ReadByte(); e != nil {
		a = nil
		return
	}
	a.Prefix = b.Bytes()
	return
}

// Calculate the stealth difference
func StealthDH(pub, priv []byte) []byte {
	pk, er := NewPublicKey(pub)
	if er != nil {
		println(er.Error())
		return nil
	}
	var p, res xyz_t
	p.x = new(big.Int).Set(pk.X)
	p.y = new(big.Int).Set(pk.Y)
	p.z = new(big.Int).SetInt64(1)
	p.mul(&res, new(big.Int).SetBytes(priv))

	s := sha256.New()
	s.Write([]byte{0x03})
	s.Write(res.x.Bytes())
	return s.Sum(nil)
}
