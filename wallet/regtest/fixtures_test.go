package regtest

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/piotrnar/gocoin/lib/btc"
	"github.com/piotrnar/gocoin/lib/script"
)

// Fixed seeds. Never change them - all the expected values derive from them.
const (
	// TestSeed is the default seed password (type-3 and type-4 wallets).
	TestSeed = "qwerty12345"

	// Mnemonic is the well known BIP39 test vector (used with bip39=-1).
	Mnemonic = "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	// Publicly known values derived from Mnemonic (BIP44 / BIP84 test vectors).
	MnemonicRootXprv  = "xprv9s21ZrQH143K3GJpoapnV8SFfukcVBSfeCficPSGfubmSFDxo1kuHnLisriDvSnRRuL2Qrg5ggqHKNVpxR86QEC8w35uxmGoggxtQTPvfUu"
	MnemonicBip44Adr  = "1LqBGSKuX5yYUonjxT5qGfpUsXKYYWeabA"
	MnemonicBip84Adr  = "bc1qcr8te4kr609gcawutmrza0j4xv80jy8z306fyu"
	MnemonicBip84Zpub = "zpub6u4KbU8TSgNuZSxzv7HaGq5Tk361gMHdZxnM4UYuwzg5CMLcNytzhobitV4Zq6vWtWHpG9QijsigkxAzXvQWyLRfLq1L7VxPP1tky1hPfD4"

	// Standalone keys imported through the .others file.
	OtherWifCompressed   = "KzAqX6gJsmvZmJjNrHk3UDZrgDytgF88KzE21TnGVXPC6e3zRHGi"
	OtherAdrCompressed   = "1M8UbAaJ132nzgWQEhBxhydswWgHpASA2R"
	OtherWifUncompressed = "5HqNqndG7xYfJu8KkkJ7AjVUfVsiWxT5AyLUpBsi2Upe5c2WaRj"
	OtherAdrUncompressed = "1AV28sMrWe81SgBK21o3KjznwUd5dTngnp"
)

// Type-3 mainnet wallet made from TestSeed: the first four keys in every address type.
var (
	P2KH   = []string{"1FFvTPc4nWKrWTxrLa9wVqbqqTF3VHYR3H", "1GWspqSfoRd5MP2KHVzf5wrgjbG3NiSWN8", "1B4G2Ptg3QuhyWFsB5jSuNZSy1nEsbM1e6", "15xXT7AU8GPpqUGyVTK6YVXzbVC9zzf1Cg"}
	Segwit = []string{"3AC9QtsA3ZJVxHPmFXhGsNN7tpfjPMuGnz", "3FLQWSv9ogfLgAudtN8tiJEbzrAg4njrsx", "3PqCPonuHRXr9wJFXKu1u2zxHc2TChfyzt", "3Lwu81V7Win2ZnFh4oYW8p7GXvjYTBUbXL"}
	Bech32 = []string{"bc1qn3jrfkqpps733jahyf2932ynnd2k7n2fapsnhk", "bc1q4gcxqs3r3jgff05dw2eax4dg4gkrwth2r6dsaz", "bc1qde8scycy6j8n26zrqjtc46c2z0e0rjwca3qz56", "bc1qxesvetzy0l03627m47kkznx7a2wuhqlks397ct"}
	Tap    = []string{"bc1pgz4puhkydh3hc5p9wdgltj3a57h92e20zp00qxeq34dfdslh6fts4uc26f", "bc1pj40jhjd6znrsge99wje4tg50urwsw7f2m6t5h5h0d8uz7h7f98pqt3u7nk", "bc1phyv7zjnjfa9qzejn7dmpqz7p7k73ns7c0hdrm42zyljj5macv97q706ddq", "bc1plm6p9eu6lz38dwzuwqdwcg6nf399zullpnar2tl7h4q9dq3ufy7qjy503v"}
	Pubkey = []string{"0240aa1e5ec46de37c50257351f5ca3da7ae55654f105ef01b208d5a96c3f7d257", "03955f2bc9ba14c70464a574b355a28fe0dd07792ade974bd2ef69f82f5fc929c2", "02b919e14a724f4a016653f376100bc1f5bd19c3d87dda3dd54227e52a6fb8617c", "02fef412e79af8a276b85c701aec23534c4a5173ff0cfa352ffebd4056823c493c"}
	WIF    = []string{"L1TWxvwjcGGmeerAYKXZ7asZHxPhgBEB1zRxcztJmyuTDf7sELMC", "KxyqnYZsfhmzZ5nwqADx54TXtEEWA2zEbSKn2sLbJX9WefGwuw8A", "KwJXQLP3jiAk615Kn66WDxYh6vSrFDk5R1eymGzp5XoGfEruJDf3", "L2x7irtvTdjeoVezL8Yc4nV9ss4wvvaqyCtkMVXoYhkJyMErmy5R"}
)

// Addresses that do not belong to the test wallet (payment destinations).
const (
	ForeignP2KH   = "1BitcoinEaterAddressDontSendf59kuE"
	ForeignP2SH   = "3J98t1WpEZ73CNmQviecrnyiWrnqRhWNLy"
	ForeignBech32 = "bc1qw508d6qejxtdg4y5r3zarvary0c5xw7kv8f3t4"
	ForeignTap    = "bc1p0xlxvlhemja6c4dqv22uapctqupfhlxm9h8z3k2e72q4k9hcz7vqzk5jj0"
	ForeignTest   = "mipcBbFg9gMiCh81Kj8tqqdgoZub1ZJRfn"
	ForeignTBech  = "tb1qw508d6qejxtdg4y5r3zarvary0c5xw7kxpjzsx"
	ForeignLTC    = "LM2WMpR1Rp6j3Sa59cMXMs1SPzj9eXpGc1"
)

// Utxo describes one unspent output to be placed in the balance/ folder.
// Outputs sharing the same Tx number end up in the same funding transaction.
type Utxo struct {
	Tx    int    // funding transaction number
	Addr  string // address of the output (any type the wallet lib can decode)
	Value uint64 // satoshis
	Label string // optional text following the outpoint in unspent.txt
	Skip  bool   // create the output in the tx, but do not list it in unspent.txt
}

// fundingTx builds a deterministic transaction that pays to the given outputs.
// The transactions do not exist on any chain - the wallet only needs them to
// know the previous outputs' scripts and values.
func fundingTx(t *testing.T, n int, outs []Utxo) *btc.Tx {
	t.Helper()
	tx := new(btc.Tx)
	tx.Version = 1
	prev := sha256.Sum256([]byte(fmt.Sprintf("gocoin wallet regtest funding tx #%d", n)))
	in := new(btc.TxIn)
	copy(in.Input.Hash[:], prev[:])
	in.Sequence = 0xffffffff
	tx.TxIn = []*btc.TxIn{in}
	for _, o := range outs {
		a, err := btc.NewAddrFromString(o.Addr)
		if err != nil {
			t.Fatalf("bad fixture address %s: %v", o.Addr, err)
		}
		tx.TxOut = append(tx.TxOut, &btc.TxOut{Value: o.Value, Pk_script: a.OutScript()})
	}
	raw := tx.Serialize()
	tx.SetHash(raw)
	return tx
}

// writeBalance creates balance/unspent.txt and the balance/<txid>.tx files.
func writeBalance(t *testing.T, dir string, utxos []Utxo) {
	t.Helper()
	groups := map[int][]Utxo{}
	var order []int
	for _, u := range utxos {
		if _, ok := groups[u.Tx]; !ok {
			order = append(order, u.Tx)
		}
		groups[u.Tx] = append(groups[u.Tx], u)
	}
	var unspent strings.Builder
	for _, n := range order {
		outs := groups[n]
		tx := fundingTx(t, n, outs)
		write(t, dir, filepath.Join("balance", tx.Hash.String()+".tx"), string(tx.Serialize()))
		for vout, o := range outs {
			if o.Skip {
				continue
			}
			fmt.Fprintf(&unspent, "%s-%d", tx.Hash.String(), vout)
			if o.Label != "" {
				unspent.WriteString(" " + o.Label)
			}
			unspent.WriteString("\n")
		}
	}
	write(t, dir, filepath.Join("balance", "unspent.txt"), unspent.String())
}

// loadBalanceTx reads one transaction from the work dir's balance/ folder.
func (r *Result) loadBalanceTx(txid *btc.Uint256) *btc.Tx {
	d, err := os.ReadFile(filepath.Join(r.Dir, "balance", txid.String()+".tx"))
	if err != nil {
		r.T.Fatalf("balance tx %s: %v", txid.String(), err)
	}
	tx, _ := btc.NewTx(d)
	if tx == nil {
		r.T.Fatalf("balance tx %s: cannot decode", txid.String())
	}
	tx.SetHash(d)
	return tx
}

// Tx decodes the (hex encoded) transaction stored in a file of the work dir.
func (r *Result) Tx(name string) *btc.Tx {
	r.T.Helper()
	raw, err := hex.DecodeString(strings.TrimSpace(r.File(name)))
	if err != nil {
		r.T.Fatalf("%s is not a hex dump: %v", name, err)
	}
	tx, n := btc.NewTx(raw)
	if tx == nil || n != len(raw) {
		r.T.Fatalf("%s: cannot decode the transaction", name)
	}
	tx.SetHash(raw)
	return tx
}

// TxID fails the test unless the transaction in the file has the expected ID.
func (r *Result) TxID(name, txid string) *btc.Tx {
	r.T.Helper()
	tx := r.Tx(name)
	if tx.Hash.String() != txid {
		r.T.Errorf("%s: TxID %s, expected %s", name, tx.Hash.String(), txid)
	}
	return tx
}

// VerifyTx checks, with the gocoin script engine, that every input of the
// transaction stored in the given file is correctly signed. It returns the tx.
func (r *Result) VerifyTx(name string) *btc.Tx {
	r.T.Helper()
	tx := r.Tx(name)
	tx.AllocVerVars()
	tx.Spent_outputs = make([]*btc.TxOut, len(tx.TxIn))
	for i, in := range tx.TxIn {
		ptx := r.loadBalanceTx(btc.NewUint256(in.Input.Hash[:]))
		if int(in.Input.Vout) >= len(ptx.TxOut) {
			r.T.Fatalf("input %d: vout %d out of range", i, in.Input.Vout)
		}
		tx.Spent_outputs[i] = ptx.TxOut[in.Input.Vout]
	}
	for i := range tx.TxIn {
		uo := tx.Spent_outputs[i]
		ok := script.VerifyTxScript(uo.Pk_script, &script.SigChecker{Tx: tx, Idx: i, Amount: uo.Value}, script.STANDARD_VERIFY_FLAGS)
		if !ok {
			r.T.Errorf("%s: input %d (%s) does not verify", name, i, tx.TxIn[i].Input.String())
		}
	}
	return tx
}

// ExpectOutputs checks the amounts and destinations of the transaction's outputs.
func (r *Result) ExpectOutputs(tx *btc.Tx, testnet bool, want ...string) {
	r.T.Helper()
	var got []string
	for _, o := range tx.TxOut {
		if len(o.Pk_script) > 0 && o.Pk_script[0] == 0x6a {
			got = append(got, fmt.Sprintf("OP_RETURN=%s", string(o.Pk_script[2:])))
			continue
		}
		a := btc.NewAddrFromPkScript(o.Pk_script, testnet)
		if a == nil {
			got = append(got, fmt.Sprintf("?=%d", o.Value))
			continue
		}
		got = append(got, fmt.Sprintf("%s=%d", a.String(), o.Value))
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		r.T.Errorf("outputs mismatch\n  want: %v\n  got:  %v", want, got)
	}
}

// VerifyMessageSig checks a base64 encoded signed message against the address.
func VerifyMessageSig(t *testing.T, addr, msg, sig string) {
	t.Helper()
	ad, err := btc.NewAddrFromString(addr)
	if err != nil {
		t.Fatal(err)
	}
	nv, s, err := btc.ParseMessageSignature(sig)
	if err != nil {
		t.Fatalf("bad signature %q: %v", sig, err)
	}
	var hash [32]byte
	btc.HashFromMessage([]byte(msg), hash[:])
	compressed := nv >= 31
	if compressed {
		nv -= 4
	}
	pub := s.RecoverPublicKey(hash[:], int(nv-27))
	if pub == nil {
		t.Fatalf("cannot recover public key from %q", sig)
	}
	if sa := btc.NewAddrFromPubkey(pub.Bytes(compressed), ad.Version); sa.Hash160 != ad.Hash160 {
		t.Errorf("signature %q does not match address %s", sig, addr)
	}
}

// lastLine returns the last non-empty line of s.
func lastLine(s string) string {
	ls := strings.Split(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(ls[len(ls)-1])
}

// multisig2of3 returns the P2SH redeem script (hex) and the address of a 2-of-3
// multisig made of wallet keys #1, #2 and the imported compressed "other" key.
func multisig2of3(t *testing.T) (redeemHex, addr string) {
	t.Helper()
	other, err := btc.DecodePrivateAddr(OtherWifCompressed)
	if err != nil {
		t.Fatal(err)
	}
	ms := btc.NewMultiSig(2)
	for _, pk := range []string{Pubkey[0], Pubkey[1], hex.EncodeToString(other.Pubkey)} {
		b, _ := hex.DecodeString(pk)
		ms.PublicKeys = append(ms.PublicKeys, b)
	}
	return hex.EncodeToString(ms.P2SH()), ms.BtcAddr(false).String()
}

// unsignedTx builds a raw (unsigned) transaction spending the given balance
// outputs (by funding tx number and vout) to the given "addr=satoshis" outputs.
func unsignedTx(t *testing.T, balance []Utxo, inputs [][2]int, outs ...string) string {
	t.Helper()
	groups := map[int][]Utxo{}
	for _, u := range balance {
		groups[u.Tx] = append(groups[u.Tx], u)
	}
	tx := new(btc.Tx)
	tx.Version = 2
	for _, in := range inputs {
		ftx := fundingTx(t, in[0], groups[in[0]])
		ti := new(btc.TxIn)
		ti.Input.Hash = ftx.Hash.Hash
		ti.Input.Vout = uint32(in[1])
		ti.Sequence = 0xffffffff
		tx.TxIn = append(tx.TxIn, ti)
	}
	for _, o := range outs {
		av := strings.SplitN(o, "=", 2)
		if len(av) != 2 {
			t.Fatalf("bad output spec %q", o)
		}
		val, err := strconv.ParseUint(av[1], 10, 64)
		if err != nil {
			t.Fatalf("bad output spec %q", o)
		}
		a, err := btc.NewAddrFromString(av[0])
		if err != nil {
			t.Fatal(err)
		}
		tx.TxOut = append(tx.TxOut, &btc.TxOut{Value: val, Pk_script: a.OutScript()})
	}
	return hex.EncodeToString(tx.Serialize())
}

func b64(s string) bool {
	_, err := base64.StdEncoding.DecodeString(s)
	return err == nil && len(s) == 88
}
