package main

import (
	"os"
	"fmt"
	"flag"
	"bytes"
	"bufio"
	"strconv"
	"strings"
	"math/big"
	"crypto/rand"
	"crypto/ecdsa"
	"encoding/hex"
	"crypto/sha256"
	"github.com/piotrnar/gocoin/btc"
	"code.google.com/p/go.crypto/ripemd160"
)

var (
	keycnt *uint = flag.Uint("c", 100, "Set maximum number of keys")
	testnet *bool = flag.Bool("t", true, "Work with testnet addresses")

	amout *float64 = flag.Float64("amount", 0.0, "Amount to spend")
	fee *float64 = flag.Float64("fee", 0.0005, "Transaction fee")
	toaddr *string  = flag.String("to", "", "Destination address (where to send the money)")
	change *string  = flag.String("change", "", "Send change to this address")

	verbyte byte

	unspentOuts []*btc.TxPrevOut
	unspentOutsLabel []string
	amBtc, feeBtc, totBtc uint64
	loadedTxs map[[32]byte] *btc.Tx = make(map[[32]byte] *btc.Tx)
)


func getpass() string {
	f, e := os.Open("wallet.sec")
	if e != nil {
		println("Make sure to create wallet.sec file put your wallet's secret/password into it")
		println(e.Error())
		os.Exit(1)
	}
	le, _ := f.Seek(0, os.SEEK_END)
	buf := make([]byte, le)
	f.Seek(0, os.SEEK_SET)
	n, e := f.Read(buf[:])
	if e != nil {
		println(e.Error())
		os.Exit(1)
	}
	if int64(n)!=le {
		println("Something is wrong with teh password file")
	}
	return string(buf)
}

func getline() string {
	li, _, _ := bufio.NewReader(os.Stdin).ReadLine()
	return string(li)
}

func sharimp160(data []byte) (res [20]byte) {
	sha := sha256.New()
	rim := ripemd160.New()
	sha.Write(data)
	rim.Write(sha.Sum(nil)[:])
	copy(res[:], rim.Sum(nil))
	return
}


func getPubKey(curv *btc.BitCurve, priv_key []byte) (res [65]byte) {
	x, y := curv.ScalarBaseMult(priv_key)
	xd := x.Bytes()
	yd := y.Bytes()

	if len(xd)>32 || len(yd)>32 {
		println("x:", len(xd), "y:", len(yd))
		os.Exit(2)
	}

	res[0] = 4
	copy(res[1+32-len(xd):33], xd)
	copy(res[33+32-len(yd):65], yd)
	return
}

func load_balance() {
	f, e := os.Open("balance/unspent.txt")
	if e != nil {
		println(e.Error())
		os.Exit(1)
	}
	rd := bufio.NewReader(f)
	for {
		l, _, e := rd.ReadLine()
		if len(l)==0 && e!=nil {
			break
		}
		if l[64]=='-' {
			txid := btc.NewUint256FromString(string(l[:64]))
			rst := strings.SplitN(string(l[65:]), " ", 2)
			vout, _ := strconv.ParseUint(rst[0], 10, 32)
			uns := new(btc.TxPrevOut)
			copy(uns.Hash[:], txid.Hash[:])
			uns.Vout = uint32(vout)
			unspentOuts = append(unspentOuts, uns)
			lab := ""
			if len(rst)>1 {
				lab = rst[1]
			}
			unspentOutsLabel = append(unspentOutsLabel, lab)
			if _, ok := loadedTxs[txid.Hash]; !ok {
				tf, _ := os.Open("balance/"+txid.String()+".tx")
				if tf != nil {
					siz, _ := tf.Seek(0, os.SEEK_END)
					tf.Seek(0, os.SEEK_SET)
					buf := make([]byte, siz)
					tf.Read(buf)
					tf.Close()
					th := btc.Sha2Sum(buf)
					if bytes.Equal(th[:], txid.Hash[:]) {
						tx, _ := btc.NewTx(buf)
						if tx != nil {
							loadedTxs[txid.Hash] = tx
						} else {
							println("transaction is corrupt:", txid.String())
						}
					} else {
						println("transaction file is corrupt:", txid.String())
						os.Exit(1)
					}
				} else {
					println("transaction file not found:", txid.String())
					os.Exit(1)
				}
			}
			totBtc += UO(uns).Value
		}
	}
	f.Close()
	fmt.Printf("%.8f BTC in %d unspent outputs\n", float64(totBtc)/1e8, len(unspentOuts))
}

func UO(uns *btc.TxPrevOut) *btc.TxOut {
	tx, _ := loadedTxs[uns.Hash]
	return tx.TxOut[uns.Vout]
}

func main() {
	if flag.Lookup("h") != nil {
		flag.PrintDefaults()
		os.Exit(0)
	}
	flag.Parse()

	if *amout<=0 {
		fmt.Println("Specify -amount parameter (how much you want to spend)")
		return
	}
	
	if *toaddr=="" {
		fmt.Println("Specify -to parameter (where you want to transfer it)")
		return
	}
	dest, e := btc.NewAddrFromString(*toaddr)
	if e != nil {
		fmt.Println("Destination address:", e.Error())
		return
	}
	
	var chng *btc.BtcAddr
	if *change!="" {
		var e error
		chng, e = btc.NewAddrFromString(*change)
		if e != nil {
			fmt.Println("Change address:", e.Error())
			return
		}
	}

	pass := getpass()
	
	if *testnet {
		verbyte = 0x6f
	} else {
		verbyte = 0
	}

	load_balance()

	amBtc = uint64(*amout*1e8)
	feeBtc = uint64(*fee*1e8)

	if amBtc + feeBtc > totBtc {
		fmt.Println("You want to spend more than you own")
		return
	}

	curv := btc.S256()
	seed_key := btc.Sha2Sum([]byte(pass))
	priv_keys := make([][32]byte, *keycnt)
	publ_keys := make([][65]byte, *keycnt)
	publ_addrs := make([]*btc.BtcAddr, *keycnt)
	fmt.Println("Generating", *keycnt, "keys...")
	var i uint
	for i < *keycnt {
		seed_key = btc.Sha2Sum(seed_key[:])
		priv_keys[i] = seed_key
		publ_keys[i] = getPubKey(curv, seed_key[:])
		h160 := sharimp160(publ_keys[i][:])
		publ_addrs[i] = btc.NewAddrFromHash160(h160[:], verbyte)
		i++
	}
	fmt.Println("Private keys re-generated")

	tx := new(btc.Tx)
	tx.Version = 1
	tx.Lock_time = 0
	
	sofar := uint64(0)
	for i=0; i<uint(len(unspentOuts)); i++ {
		uo := UO(unspentOuts[i])
		
		tin := new(btc.TxIn)
		tin.Input = *unspentOuts[i]
		tin.Sequence = 0xffffffff
		tx.TxIn = append(tx.TxIn, tin)

		sofar += uo.Value
		if sofar >= amBtc + feeBtc {
			break
		}
	}
	fmt.Printf("Spending %d out of %d outputs...\n", i+1, len(unspentOuts))
	tx.TxOut = append(tx.TxOut, &btc.TxOut{Value:amBtc, Pk_script:dest.OutScript()})
	
	if sofar - amBtc - feeBtc > 0 {
		if chng == nil {
			// If change address was not specified, send the change to the first input address
			tx.TxOut = append(tx.TxOut, 
				&btc.TxOut{Value:sofar - amBtc - feeBtc, Pk_script:UO(unspentOuts[0]).Pk_script})
		} else {
			tx.TxOut = append(tx.TxOut, 
				&btc.TxOut{Value:sofar - amBtc - feeBtc, Pk_script:chng.OutScript()})
		}
	}

	//fmt.Println("Unsigned:", hex.EncodeToString(tx.Serialize()))
	
	for in := range tx.TxIn {
		uo := UO(unspentOuts[in])
		var found bool
		for j := range publ_addrs {
			if publ_addrs[j].Owns(uo.Pk_script) {
				if chng == nil {
					chng = publ_addrs[j]
				}
				// Load the private key
				var key ecdsa.PrivateKey
				key.PublicKey.Curve = btc.S256()
				key.PublicKey.X = new(big.Int).SetBytes(publ_keys[j][1:33])
				key.PublicKey.Y = new(big.Int).SetBytes(publ_keys[j][33:65])
				key.D = new(big.Int).SetBytes(priv_keys[j][:])

				//Calculate proper transaction hash
				h := tx.SignatureHash(uo.Pk_script, in, btc.SIGHASH_ALL)
				//fmt.Println("SignatureHash:", btc.NewUint256(h).String())
				
				// Sign
				r, s, err := ecdsa.Sign(rand.Reader, &key, h)
				if err != nil {
					println("Sign:", err.Error())
					return
				}
				rb := r.Bytes()
				sb := s.Bytes()
				
				if rb[0] >= 0x80 {
					rb = append([]byte{0x00}, rb...)
				}

				if sb[0] >= 0x80 {
					sb = append([]byte{0x00}, sb...)
				}

				// Output the signing result into a buffer
				busig := new(bytes.Buffer)
				busig.WriteByte(0x30)
				busig.WriteByte(byte(4+len(rb)+len(sb)))
				busig.WriteByte(0x02)
				busig.WriteByte(byte(len(rb)))
				busig.Write(rb)
				busig.WriteByte(0x02)
				busig.WriteByte(byte(len(sb)))
				busig.Write(sb)
				busig.WriteByte(0x01) // hash type

				// Output the signature and the public key into tx.ScriptSig
				buscr := new(bytes.Buffer)
				buscr.WriteByte(byte(busig.Len()))
				buscr.Write(busig.Bytes())
				
				buscr.WriteByte(0x41)
				buscr.Write(publ_keys[j][:])

				// assign:
				tx.TxIn[in].ScriptSig = buscr.Bytes()

				found = true
				break
			}
		}
		if !found {
			fmt.Println("Key not found for", hex.EncodeToString(uo.Pk_script))
			os.Exit(1)
		}
	}

	rawtx := tx.Serialize()
	tx.Hash = btc.NewSha2Hash(rawtx)

	hs := tx.Hash.String()
	fmt.Println(hs)
	
	f, _ := os.Create(hs[:8]+".txt")
	if f != nil {
		f.Write([]byte(hex.EncodeToString(rawtx)))
		f.Close()
		fmt.Println("Transaction data stored in", hs[:8]+".txt")
	}

	f, _ = os.Create("balance/unspent.txt")
	if f != nil {
		for j:=uint(0); j<uint(len(unspentOuts)); j++ {
			if j>i {
				fmt.Fprintln(f, unspentOuts[j], unspentOutsLabel[j])
			}
		}
		fmt.Println(i, "spent output(s) removed from 'balance/unspent.txt'")

		var addback int
		for out := range tx.TxOut {
			for j := range publ_addrs {
				if publ_addrs[j].Owns(tx.TxOut[out].Pk_script) {
					fmt.Fprintf(f, "%s-%03d # %.8f / %s\n", tx.Hash.String(), out,
						float64(tx.TxOut[out].Value)/1e8, publ_addrs[j].String())
					addback++
				}
			}
		}
		f.Close()
		if addback > 0 {
			f, _ = os.Create("balance/"+hs+".tx")
			if f != nil {
				f.Write(rawtx)
				f.Close()
			}
			fmt.Println(addback, "new output(s) appended to 'balance/unspent.txt'")
		}
	}

	//fmt.Println(hex.EncodeToString(tx.Serialize()))

	// Make the transaction
}
