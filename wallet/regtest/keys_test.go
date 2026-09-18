package regtest

import (
	"strings"
	"testing"
)

// Deterministic (RFC6979) signatures produced by the wallet for fixed inputs.
const (
	MsgSigKey1     = "H+rDfG7c18sKb9eQ2ga2/aVZMz0azjJrbDvLOekGF5Asdx6uN8daCyLaEd4HPYuftjzyeEVdNDq6UBylNvWIwaw="
	MsgSigOtherUnc = "G0vpc2cG8b1gyl0JED175XSp//+a66T5u4QQ1zEbdaYoLcShN4mUMInxUKJeitJgsGCK05gVsyseHQXzc4/fItc="
	MsgSigLTCKey1  = "H6TPsIuIGznpM2QjxgaH0pUtuL0x2DALhJq5SUl4zWGbBRBKNc0yjecbXmKnHh6pO1JKMm7Db/Zs7cLkI6EKaLA="
	HashSigKey1    = "304402206a4da4df5c997de31edfd197f0a1c334c8c4c3c1595ba256e1a77370b69b789c0220360b528d8d2967f049a2ba81e91c5a04e8f20f7e0c3505ba39a0d35b04d5f48d01"
)

const testHash = "0000000000000000000000000000000000000000000000000000000000000001"

func TestOthers(t *testing.T) {
	Run(t, []Case{
		{
			Name:   "import_compressed_with_label",
			Others: OtherWifCompressed + " my imported key\n",
			Args:   []string{"-l", "-n", "1"},
			Out:    []string{OtherAdrCompressed + " my imported key", P2KH[0] + " TypC 1"},
			Check: func(r *Result) {
				if !strings.HasPrefix(r.File("wallet.txt"), "# Deterministic Walet Type 3\n"+OtherAdrCompressed+" my imported key\n") {
					r.T.Error("imported keys must come first in wallet.txt")
				}
			},
		},
		{
			Name:   "import_uncompressed_default_label",
			Others: OtherWifUncompressed + "\n",
			Args:   []string{"-l", "-n", "1", "-v"},
			Out:    []string{OtherAdrUncompressed + " Other 0", "1 keys imported from .others"},
		},
		{
			Name:   "import_uncompressed_no_segwit_address",
			Others: OtherWifUncompressed + "\n",
			Args:   []string{"-l", "-n", "1", "-atype", "bech32"},
			Out:    []string{"-=CompressedKey=- Other 0 (" + OtherAdrUncompressed + ")", Bech32[0] + " TypC 1"},
		},
		{
			Name:   "import_comments_and_blank_lines",
			Others: "# a comment\n\n   " + OtherWifCompressed + "   \n# another\n" + OtherWifUncompressed + " second\n",
			Args:   []string{"-l", "-n", "1"},
			Out:    []string{OtherAdrCompressed + " Other 0", OtherAdrUncompressed + " second"},
		},
		{
			Name:   "import_bad_key_skipped",
			Others: "notavalidkey\n" + OtherWifCompressed + "\n",
			Args:   []string{"-l", "-n", "1", "-v"},
			Out:    []string{"DecodePrivateAddr error:", "notavalidkey", OtherAdrCompressed + " Other 0"},
		},
		{
			Name:   "import_wrong_network_warning",
			Others: TestnetWIF1 + "\n",
			Args:   []string{"-l", "-n", "1"},
			Out:    []string{"has version 239 while we expect 128", "You may want to play with -t or -ltc switch"},
		},
		{
			Name:   "import_testnet_key",
			Others: TestnetWIF1 + " tkey\n",
			Args:   []string{"-l", "-n", "1", "-t"},
			Out:    []string{TestnetP2KH1 + " tkey", TestnetP2KH1 + " TypC 1"},
			NotOut: []string{"has version"},
		},
		{
			Name:  "others_file_from_cfg",
			Files: map[string]string{"keys/extra.txt": OtherWifCompressed + " cfg-others\n"},
			Cfg:   "others=keys/extra.txt\nkeycnt=1\n",
			Args:  []string{"-l"},
			Out:   []string{OtherAdrCompressed + " cfg-others"},
		},
	})
}

func TestDump(t *testing.T) {
	Run(t, []Case{
		{
			Name: "dump_by_p2kh",
			Args: []string{"-dump", P2KH[1], "-n", "4"},
			Out: []string{
				"Public address: " + P2KH[1] + " TypC 2",
				"Public hexdump: " + Pubkey[1],
				"Public compressed: true",
				"Private encoded: " + WIF[1],
				"Private hexdump: ",
			},
			Golden: "dump_key2",
		},
		{
			Name: "dump_by_segwit_address",
			Args: []string{"-dump", Segwit[2], "-n", "4"},
			Out:  []string{"Private encoded: " + WIF[2]},
		},
		{
			Name: "dump_by_bech32_address",
			Args: []string{"-dump", Bech32[3], "-n", "4"},
			Out:  []string{"Private encoded: " + WIF[3]},
		},
		{
			Name: "dump_by_taproot_address",
			Args: []string{"-dump", Tap[0], "-n", "4"},
			Out:  []string{"Private encoded: " + WIF[0]},
		},
		{
			Name:   "dump_all",
			Args:   []string{"-dump", "*", "-n", "4"},
			Golden: "dump_all",
			Out:    []string{WIF[3] + " " + P2KH[3] + " TypC 4"},
		},
		{
			Name:   "dump_all_with_others",
			Others: OtherWifCompressed + " lab\n",
			Args:   []string{"-dump", "*", "-n", "1"},
			Out:    []string{OtherWifCompressed + " " + OtherAdrCompressed + " lab", WIF[0] + " " + P2KH[0] + " TypC 1"},
		},
		{
			Name:   "dump_uncompressed_other",
			Others: OtherWifUncompressed + "\n",
			Args:   []string{"-dump", OtherAdrUncompressed, "-n", "1"},
			Out:    []string{"Public compressed: false", "Private encoded: " + OtherWifUncompressed},
		},
		{
			Name: "dump_testnet",
			Args: []string{"-dump", TestnetP2KH1, "-n", "1", "-t"},
			Out:  []string{"Private encoded: " + TestnetWIF1},
		},
		{
			Name:   "dump_not_in_wallet",
			Args:   []string{"-dump", ForeignP2KH, "-n", "2"},
			Out:    []string{"Dump Private Key: " + ForeignP2KH + " not found it the wallet"},
			NotOut: []string{"Private encoded"},
		},
		{
			Name: "dump_bad_address",
			Args: []string{"-dump", "notanaddress", "-n", "1"},
			Exit: 1,
			Out:  []string{"Cannot Decode address notanaddress"},
		},
		{
			Name: "pub_p2kh",
			Args: []string{"-pub", P2KH[2], "-n", "4"},
			Out:  []string{"Public address: " + P2KH[2], "Public hexdump: " + Pubkey[2]},
		},
		{
			Name: "pub_bech32_mode",
			Args: []string{"-pub", Bech32[2], "-n", "4", "-atype", "bech32"},
			Out:  []string{"Public address: " + Bech32[2], "Public hexdump: " + Pubkey[2]},
		},
		{
			Name: "pub_segwit_addr_in_p2kh_mode",
			Args: []string{"-pub", Segwit[2], "-n", "4"},
			Out:  []string{"Public address: " + P2KH[2], "Public hexdump: " + Pubkey[2]},
		},
		{
			Name:   "pub_unknown",
			Args:   []string{"-pub", ForeignP2KH, "-n", "2"},
			NotOut: []string{"Public hexdump"},
		},
	})
}

func TestSignMessage(t *testing.T) {
	Run(t, []Case{
		{
			Name: "sign_msg_rfc6979",
			Args: []string{"-sign", P2KH[0], "-msg", "hello gocoin", "-rfc6979", "-n", "1"},
			Out:  []string{MsgSigKey1},
			Check: func(r *Result) {
				VerifyMessageSig(r.T, P2KH[0], "hello gocoin", lastLine(r.Stdout))
			},
		},
		{
			Name: "sign_msg_rfc6979_from_cfg",
			Cfg:  "rfc6979=true\nkeycnt=1\n",
			Args: []string{"-sign", P2KH[0], "-msg", "hello gocoin"},
			Out:  []string{MsgSigKey1},
		},
		{
			Name: "sign_msg_random_nonce",
			Args: []string{"-sign", P2KH[1], "-msg", "random nonce", "-n", "2"},
			Check: func(r *Result) {
				sig := lastLine(r.Stdout)
				if !b64(sig) {
					r.T.Fatalf("no signature on the last line: %q", sig)
				}
				VerifyMessageSig(r.T, P2KH[1], "random nonce", sig)
			},
		},
		{
			Name: "sign_msg_by_bech32_address",
			Args: []string{"-sign", Bech32[0], "-msg", "hello gocoin", "-rfc6979", "-n", "1"},
			Out:  []string{MsgSigKey1},
		},
		{
			Name:  "sign_msg_from_stdin",
			Stdin: "hello gocoin",
			Args:  []string{"-sign", P2KH[0], "-rfc6979", "-n", "1"},
			Out:   []string{MsgSigKey1},
		},
		{
			Name:   "sign_msg_uncompressed_key",
			Others: OtherWifUncompressed + "\n",
			Args:   []string{"-sign", OtherAdrUncompressed, "-msg", "uncompressed", "-rfc6979", "-n", "1"},
			Out:    []string{MsgSigOtherUnc},
			Check: func(r *Result) {
				sig := lastLine(r.Stdout)
				if !strings.HasPrefix(sig, "G") { // header byte 27/28 - uncompressed
					r.T.Errorf("uncompressed key signature should start with G: %s", sig)
				}
				VerifyMessageSig(r.T, OtherAdrUncompressed, "uncompressed", sig)
			},
		},
		{
			Name: "sign_msg_litecoin",
			Args: []string{"-sign", LitecoinP2KH1, "-msg", "hello gocoin", "-rfc6979", "-ltc", "-n", "1"},
			Out:  []string{MsgSigLTCKey1},
		},
		{
			Name: "sign_msg_testnet",
			Args: []string{"-sign", TestnetP2KH1, "-msg", "hello gocoin", "-rfc6979", "-t", "-n", "1"},
			Out:  []string{MsgSigKey1}, // same key, same hash - network does not matter
		},
		{
			Name:   "sign_msg_unknown_address",
			Args:   []string{"-sign", ForeignP2KH, "-msg", "x", "-n", "1"},
			Out:    []string{"You do not have a private key for " + ForeignP2KH},
			NotOut: []string{"="},
		},
		{
			Name: "sign_hash_rfc6979",
			Args: []string{"-sign", P2KH[0], "-hash", testHash, "-rfc6979", "-n", "1"},
			Out:  []string{"PublicKey: " + Pubkey[0], HashSigKey1},
		},
		{
			Name: "sign_hash_bad_hex",
			Args: []string{"-sign", P2KH[0], "-hash", "zz", "-n", "1"},
			Out:  []string{"Incorrect content of -hash parameter"},
		},
		{
			Name: "sign_msg_then_send", // -sign together with -send continues to spending
			Balance: []Utxo{
				{Tx: 1, Addr: P2KH[0], Value: 100000000},
			},
			Args: []string{"-sign", P2KH[0], "-msg", "hello gocoin", "-rfc6979", "-n", "1", "-send", ForeignP2KH + "=0.1", "-txfn", "tx.txt"},
			Out:  []string{MsgSigKey1, "Transaction data stored in tx.txt"},
			Check: func(r *Result) {
				r.VerifyTx("tx.txt")
			},
		},
	})
}
