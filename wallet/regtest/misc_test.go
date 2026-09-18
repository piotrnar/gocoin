package regtest

import (
	"encoding/hex"
	"strings"
	"testing"
)

func mustHex(t *testing.T, s string) []byte {
	t.Helper()
	d, err := hex.DecodeString(s)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

const plainText = "The quick brown fox jumps over the lazy dog\n\x00\x01\x02\xff binary too"

func TestEncrypt(t *testing.T) {
	// decryptWith runs -decrypt on the given ciphertext with the given seed.
	decryptWith := func(t *testing.T, seed, name, cipher string, args ...string) *Result {
		t.Helper()
		return runCase(t, &Case{Seed: seed, Files: map[string]string{name: cipher}, Args: append([]string{"-decrypt", name}, args...)})
	}

	Run(t, []Case{
		{
			Name:       "encrypt_decrypt_roundtrip_type3",
			Files:      map[string]string{"secret.txt": plainText},
			Args:       []string{"-encrypt", "secret.txt"},
			Out:        []string{"# Deterministic Walet Type 3", "Encryped file saved as secret.txt.enc", "WARNING: verify that the x-pub-keys above match with your wallet"},
			FileExists: []string{"secret.txt.enc"},
			NotOut:     []string{"TypC"},
			Check: func(r *Result) {
				enc := r.File("secret.txt.enc")
				if enc == plainText || len(enc) <= len(plainText) {
					r.T.Fatal("file does not look encrypted")
				}
				r2 := decryptWith(r.T, TestSeed, "secret.txt.enc", enc)
				r2.Contains("Decryped file saved as secret.txt")
				if got := r2.File("secret.txt"); got != plainText {
					r.T.Errorf("decrypted content differs: %q", got)
				}
				// same file encrypted twice gives different ciphertext (random nonce) but decrypts the same
				r3 := runCase(r.T, &Case{Files: map[string]string{"secret.txt": plainText}, Args: []string{"-encrypt", "secret.txt"}})
				if r3.File("secret.txt.enc") == enc {
					r.T.Error("nonce is not random")
				}
			},
		},
		{
			Name:  "decrypt_wrong_password",
			Files: map[string]string{"secret.txt": plainText},
			Args:  []string{"-encrypt", "secret.txt"},
			Check: func(r *Result) {
				r2 := decryptWith(r.T, "wrong password", "secret.txt.enc", r.File("secret.txt.enc"))
				if r2.Exit != 1 {
					r.T.Errorf("exit code %d, expected 1", r2.Exit)
				}
				r2.Contains("message authentication failed")
				if r2.Exists("secret.txt") {
					r.T.Error("no output file expected")
				}
			},
		},
		{
			Name:  "decrypt_wrong_wallet_type",
			Files: map[string]string{"secret.txt": plainText},
			Args:  []string{"-encrypt", "secret.txt"},
			Check: func(r *Result) {
				r2 := decryptWith(r.T, TestSeed, "secret.txt.enc", r.File("secret.txt.enc"), "-type", "4")
				if r2.Exit != 1 {
					r.T.Errorf("type-4 key must not decrypt a type-3 file (exit %d)", r2.Exit)
				}
			},
		},
		{
			Name:  "encrypt_decrypt_roundtrip_type4_bip39",
			Cfg:   "type=4\nbip39=12\nhdpath=m/0/0\n",
			Files: map[string]string{"data.bin": plainText},
			Args:  []string{"-encrypt", "data.bin"},
			Out:   []string{"# Deterministic Walet Type 4", "# Based on 12 BIP39 words", "# Root: xpub", "# Leaf: xpub"},
			Check: func(r *Result) {
				r2 := runCase(r.T, &Case{Cfg: "type=4\nbip39=12\nhdpath=m/0/0\n",
					Files: map[string]string{"data.bin.enc": r.File("data.bin.enc")}, Args: []string{"-decrypt", "data.bin.enc"}})
				if r2.File("data.bin") != plainText {
					r.T.Error("roundtrip failed")
				}
				// a different hdpath does not change the AES key (it is derived from the master key)
				r3 := runCase(r.T, &Case{Cfg: "type=4\nbip39=12\nhdpath=m/44'/0'/0'/0\n",
					Files: map[string]string{"data.bin.enc": r.File("data.bin.enc")}, Args: []string{"-decrypt", "data.bin.enc"}})
				if r3.File("data.bin") != plainText {
					r.T.Error("roundtrip with a different hdpath failed")
				}
			},
		},
		{
			Name:  "decrypt_file_without_enc_suffix",
			Files: map[string]string{"secret.txt": plainText},
			Args:  []string{"-encrypt", "secret.txt"},
			Check: func(r *Result) {
				r2 := decryptWith(r.T, TestSeed, "blob", r.File("secret.txt.enc"))
				r2.Contains("Decryped file saved as blob.dec")
				if r2.File("blob.dec") != plainText {
					r.T.Error("roundtrip failed")
				}
			},
		},
		{
			Name:  "encrypt_then_list", // -l along with -encrypt still lists the addresses
			Files: map[string]string{"secret.txt": plainText},
			Args:  []string{"-encrypt", "secret.txt", "-l", "-n", "1"},
			Out:   []string{"Encryped file saved as", P2KH[0] + " TypC 1"},
		},
		{
			Name: "encrypt_missing_file",
			Args: []string{"-encrypt", "nope.txt"},
			Exit: 1,
			Out:  []string{"nope.txt"},
		},
		{
			Name:  "decrypt_too_short",
			Files: map[string]string{"x.enc": "abc"},
			Args:  []string{"-decrypt", "x.enc"},
			Exit:  1,
			Out:   []string{"ERROR: Encrypted message is shorter than the nonce size"},
		},
		{
			Name: "encrypt_and_decrypt_together",
			Args: []string{"-encrypt", "a", "-decrypt", "b"},
			Exit: 1,
			Out:  []string{"ERROR: you cannot do -encrypt and -decrypt at the same time"},
		},
	})
}

func TestErrors(t *testing.T) {
	Run(t, []Case{
		{
			Name: "unknown_switch",
			Args: []string{"-nosuchswitch"},
			Exit: 2,
			Out:  []string{"flag provided but not defined: -nosuchswitch"},
		},
		{
			Name: "help",
			Args: []string{"-h"},
			Exit: 0,
			Out:  []string{"-send string", "-list", "Same as -l (above)", "-cfg string"},
		},
		{
			Name: "uncompressed_disabled",
			Args: []string{"-l", "-u"},
			Exit: 1,
			Out:  []string{"For SegWit address safety, uncompressed keys are disabled in this version"},
		},
		{
			Name: "uncompressed_disabled_from_cfg",
			Cfg:  "uncompressed=true\n",
			Args: []string{"-l"},
			Exit: 1,
			Out:  []string{"WARNING: Using uncompressed keys", "uncompressed keys are disabled"},
		},
		{
			Name: "scrypt_too_big",
			Args: []string{"-l", "-scrypt", "31"},
			Exit: 1,
			Out:  []string{"ERROR: scrypt value too big"},
		},
		{
			Name: "scrypt_with_mnemonic",
			Seed: Mnemonic,
			Args: []string{"-l", "-type", "4", "-bip39", "-1", "-scrypt", "10"},
			Exit: 1,
			Out:  []string{"ERROR: Cannot use scrypt function in BIP39 mnemonic mode"},
		},
		{
			Name: "wallet_type_1_unsupported",
			Args: []string{"-l", "-type", "1"},
			Exit: 1,
			Out:  []string{"ERROR: Wallets Type 1 are no longer supported"},
		},
		{
			Name: "wallet_type_2_unsupported",
			Args: []string{"-l", "-type", "2"},
			Exit: 1,
			Out:  []string{"ERROR: Wallets Type 2 are no longer supported"},
		},
		{
			Name: "wallet_type_9_unsupported",
			Args: []string{"-l", "-type", "9"},
			Exit: 1,
			Out:  []string{"ERROR: Unsupported wallet type 9"},
		},
		{
			Name: "bip39_with_type3",
			Args: []string{"-l", "-bip39", "12"},
			Exit: 1,
			Out:  []string{"ERROR: BIP39 features not supported for wallet type 3"},
		},
		{
			Name: "words_with_type3",
			Args: []string{"-words"},
			Exit: 1,
			Out:  []string{"ERROR: BIP39 features not supported for wallet type 3"},
		},
		{
			Name: "bip39_bad_word_count",
			Args: []string{"-l", "-type", "4", "-bip39", "13"},
			Exit: 1,
			Out:  []string{"ERROR: Incorrect value for BIP39 words count 13"},
		},
		{
			Name: "hdpath_no_m",
			Args: []string{"-l", "-type", "4", "-hdpath", "x/0"},
			Exit: 1,
			Out:  []string{"ERROR: hdpath - top level syntax error: x/0"},
		},
		{
			Name: "hdpath_just_m",
			Args: []string{"-l", "-type", "4", "-hdpath", "m"},
			Exit: 1,
			Out:  []string{"ERROR: hdpath - top level syntax error: m"},
		},
		{
			Name: "hdpath_bad_index",
			Args: []string{"-l", "-type", "4", "-hdpath", "m/0/abc"},
			Exit: 1,
			Out:  []string{"ERROR: hdpath - syntax error. non-negative integer expected: abc"},
		},
		{
			Name: "hdpath_negative_index",
			Args: []string{"-l", "-type", "4", "-hdpath", "m/-1"},
			Exit: 1,
			Out:  []string{"non-negative integer expected: -1"},
		},
		{
			Name: "atype_invalid",
			Args: []string{"-l", "-atype", "p2pk"},
			Exit: 1,
			Out:  []string{"ERROR: Invalid value of atype: p2pk"},
		},
		{
			Name: "atype_invalid_from_cfg",
			Cfg:  "atype=nope\n",
			Args: []string{"-l"},
			Exit: 1,
			Out:  []string{"ERROR: Invalid value of atype: nope"},
		},
		{
			Name: "fee_invalid",
			Args: []string{"-l", "-fee", "1,5"},
			Exit: 1,
			Out:  []string{"Incorrect fee value 1,5"},
		},
		{
			Name: "fee_invalid_from_cfg",
			Cfg:  "fee=free\n",
			Args: []string{"-l"},
			Exit: 1,
			Out:  []string{"Incorrect fee value free"},
		},
		{
			Name:   "ask4pass_with_stdin",
			Args:   []string{"-l", "-p", "-stdin"},
			Stdin:  TestSeed,
			Out:    []string{"both -p and -stdin switches are not allowed"},
			NotOut: []string{"TypC"},
		},
		{
			Name:   "key_index_out_of_range_dump", // -n 2 does not include key #4
			Args:   []string{"-dump", P2KH[3], "-n", "2"},
			Out:    []string{"not found it the wallet"},
			NotOut: []string{WIF[3]},
		},
		{
			Name:        "no_wallet_txt_on_error",
			Args:        []string{"-l", "-type", "4", "-hdpath", "bad"},
			Exit:        1,
			FileMissing: []string{"wallet.txt"},
		},
		{
			Name: "keys_cleared_message_on_error", // cleanExit path with -v
			Args: []string{"-v", "-type", "4", "-hdpath", "bad", "-l"},
			Exit: 1,
			Out:  []string{"Cleaning up 0 private keys"},
		},
		{
			Name: "list_unspendable_hint_uses_stderr", // sanity check that stderr is captured separately
			Args: []string{"-nosuchswitch"},
			Exit: 2,
			Check: func(r *Result) {
				if !strings.Contains(r.Stderr, "Gocoin Wallet version") {
					r.T.Error("logo expected on stderr")
				}
				if strings.Contains(r.Stdout, "Gocoin Wallet version") {
					r.T.Error("logo must not be on stdout")
				}
			},
		},
	})
}
