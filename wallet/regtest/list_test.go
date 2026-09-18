package regtest

import (
	"regexp"
	"strings"
	"testing"
)

// Testnet / Litecoin flavours of the first type-3 keys.
const (
	TestnetP2KH1   = "mumskSh3bXm7HaSU498KKkpAhSqkLzggeD"
	TestnetP2KH2   = "mw2q7tXecT4L8VVw14y2us51barkKzbHtB"
	TestnetBech1   = "tb1qn3jrfkqpps733jahyf2932ynnd2k7n2fh8tqv9"
	TestnetWIF1    = "cRpWRqwb3Ky2p6KRvjLgUuNcvBh7LdKs62aRjRLpH6ZTUQ8bJKEV"
	LitecoinP2KH1  = "LZUsibutsAZumGf1Wi9Emrfc3fcKXsm3FJ"
	LitecoinP2KH2  = "Lajq63kVt5s8cBiUTdyxMxvSwodKXJy668"
	noConfigNotice = "wallet.cfg not found - proceeding with the default config values."
)

// TestList covers key generation and the -l/-list output for all the
// wallet types, address types, HD paths and BIP39 modes.
func TestList(t *testing.T) {
	Run(t, []Case{
		{
			Name:        "type3_p2kh_default",
			Args:        []string{"-l", "-n", "4"},
			Out:         []string{noConfigNotice, "You can find all the addresses in wallet.txt file"},
			Golden:      "list_type3_p2kh",
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_p2kh"},
		},
		{
			Name: "list_long_switch_and_default_keycnt",
			Args: []string{"-list", "-q"},
			Check: func(r *Result) {
				lines := strings.Split(strings.TrimSpace(r.File("wallet.txt")), "\n")
				if len(lines) != 251 || lines[0] != "# Deterministic Walet Type 3" {
					r.T.Errorf("expected 250 addresses in wallet.txt, got %d lines", len(lines))
				}
				if lines[1] != P2KH[0]+" TypC 1" {
					r.T.Errorf("unexpected first address line: %s", lines[1])
				}
			},
		},
		{
			Name: "type3_300th_key", // vector from the original wallet_test.go
			Args: []string{"-l", "-q", "-n", "300"},
			Out:  []string{"1M8UbAaJ132nzgWQEhBxhydswWgHpASA2R TypC 300"},
		},
		{
			Name: "testnet_from_cfg",
			Cfg:  "testnet=true\nkeycnt=300\n",
			Args: []string{"-l", "-q"},
			Out:  []string{"Using config file wallet.cfg", TestnetP2KH1 + " TypC 1", "n1eRtDfGp4U3mnz1xGALXtrCoWGzhjrDDr TypC 300"},
		},
		{
			Name: "testnet_from_switch",
			Args: []string{"-l", "-t", "-n", "2"},
			Out:  []string{TestnetP2KH1 + " TypC 1", TestnetP2KH2 + " TypC 2"},
		},
		{
			Name:        "switch_overrides_cfg",
			Cfg:         "testnet=true\nkeycnt=100\natype=bech32\n",
			Args:        []string{"-l", "-t=false", "-n", "4", "-atype", "p2kh"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_p2kh"},
		},
		{
			Name: "litecoin",
			Args: []string{"-l", "-ltc", "-n", "2"},
			Out:  []string{LitecoinP2KH1 + " TypC 1", LitecoinP2KH2 + " TypC 2"},
		},
		{
			Name:        "atype_segwit",
			Cfg:         "atype=segwit\nkeycnt=4\n",
			Args:        []string{"-l"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_segwit"},
			Out:         []string{Segwit[0] + " TypC 1 (" + P2KH[0] + ")"},
		},
		{
			Name:        "atype_bech32",
			Args:        []string{"-l", "-atype", "bech32", "-n", "4"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_bech32"},
			Out:         []string{Bech32[3] + " TypC 4 (" + P2KH[3] + ")"},
		},
		{
			Name:        "atype_tap",
			Args:        []string{"-l", "-atype", "tap", "-n", "4"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_tap"},
			Out:         []string{Tap[0] + " TypC 1 (" + P2KH[0] + ")"},
		},
		{
			Name:        "atype_pks",
			Args:        []string{"-l", "-atype", "pks", "-n", "4"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_pks"},
			Out:         []string{Pubkey[1] + " TypC 2"},
		},
		{
			Name: "testnet_bech32",
			Args: []string{"-l", "-t", "-atype", "bech32", "-n", "1"},
			Out:  []string{TestnetBech1 + " TypC 1 (" + TestnetP2KH1 + ")"},
		},
		{
			Name: "verbose",
			Args: []string{"-l", "-v", "-n", "3"},
			Out:  []string{"Generating 3 keys, version 0 ...", "Private keys re-generated", "Cleaning up 3 private keys"},
		},

		// Type-4 (HD) wallets - vectors from the original wallet_test.go
		{
			Name:   "hd_m0h",
			Cfg:    "type=4\nkeycnt=20\n",
			Args:   []string{"-l"},
			Out:    []string{"15oN9iS7Ym5cAEZd2bPHw5ZfqtChnBoycb m/0'", "1FvWLNinb9RfQ4pFanWVMZJKq3DiB817X9 m/19'"},
			NotOut: []string{"# Root:"}, // hardened path: no xpub printed
			Golden: "list_hd_m0h",
		},
		{
			Name:   "hd_electrum_m00",
			Cfg:    "type=4\nkeycnt=20\nhdpath=m/0/0\n",
			Args:   []string{"-l"},
			Out:    []string{"13M4ypZeacDM2rZ62rqG8jZNg1LVRHhSGy m/0/19", "# Root: xpub", "# Prnt: xpub", "# Leaf: xpub"},
			Golden: "list_hd_electrum",
		},
		{
			Name: "hd_bip39_12_words",
			Cfg:  "type=4\nkeycnt=20\nhdpath=m/0/0\nbip39=12\n",
			Args: []string{"-l"},
			Out:  []string{"# Based on 12 BIP39 words", "1PP9HRai5dfWW8JByuP8jBEeu42b7AFRfR m/0/19"},
		},
		{
			Name: "hd_bip39_15_words",
			Args: []string{"-l", "-type", "4", "-n", "20", "-hdpath", "m/0/0", "-bip39", "15"},
			Out:  []string{"1DvsyQDNhnX1wSWBnaFfhaBCTMAiGAG6ig m/0/19"},
		},
		{
			Name: "hd_bip39_18_words",
			Args: []string{"-l", "-type", "4", "-n", "20", "-hdpath", "m/0/0", "-bip39", "18"},
			Out:  []string{"1BAkYsi4CzAjvgMBUe78QEVYhPJnmkNAyQ m/0/19"},
		},
		{
			Name: "hd_bip39_21_words",
			Args: []string{"-l", "-type", "4", "-n", "20", "-hdpath", "m/0/0", "-bip39", "21"},
			Out:  []string{"192TT86GEBkhRJUT6qD2YPgAVzKRU3f6V6 m/0/19"},
		},
		{
			Name: "hd_bip39_24_words",
			Args: []string{"-l", "-type", "4", "-n", "20", "-hdpath", "m/0/0", "-bip39", "24"},
			Out:  []string{"1JRQ1zkTSuWkmVFDtz9A9ErD9x4BNNAYmY m/0/19"},
		},
		{
			Name: "hd_bitcoin_core_path",
			Args: []string{"-l", "-type", "4", "-n", "20", "-hdpath", "m/0'/0'/0'", "-bip39", "12"},
			Out:  []string{"14iD1SLEFL9SHWoo8WrT9Wa6Mde3b2R79j m/0'/0'/19'"},
		},
		{
			Name: "hd_multibit_path",
			Args: []string{"-l", "-type", "4", "-n", "20", "-hdpath", "m/0'/0/0"},
			Out:  []string{"1HDTrCbonnRdN6dBmhEDmLstkDxTT6BEQM m/0'/0/19"},
		},
		{
			Name: "hd_bip44_path",
			Args: []string{"-l", "-type", "4", "-n", "20", "-hdpath", "m/44'/0'/0'/0"},
			Out:  []string{"1ABhTNjkFGquAo9Wq8yj2UirN65oUSiKWR m/44'/0'/0'/19"},
		},
		{
			Name:        "hd_bip84_bech32",
			Cfg:         "type=4\nkeycnt=10\nhdpath=m/84'/0'/0'/0/0\nbip39=12\natype=bech32\n",
			Args:        []string{"-l"},
			Out:         []string{"bc1qsh35g0djgwj7yw6evkhlkajke3twaua30ke3em m/84'/0'/0'/0/9 (1DCw8Gjgy3pfAh2NWQvgJEXHBjZvB4PAoD)", "# Prnt: zpub", "# Leaf: zpub"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_hd_bip84"},
		},
		{
			Name:        "hd_hdsubs_2",
			Cfg:         "type=4\nkeycnt=3\nhdpath=m/84'/0'/0'/0/0\nbip39=12\natype=bech32\nhdsubs=2\n",
			Args:        []string{"-l"},
			Out:         []string{"m/84'/0'/0'/0/2", "m/84'/0'/0'/1/0", "m/84'/0'/0'/1/2"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_hd_hdsubs2"},
			Check: func(r *Result) {
				if n := len(r.Lines("bc1q")); n != 6 {
					r.T.Errorf("expected 6 addresses (3 keys x 2 chains), got %d", n)
				}
			},
		},
		{
			Name: "hd_hdsubs_switch",
			Args: []string{"-l", "-type", "4", "-n", "2", "-hdpath", "m/0'/0/0", "-hdsubs", "3"},
			Out:  []string{"m/0'/0/1", "m/0'/1/1", "m/0'/2/1"},
		},

		// bip39=-1: the seed password is a BIP39 mnemonic (public test vectors)
		{
			Name:   "mnemonic_bip84_vector",
			Seed:   Mnemonic,
			Cfg:    "type=4\nbip39=-1\nhdpath=m/84'/0'/0'/0/0\natype=bech32\nkeycnt=2\n",
			Args:   []string{"-l"},
			Out:    []string{"The seed password is considered to be BIP39 mnemonic", MnemonicBip84Adr + " m/84'/0'/0'/0/0", "# Leaf: " + MnemonicBip84Zpub},
			Golden: "list_mnemonic_bip84",
		},
		{
			Name: "mnemonic_bip44_vector_xprv",
			Seed: Mnemonic,
			Args: []string{"-l", "-type", "4", "-bip39", "-1", "-hdpath", "m/44'/0'/0'/0/0", "-n", "2", "-xprv"},
			Out:  []string{"Root: " + MnemonicRootXprv, "Leaf: xprvA1Lvv1qpvx3f8iuRHfaEG45fyvDc3h7Ur5afz5SyRfkAsZ2765KfFfmg6Q9oEJDgf4UdYHphzzJybLykZfznUMKL2KNUU8pLRQgstN5kmFe", MnemonicBip44Adr + " m/44'/0'/0'/0/0"},
		},
		{
			Name: "mnemonic_seed_prefix_words", // leading words of the mnemonic kept in the config, the rest in .secret
			Seed: "abandon about",
			Cfg:  "type=4\nbip39=-1\nhdpath=m/44'/0'/0'/0/0\nkeycnt=1\nseed=abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon\n",
			Args: []string{"-l"},
			Out:  []string{MnemonicBip44Adr + " m/44'/0'/0'/0/0"},
		},
		{
			Name: "mnemonic_normalized", // case, punctuation and extra blanks are ignored
			Seed: "  Abandon, ABANDON abandon\tabandon abandon abandon abandon abandon abandon abandon abandon  about.\n",
			Args: []string{"-l", "-type", "4", "-bip39", "-1", "-hdpath", "m/44'/0'/0'/0/0", "-n", "1"},
			Out:  []string{MnemonicBip44Adr + " m/44'/0'/0'/0/0"},
		},
		{
			Name: "mnemonic_bad_checksum",
			Seed: "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon",
			Args: []string{"-l", "-type", "4", "-bip39", "-1", "-n", "1"},
			Exit: 1,
			Out:  []string{"checksum"},
		},
		{
			Name: "xprv_type4_default_path",
			Args: []string{"-type", "4", "-xprv"},
			Out:  []string{"Root: xprv", "Leaf: xprv"},
			Check: func(r *Result) {
				if r.Exists("wallet.txt") {
					r.T.Error("-xprv alone must not list the addresses")
				}
			},
		},
		{
			Name: "words_roundtrip", // words printed by -words must regenerate the same wallet in bip39=-1 mode
			Args: []string{"-type", "4", "-bip39", "12", "-words", "-l", "-n", "3", "-hdpath", "m/0/0"},
			Out:  []string{"===================== BIP39 mnemonic =====================", " 12: "},
			Check: func(r *Result) {
				re := regexp.MustCompile(`\d+: +([a-z]+)`)
				var words []string
				for _, m := range re.FindAllStringSubmatch(r.Stdout, -1) {
					words = append(words, m[1])
				}
				if len(words) != 12 {
					r.T.Fatalf("expected 12 words, got %d: %v", len(words), words)
				}
				r2 := runCase(r.T, &Case{
					Name: "words_roundtrip_regen",
					Seed: strings.Join(words, " "),
					Args: []string{"-type", "4", "-bip39", "-1", "-l", "-n", "3", "-hdpath", "m/0/0"},
				})
				if r2.Exit != 0 {
					r.T.Fatalf("regeneration failed:\n%s", r2.Both())
				}
				// only the "# Based on 12 BIP39 words" header line may differ
				if a, b := addressLines(r.File("wallet.txt")), addressLines(r2.File("wallet.txt")); a != b {
					r.T.Errorf("wallet regenerated from the mnemonic differs\n%s", diffLines(a, b))
				}
			},
		},
		{
			Name:        "scrypt",
			Cfg:         "scrypt=10\nkeycnt=2\n",
			Args:        []string{"-l"},
			Out:         []string{"Running scrypt function with complexity 1024 ... took"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_scrypt10"},
		},
		{
			Name: "scrypt_switch",
			Args: []string{"-l", "-scrypt", "10", "-n", "2", "-q"},
			Check: func(r *Result) {
				// same keys as the config file variant
				r2 := runCase(r.T, &Case{Cfg: "scrypt=10\nkeycnt=2\n", Args: []string{"-l"}})
				if r.File("wallet.txt") != r2.File("wallet.txt") {
					r.T.Error("-scrypt switch gives different keys than scrypt= in config")
				}
			},
		},
	})
}

// addressLines drops the comment lines from a wallet.txt content.
func addressLines(s string) string {
	var res []string
	for _, l := range strings.Split(s, "\n") {
		if !strings.HasPrefix(l, "#") {
			res = append(res, l)
		}
	}
	return strings.Join(res, "\n")
}

// TestSeedInput covers all the ways the seed password can reach the wallet.
func TestSeedInput(t *testing.T) {
	Run(t, []Case{
		{
			Name:        "seed_prefix_from_cfg",
			Cfg:         "seed=prefix-\nkeycnt=4\n",
			Args:        []string{"-l"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_seedprefix"},
		},
		{
			Name:        "seed_prefix_inline_equivalent",
			Seed:        "prefix-" + TestSeed,
			Args:        []string{"-l", "-n", "4"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_seedprefix"},
		},
		{
			Name:        "seed_prefix_ignored_with_is",
			Cfg:         "seed=prefix-\nkeycnt=4\n",
			Args:        []string{"-l", "-is"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_p2kh"},
		},
		{
			Name:        "seed_prefix_ignored_with_is_equal_form",
			Cfg:         "seed=prefix-\nkeycnt=4\n",
			Args:        []string{"-l", "--is=true"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_p2kh"},
		},
		{
			Name:        "secret_file_from_cfg",
			SeedFile:    "keys/my.secret",
			Cfg:         "secret=keys/my.secret\nkeycnt=4\n",
			Args:        []string{"-l"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_p2kh"},
		},
		{
			Name:        "password_from_stdin",
			NoSeed:      true,
			Stdin:       TestSeed,
			Args:        []string{"-l", "-stdin", "-n", "4"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_p2kh"},
			NotOut:      []string{"Enter your wallet's seed password"},
		},
		{
			Name:   "password_from_stdin_empty",
			NoSeed: true,
			Stdin:  "",
			Args:   []string{"-l", "-stdin"},
			Out:    []string{"empty seed password provided via stdin"},
		},
		{
			Name:   "seed_file_empty",
			NoSeed: true,
			Files:  map[string]string{".secret": ""},
			Args:   []string{"-l"},
			Out:    []string{"empty seed file .secret"},
			NotOut: []string{"TypC"},
		},
		{
			Name: "seed_too_long",
			Seed: strings.Repeat("x", 1025),
			Args: []string{"-l"},
			Out:  []string{"seed password provided is longer than 1024 bytes"},
		},
		{
			Name: "seed_max_length",
			Seed: strings.Repeat("x", 1024),
			Args: []string{"-l", "-n", "1"},
			Out:  []string{"TypC 1"},
		},
		{
			Name: "seed_nonprintable_warning",
			Seed: "abc\x01def",
			Args: []string{"-l", "-n", "1"},
			Out:  []string{"WARNING: Your secret contains non-printable characters", "TypC 1"},
		},
	})
}

// TestConfig covers locating the config file and parsing its values.
func TestConfig(t *testing.T) {
	Run(t, []Case{
		{
			Name:        "cfg_switch",
			CfgFile:     "custom.cfg",
			Cfg:         "keycnt=4\natype=bech32\n",
			Args:        []string{"-cfg", "custom.cfg", "-l"},
			Out:         []string{"Using config file custom.cfg"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_bech32"},
		},
		{
			Name:        "cfg_switch_with_equal_sign",
			CfgFile:     "custom.cfg",
			Cfg:         "keycnt=4\natype=bech32\n",
			Args:        []string{"-l", "--cfg=custom.cfg"},
			Out:         []string{"Using config file custom.cfg"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_bech32"},
		},
		{
			Name:        "cfg_from_env",
			CfgFile:     "sub/env.cfg",
			Cfg:         "keycnt=4\natype=segwit\n",
			Env:         map[string]string{"GOCOIN_WALLET_CONFIG": "sub/env.cfg"},
			Args:        []string{"-l"},
			Out:         []string{"Using config file sub/env.cfg"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_segwit"},
		},
		{
			Name:        "cfg_switch_overrides_env",
			CfgFile:     "custom.cfg",
			Cfg:         "keycnt=4\n",
			Files:       map[string]string{"env.cfg": "keycnt=1\ntestnet=true\n"},
			Env:         map[string]string{"GOCOIN_WALLET_CONFIG": "env.cfg"},
			Args:        []string{"-cfg", "custom.cfg", "-l"},
			Out:         []string{"Using config file custom.cfg"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_p2kh"},
		},
		{
			Name: "cfg_missing_file",
			Args: []string{"-cfg", "nonexistent.cfg", "-l", "-n", "1"},
			Out:  []string{"nonexistent.cfg not found - proceeding with the default config values.", P2KH[0] + " TypC 1"},
		},
		{
			Name: "cfg_switch_without_filename",
			Args: []string{"-l", "-cfg"},
			Exit: 1,
			Out:  []string{"Missing the file name for -cfg argument"},
		},
		{
			Name:        "cfg_comments_blanks_and_case", // keys are case insensitive, quotes around hdpath/atype are stripped
			Cfg:         "# comment\n\n  KEYCNT = 4 \nType=4\nHdPath=\"m/84'/0'/0'/0/0\"\nBIP39=12\nAtype=\"bech32\"\n",
			Args:        []string{"-l"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_hd_bip84_4keys"},
		},
		{
			Name: "cfg_syntax_error_is_not_fatal",
			Cfg:  "this line has no equal sign\nkeycnt=1\n",
			Args: []string{"-l"},
			Out:  []string{"wallet.cfg: syntax error in line", P2KH[0] + " TypC 1"},
		},
		{
			Name: "cfg_unknown_key_ignored",
			Cfg:  "whatever=42\nkeycnt=1\n",
			Args: []string{"-l"},
			Out:  []string{P2KH[0] + " TypC 1"},
		},
		{
			Name: "cfg_bool_value_error",
			Cfg:  "testnet=maybe\n",
			Args: []string{"-l"},
			Exit: 1,
			Out:  []string{"wallet.cfg: value error for testnet"},
		},
		{
			Name: "cfg_type_out_of_range",
			Cfg:  "type=5\n",
			Args: []string{"-l"},
			Exit: 1,
			Out:  []string{"wallet.cfg: incorrect wallet type 5"},
		},
		{
			Name: "cfg_bip39_bad_value",
			Cfg:  "type=4\nbip39=13\n",
			Args: []string{"-l"},
			Exit: 1,
			Out:  []string{"wallet.cfg: incorrect bip39 value 13"},
		},
		{
			Name: "cfg_keycnt_zero",
			Cfg:  "keycnt=0\n",
			Args: []string{"-l"},
			Exit: 1,
			Out:  []string{"wallet.cfg: incorrect key count 0"},
		},
		{
			Name: "cfg_hdsubs_zero",
			Cfg:  "hdsubs=0\n",
			Args: []string{"-l"},
			Exit: 1,
			Out:  []string{"wallet.cfg: incorrect hdsubs value 0"},
		},
		{
			Name: "cfg_scrypt_zero",
			Cfg:  "scrypt=0\n",
			Args: []string{"-l"},
			Exit: 1,
			Out:  []string{"wallet.cfg: incorrect scrypt value 0"},
		},
		{
			Name: "cfg_minsig_ignored_with_rfc6979", // used to loop forever
			Cfg:  "rfc6979=true\nminsig=true\n",
			Balance: []Utxo{
				{Tx: 1, Addr: P2KH[0], Value: 100000000},
			},
			Args: []string{"-send", ForeignP2KH + "=0.5", "-txfn", "out.txt"},
			Out:  []string{"TxID " + TxidSendSimple2},
			Check: func(r *Result) {
				r.VerifyTx("out.txt")
				r2 := runCase(r.T, &Case{Cfg: "rfc6979=true\n", Balance: []Utxo{{Tx: 1, Addr: P2KH[0], Value: 100000000}},
					Args: []string{"-send", ForeignP2KH + "=0.5", "-txfn", "out.txt"}})
				if r2.File("out.txt") != r.File("out.txt") {
					r.T.Error("minsig must not change RFC6979 signatures")
				}
			},
		},
		{
			Name: "cfg_rfc6979_fee_apply2bal",
			Cfg:  "rfc6979=true\nfee=0.0005\napply2bal=false\n",
			Balance: []Utxo{
				{Tx: 1, Addr: P2KH[0], Value: 100000000},
			},
			Args: []string{"-send", ForeignP2KH + "=0.5", "-txfn", "out.txt"},
			Out:  []string{"Transaction data stored in out.txt"},
			Check: func(r *Result) {
				tx := r.VerifyTx("out.txt")
				r.ExpectOutputs(tx, false, ForeignP2KH+"=50000000", P2KH[0]+"=49950000") // 0.0005 fee from cfg
				if r.Exists("balance/" + tx.Hash.String() + ".tx") {
					r.T.Error("apply2bal=false must not touch the balance folder")
				}
			},
		},
	})
}
