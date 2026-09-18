package regtest

import (
	"strings"
	"testing"

	"github.com/piotrnar/gocoin/lib/btc"
)

// Expected IDs of transactions signed with RFC6979 (deterministic) signatures.
const (
	TxidSendSimple    = "3cbf46e489718735b6ab4f95b5ee3a2d7d44dcff4584cf4479ac3382559722ff"
	TxidSendMixed     = "2c076d2d8ac3fc6430ccc81aa5425fe2074cd3eaec219a55bdfecfa2c7692e2b"
	TxidRawSigned     = "6464fd4a7907a8aee0386b8583e2a7345d5c004e507c62f55c4aa1d1dfa570ee"
	TxidMultisigDone  = "f46cd2f5f01035f3b5e1ddcbdeabb0b78d3889e75f7340e230dcf221d1560935"
	TxidMultisigInWal = "87f69a262e23e73d414e3056e3361218d2fa30664690f59bc87db743af322766"
	TxidSendSimple2   = "dce363e9f16ca5ae9ef7c52da58e2976692d06032f6728c2efb4ca27c4c2ef56"
)

// balanceBasic: 2.0 BTC in four outputs of all the single-key types.
var balanceBasic = []Utxo{
	{Tx: 1, Addr: P2KH[0], Value: 100000000, Label: "# 1 BTC @ key1"},
	{Tx: 1, Addr: P2KH[1], Value: 50000000},
	{Tx: 2, Addr: Segwit[2], Value: 30000000, Label: "p2sh-segwit"},
	{Tx: 3, Addr: Bech32[3], Value: 20000000, Label: "native segwit"},
}

var balanceP2KH = []Utxo{
	{Tx: 1, Addr: P2KH[0], Value: 100000000},
}

func rfcArgs(a ...string) []string {
	return append([]string{"-rfc6979", "-n", "4"}, a...)
}

func TestBalance(t *testing.T) {
	_, msAddr := multisig2of3(t)
	Run(t, []Case{
		{
			Name:    "show_balance",
			Balance: balanceBasic,
			Args:    []string{"-n", "4"},
			Out:     []string{"You have 2.00000000 BTC in 4 keyhash outputs"},
			NotOut:  []string{"multisig outputs", "unspendable"},
		},
		{
			Name: "show_balance_with_multisig_and_unknown",
			Balance: append(balanceBasic,
				Utxo{Tx: 4, Addr: msAddr, Value: 70000000},
				Utxo{Tx: 5, Addr: ForeignP2KH, Value: 10000000},
				Utxo{Tx: 5, Addr: P2KH[3], Value: 1, Skip: true},
			),
			Args: []string{"-n", "4", "-v"},
			Out: []string{
				"You have 2.00000000 BTC in 4 keyhash outputs",
				"There is 0.70000000 BTC in 1 multisig outputs",
				"WARNING: 1 unspendable inputs (-v to print them).",
				"WARNING: Don't know how to sign",
			},
		},
		{
			Name: "balance_only_keys_we_own_count",
			Balance: []Utxo{
				{Tx: 1, Addr: P2KH[0], Value: 100000000},
				{Tx: 1, Addr: P2KH[3], Value: 100000000}, // key #4 is not generated with -n 2
			},
			Args: []string{"-n", "2"},
			Out:  []string{"You have 1.00000000 BTC in 1 keyhash outputs", "WARNING: 1 unspendable inputs"},
		},
		{
			Name: "balance_taproot_outputs",
			Balance: []Utxo{
				{Tx: 1, Addr: Tap[0], Value: 100000000},
			},
			Args: []string{"-n", "1"},
			Out:  []string{"You have 1.00000000 BTC in 1 keyhash outputs"},
		},
		{
			Name: "balance_testnet",
			Balance: []Utxo{
				{Tx: 1, Addr: TestnetP2KH1, Value: 12345678},
				{Tx: 1, Addr: TestnetBech1, Value: 1},
			},
			Args: []string{"-n", "1", "-t"},
			Out:  []string{"You have 0.12345679 BTC in 2 keyhash outputs"},
		},
		{
			Name: "balance_folder_missing",
			Args: []string{"-n", "1"},
			Exit: 1,
			Out:  []string{"Failed to load wallet's balance data"},
		},
		{
			Name:    "balance_bad_unspent_line",
			Balance: balanceP2KH,
			Files:   map[string]string{"balance/unspent.txt": "garbage line\n"},
			Args:    []string{"-n", "1"},
			Out:     []string{"ERROR in unspent.txt:  garbage line", "You have 0.00000000 BTC in 0 keyhash outputs"},
		},
		{
			Name:    "balance_missing_tx_file",
			Balance: balanceP2KH,
			Files:   map[string]string{"balance/unspent.txt": strings.Repeat("00", 32) + "-0\n"},
			Args:    []string{"-n", "1"},
			Exit:    1,
			Out:     []string{"Error reading transaction file: balance/" + strings.Repeat("00", 32) + ".tx"},
		},
	})
}

func TestSend(t *testing.T) {
	Run(t, []Case{
		{
			Name:    "send_simple",
			Balance: balanceP2KH,
			Args:    rfcArgs("-send", ForeignP2KH+"=0.3", "-txfn", "tx.txt"),
			Out:     []string{"TxID " + TxidSendSimple, "Transaction data stored in tx.txt", "Adding 2 new output(s) to the balance/ folder..."},
			Check: func(r *Result) {
				tx := r.TxID("tx.txt", TxidSendSimple)
				r.VerifyTx("tx.txt")
				r.ExpectOutputs(tx, false, ForeignP2KH+"=30000000", P2KH[0]+"=69990000") // change - default fee 0.0001
				if tx.Version != 2 || tx.Lock_time != 0 || tx.TxIn[0].Sequence != 0xfffffffd {
					r.T.Errorf("unexpected tx fields: ver=%d lock=%d seq=%x", tx.Version, tx.Lock_time, tx.TxIn[0].Sequence)
				}
				// apply2bal: the spent output is gone, the change output is now in unspent.txt
				want := TxidSendSimple + "-001 # 0.69990000 BTC @ " + P2KH[0] + "\n"
				if got := r.File("balance/unspent.txt"); got != want {
					r.T.Errorf("unspent.txt after send:\n%s", diffLines(want, got))
				}
				if !r.Exists("balance/" + TxidSendSimple + ".tx") {
					r.T.Error("the new transaction was not stored in balance/")
				}
			},
		},
		{
			Name:       "send_random_txfn",
			Balance:    balanceP2KH,
			Args:       rfcArgs("-send", ForeignP2KH+"=0.3"),
			FileExists: []string{TxidSendSimple[:8] + ".txt"},
		},
		{
			Name:    "send_no_apply2bal",
			Balance: balanceP2KH,
			Args:    rfcArgs("-send", ForeignP2KH+"=0.3", "-txfn", "tx.txt", "-a=false"),
			NotOut:  []string{"Adding 2 new output(s)"},
			Check: func(r *Result) {
				r.VerifyTx("tx.txt")
				if !strings.HasPrefix(r.File("balance/unspent.txt"), fundingTx(r.T, 1, balanceP2KH).Hash.String()+"-0") {
					r.T.Error("unspent.txt must stay untouched with -a=false")
				}
				if r.Exists("balance/" + TxidSendSimple + ".tx") {
					r.T.Error("balance/ must stay untouched with -a=false")
				}
			},
		},
		{
			Name:    "send_mixed_inputs_all_types", // p2kh + p2sh-segwit + p2wpkh inputs in one tx
			Balance: balanceBasic,
			Args:    rfcArgs("-send", ForeignBech32+"=1.95", "-txfn", "tx.txt", "-v"),
			Out:     []string{"TxID " + TxidSendMixed, "Spending 4 out of 4 outputs..."},
			Check: func(r *Result) {
				tx := r.TxID("tx.txt", TxidSendMixed)
				r.VerifyTx("tx.txt")
				r.ExpectOutputs(tx, false, ForeignBech32+"=195000000", P2KH[0]+"=4990000")
				if tx.SegWit == nil {
					r.T.Error("expected a segwit transaction")
				}
			},
		},
		{
			Name: "send_taproot_input",
			Balance: []Utxo{
				{Tx: 1, Addr: Tap[1], Value: 100000000},
			},
			Args: rfcArgs("-send", ForeignTap+"=0.5", "-txfn", "tx.txt"),
			Check: func(r *Result) {
				tx := r.VerifyTx("tx.txt") // schnorr signatures are randomized: verify only
				r.ExpectOutputs(tx, false, ForeignTap+"=50000000", Tap[1]+"=49990000")
			},
		},
		{
			Name: "send_change_goes_to_first_input_type",
			Balance: []Utxo{
				{Tx: 1, Addr: Bech32[0], Value: 100000000},
				{Tx: 1, Addr: P2KH[1], Value: 100000000},
			},
			Args: rfcArgs("-send", ForeignP2KH+"=1.5", "-txfn", "tx.txt"),
			Check: func(r *Result) {
				tx := r.VerifyTx("tx.txt")
				r.ExpectOutputs(tx, false, ForeignP2KH+"=150000000", Bech32[0]+"=49990000")
			},
		},
		{
			Name:    "send_explicit_change_address",
			Balance: balanceP2KH,
			Args:    rfcArgs("-send", ForeignP2KH+"=0.3", "-change", ForeignP2SH, "-txfn", "tx.txt"),
			Check: func(r *Result) {
				tx := r.VerifyTx("tx.txt")
				r.ExpectOutputs(tx, false, ForeignP2KH+"=30000000", ForeignP2SH+"=69990000")
				if strings.TrimSpace(r.File("balance/unspent.txt")) != "" {
					r.T.Error("no output belongs to the wallet - unspent.txt should be empty")
				}
			},
		},
		{
			Name:    "send_multiple_outputs",
			Balance: balanceP2KH,
			Args:    rfcArgs("-send", ForeignP2KH+"=0.1, "+ForeignP2SH+"=0.2 ,"+ForeignBech32+"=0.3,"+ForeignTap+"=0.3", "-txfn", "tx.txt"),
			Check: func(r *Result) {
				tx := r.VerifyTx("tx.txt")
				r.ExpectOutputs(tx, false, ForeignP2KH+"=10000000", ForeignP2SH+"=20000000", ForeignBech32+"=30000000", ForeignTap+"=30000000", P2KH[0]+"=9990000")
			},
		},
		{
			Name:    "send_exact_amount_no_change",
			Balance: balanceP2KH,
			Args:    rfcArgs("-send", ForeignP2KH+"=0.9999", "-txfn", "tx.txt"),
			Check: func(r *Result) {
				tx := r.VerifyTx("tx.txt")
				r.ExpectOutputs(tx, false, ForeignP2KH+"=99990000")
			},
		},
		{
			Name:    "send_custom_fee",
			Balance: balanceP2KH,
			Args:    rfcArgs("-send", ForeignP2KH+"=0.5", "-fee", "0.00123", "-txfn", "tx.txt"),
			Check: func(r *Result) {
				tx := r.VerifyTx("tx.txt")
				r.ExpectOutputs(tx, false, ForeignP2KH+"=50000000", P2KH[0]+"=49877000")
			},
		},
		{
			Name:    "send_subtract_fee_from_first_output",
			Balance: balanceP2KH,
			Args:    rfcArgs("-send", ForeignP2KH+"=0.5,"+ForeignP2SH+"=0.2", "-f", "-txfn", "tx.txt"),
			Check: func(r *Result) {
				tx := r.VerifyTx("tx.txt")
				r.ExpectOutputs(tx, false, ForeignP2KH+"=49990000", ForeignP2SH+"=20000000", P2KH[0]+"=30000000")
			},
		},
		{
			Name:    "send_with_op_return_message",
			Balance: balanceP2KH,
			Args:    rfcArgs("-send", ForeignP2KH+"=0.5", "-msg", "hello chain", "-txfn", "tx.txt"),
			Check: func(r *Result) {
				tx := r.VerifyTx("tx.txt")
				r.ExpectOutputs(tx, false, ForeignP2KH+"=50000000", P2KH[0]+"=49990000", "OP_RETURN=hello chain")
			},
		},
		{
			Name:    "send_useallinputs",
			Balance: balanceBasic,
			Args:    rfcArgs("-send", ForeignP2KH+"=0.1", "-useallinputs", "-txfn", "tx.txt"),
			Check: func(r *Result) {
				tx := r.VerifyTx("tx.txt")
				if len(tx.TxIn) != 4 {
					r.T.Errorf("expected 4 inputs, got %d", len(tx.TxIn))
				}
				r.ExpectOutputs(tx, false, ForeignP2KH+"=10000000", P2KH[0]+"=189990000")
			},
		},
		{
			Name:    "send_minimal_inputs",
			Balance: balanceBasic,
			Args:    rfcArgs("-send", ForeignP2KH+"=1.2", "-txfn", "tx.txt"),
			Check: func(r *Result) {
				tx := r.VerifyTx("tx.txt")
				if len(tx.TxIn) != 2 {
					r.T.Errorf("expected 2 inputs, got %d", len(tx.TxIn))
				}
			},
		},
		{
			Name:    "send_locktime_version_sequence",
			Balance: balanceP2KH,
			Args:    rfcArgs("-send", ForeignP2KH+"=0.5", "-locktime", "700000", "-txver", "1", "-seq", "-1", "-txfn", "tx.txt"),
			Check: func(r *Result) {
				tx := r.VerifyTx("tx.txt")
				if tx.Version != 1 || tx.Lock_time != 700000 || tx.TxIn[0].Sequence != 0xffffffff {
					r.T.Errorf("unexpected tx fields: ver=%d lock=%d seq=%x", tx.Version, tx.Lock_time, tx.TxIn[0].Sequence)
				}
			},
		},
		{
			Name:    "send_rbf_sequence",
			Balance: balanceP2KH,
			Args:    rfcArgs("-send", ForeignP2KH+"=0.5", "-seq", "1", "-txfn", "tx.txt"),
			Check: func(r *Result) {
				if tx := r.VerifyTx("tx.txt"); tx.TxIn[0].Sequence != 1 {
					r.T.Errorf("sequence %x", tx.TxIn[0].Sequence)
				}
			},
		},
		{
			Name:    "send_batch_file",
			Balance: balanceP2KH,
			Files:   map[string]string{"batch.txt": "# comment=with equal sign\n" + ForeignP2KH + "=0.1\n  " + ForeignBech32 + "=0.2  \n"},
			Args:    rfcArgs("-batch", "batch.txt", "-txfn", "tx.txt"),
			Check: func(r *Result) {
				tx := r.VerifyTx("tx.txt")
				r.ExpectOutputs(tx, false, ForeignP2KH+"=10000000", ForeignBech32+"=20000000", P2KH[0]+"=69990000")
			},
		},
		{
			Name:    "send_and_batch_combined",
			Balance: balanceP2KH,
			Files:   map[string]string{"batch.txt": ForeignBech32 + "=0.2\n"},
			Args:    rfcArgs("-send", ForeignP2KH+"=0.1", "-batch", "batch.txt", "-txfn", "tx.txt"),
			Check: func(r *Result) {
				tx := r.VerifyTx("tx.txt")
				r.ExpectOutputs(tx, false, ForeignP2KH+"=10000000", ForeignBech32+"=20000000", P2KH[0]+"=69990000")
			},
		},
		{
			Name:    "send_batch_comment_line",
			Balance: balanceP2KH,
			Files:   map[string]string{"batch.txt": "# comment\n\n   \n" + ForeignP2KH + "=0.1\n"},
			Args:    rfcArgs("-batch", "batch.txt", "-txfn", "tx.txt"),
			Check: func(r *Result) {
				r.ExpectOutputs(r.VerifyTx("tx.txt"), false, ForeignP2KH+"=10000000", P2KH[0]+"=89990000")
			},
		},
		{
			Name:    "send_batch_missing_file",
			Balance: balanceP2KH,
			Args:    rfcArgs("-batch", "nope.txt"),
			Exit:    1,
			Out:     []string{"nope.txt"},
		},
		{
			Name:    "send_batch_bad_line",
			Balance: balanceP2KH,
			Files:   map[string]string{"batch.txt": ForeignP2KH + "=0.1\nbroken\n"},
			Args:    rfcArgs("-batch", "batch.txt"),
			Exit:    1,
			Out:     []string{"Error in the batch file line 2"},
		},
		{
			Name: "send_testnet",
			Balance: []Utxo{
				{Tx: 1, Addr: TestnetBech1, Value: 100000000},
			},
			Args: rfcArgs("-t", "-send", ForeignTBech+"=0.4,"+ForeignTest+"=0.1", "-txfn", "tx.txt"),
			Check: func(r *Result) {
				tx := r.VerifyTx("tx.txt")
				r.ExpectOutputs(tx, true, ForeignTBech+"=40000000", ForeignTest+"=10000000", TestnetBech1+"=49990000")
			},
		},
		{
			Name:    "send_minsig_random_nonce", // without RFC6979 every run differs, but sigs must stay short and valid
			Balance: balanceBasic,
			Args:    []string{"-n", "4", "-minsig", "-send", ForeignP2KH + "=1.9", "-txfn", "tx.txt", "-useallinputs"},
			Check: func(r *Result) {
				tx := r.VerifyTx("tx.txt")
				for i := range tx.TxIn {
					var sig []byte
					if tx.SegWit != nil && len(tx.SegWit[i]) > 0 {
						sig = tx.SegWit[i][0]
					} else {
						sig = tx.TxIn[i].ScriptSig[1 : 1+int(tx.TxIn[i].ScriptSig[0])]
					}
					if len(sig) > 71 {
						r.T.Errorf("input %d: signature is %d bytes long", i, len(sig))
					}
				}
			},
		},

		// errors
		{
			Name:        "send_insufficient_funds",
			Balance:     balanceP2KH,
			Args:        rfcArgs("-send", ForeignP2KH+"=1.0"),
			Exit:        1,
			Out:         []string{"ERROR: You have 1.00000000 BTC, but you need 1.00010000 BTC for the transaction"},
			FileMissing: []string{"tx.txt"},
		},
		{
			Name:    "send_bad_format",
			Balance: balanceP2KH,
			Args:    rfcArgs("-send", ForeignP2KH),
			Exit:    1,
			Out:     []string{"The outputs must be in a format address1=amount1"},
		},
		{
			Name:    "send_bad_address",
			Balance: balanceP2KH,
			Args:    rfcArgs("-send", "1Nope=0.1"),
			Exit:    1,
			Out:     []string{"NewAddrFromString:"},
		},
		{
			Name:    "send_bad_amount",
			Balance: balanceP2KH,
			Args:    rfcArgs("-send", ForeignP2KH+"=abc"),
			Exit:    1,
			Out:     []string{"Incorrect amount:  abc"},
		},
		{
			Name:    "send_testnet_address_on_mainnet",
			Balance: balanceP2KH,
			Args:    rfcArgs("-send", ForeignTest+"=0.1"),
			Exit:    1,
			Out:     []string{"has an incorrect version 111"},
		},
		{
			Name:    "send_testnet_bech32_on_mainnet",
			Balance: balanceP2KH,
			Args:    rfcArgs("-send", ForeignTBech+"=0.1"),
			Exit:    1,
			Out:     []string{"has an incorrect HRP string tb"},
		},
		{
			Name:    "send_bad_change_address",
			Balance: balanceP2KH,
			Args:    rfcArgs("-send", ForeignP2KH+"=0.1", "-change", ForeignTest),
			Exit:    1,
			Out:     []string{"has an incorrect version 111"},
		},
		{
			Name:    "send_bad_fee",
			Balance: balanceP2KH,
			Args:    rfcArgs("-send", ForeignP2KH+"=0.1", "-fee", "lots"),
			Exit:    1,
			Out:     []string{"Incorrect fee value lots"},
		},
	})
}

func TestRawTx(t *testing.T) {
	rawUnsigned := unsignedTx(t, balanceBasic, [][2]int{{1, 0}, {3, 0}}, ForeignP2KH+"=119990000")
	rawUnknownInput := unsignedTx(t, balanceBasic, [][2]int{{1, 0}, {4, 0}}, ForeignP2KH+"=1000")
	Run(t, []Case{
		{
			Name:    "raw_sign_hex_file",
			Balance: balanceBasic,
			Files:   map[string]string{"unsigned.txt": rawUnsigned},
			Args:    rfcArgs("-raw", "unsigned.txt", "-txfn", "signed.txt"),
			Out:     []string{"TxID " + TxidRawSigned, "Transaction data stored in signed.txt"},
			NotOut:  []string{"WARNING"},
			Check: func(r *Result) {
				tx := r.TxID("signed.txt", TxidRawSigned)
				r.VerifyTx("signed.txt")
				r.ExpectOutputs(tx, false, ForeignP2KH+"=119990000")
			},
		},
		{
			Name:    "raw_sign_binary_file",
			Balance: balanceBasic,
			Files:   map[string]string{"unsigned.bin": string(mustHex(t, rawUnsigned))},
			Args:    rfcArgs("-raw", "unsigned.bin", "-txfn", "signed.txt"),
			Out:     []string{"TxID " + TxidRawSigned},
		},
		{
			Name:    "raw_sign_hex_on_command_line",
			Balance: balanceBasic,
			Args:    rfcArgs("-raw", rawUnsigned, "-txfn", "signed.txt"),
			Out:     []string{"TxID " + TxidRawSigned},
		},
		{
			Name:    "raw_sign_missing_key",
			Balance: balanceBasic,
			Files:   map[string]string{"unsigned.txt": rawUnsigned},
			Args:    rfcArgs("-raw", "unsigned.txt", "-txfn", "signed.txt", "-n", "1"), // key #4 (input 2) not in wallet
			Out:     []string{"WARNING: You do not have key for " + Bech32[3] + " at input 1", "WARNING: Not all the inputs have been signed"},
		},
		{
			Name:    "raw_sign_unknown_input",
			Balance: balanceBasic,
			Files:   map[string]string{"unsigned.txt": rawUnknownInput},
			Args:    rfcArgs("-raw", "unsigned.txt"),
			Exit:    1,
			Out:     []string{"Error reading transaction file: balance/"},
		},
		{
			Name:    "raw_sign_bad_file",
			Balance: balanceBasic,
			Files:   map[string]string{"bad.txt": "this is not a transaction"},
			Args:    rfcArgs("-raw", "bad.txt"),
			Out:     []string{"ERROR: Cannot decode the raw transaction"},
		},
		{
			Name:    "decode_unsigned",
			Balance: balanceBasic,
			Files:   map[string]string{"unsigned.txt": rawUnsigned},
			Args:    []string{"-d", "unsigned.txt"},
			Golden:  "decode_unsigned",
			Out:     []string{ForeignP2KH, "Lock Time: 0"},
			Check: func(r *Result) {
				if strings.Contains(r.Both(), "seed password") {
					r.T.Error("-d must not need the seed")
				}
			},
		},
		{
			Name:   "decode_without_balance", // prev outputs unknown, still decodes
			NoSeed: true,
			Files:  map[string]string{"unsigned.txt": rawUnsigned},
			Args:   []string{"-d", "unsigned.txt"},
			Out:    []string{ForeignP2KH},
		},
		{
			Name:  "decode_bad_file",
			Files: map[string]string{"bad.txt": "xyz"},
			Args:  []string{"-d", "bad.txt"},
			Exit:  1,
			Out:   []string{"ERROR: Cannot decode the raw transaction"},
		},
	})
}

func TestMultisig(t *testing.T) {
	redeem, msAddr := multisig2of3(t)
	balance := []Utxo{
		{Tx: 1, Addr: msAddr, Value: 100000000, Label: "2-of-3"},
		{Tx: 1, Addr: msAddr, Value: 50000000},
	}
	raw := unsignedTx(t, balance, [][2]int{{1, 0}, {1, 1}}, ForeignP2KH+"=149990000")

	// stage runs the wallet with the given args in a fresh dir that also has
	// the given extra files, and returns the result.
	stage := func(t *testing.T, files map[string]string, args ...string) *Result {
		t.Helper()
		r := runCase(t, &Case{Balance: balance, Files: files, Args: rfcArgs(args...), Others: OtherWifCompressed + " third\n"})
		if r.Exit != 0 {
			t.Fatalf("exit code %d\n%s", r.Exit, r.Both())
		}
		return r
	}

	Run(t, []Case{
		{
			Name:    "p2sh_prepare",
			Balance: balance,
			Files:   map[string]string{"raw.txt": raw},
			Args:    []string{"-raw", "raw.txt", "-p2sh", redeem},
			Out: []string{
				"The P2SH data points to address " + msAddr,
				"Input number 0  - hash to sign:",
				"Input number 1  - hash to sign:",
				"Transaction with 2 inputs ready for multi-signing, stored in multi2sign.txt",
			},
			Golden: "p2sh_prepare",
			Check: func(r *Result) {
				tx := r.Tx("multi2sign.txt")
				for i := range tx.TxIn {
					ms, err := btc.NewMultiSigFromScript(tx.TxIn[i].ScriptSig)
					if err != nil || len(ms.PublicKeys) != 3 || ms.SigsNeeded != 2 || len(ms.Signatures) != 0 {
						r.T.Errorf("input %d: bad multisig script: %v", i, err)
					}
				}
			},
		},
		{
			Name:    "p2sh_prepare_single_input",
			Balance: balance,
			Files:   map[string]string{"raw.txt": raw},
			Args:    []string{"-raw", "raw.txt", "-p2sh", redeem, "-input", "1"},
			Out:     []string{"Input number 1  - hash to sign:"},
			NotOut:  []string{"Input number 0"},
			Check: func(r *Result) {
				tx := r.Tx("multi2sign.txt")
				if len(tx.TxIn[0].ScriptSig) != 0 || len(tx.TxIn[1].ScriptSig) == 0 {
					r.T.Error("only input 1 should have the P2SH script")
				}
			},
		},
		{
			Name:        "p2sh_bad_hex",
			Balance:     balance,
			Files:       map[string]string{"raw.txt": raw},
			Args:        []string{"-raw", "raw.txt", "-p2sh", "zz"},
			Out:         []string{"P2SH hex data:"},
			FileMissing: []string{"multi2sign.txt"},
		},
		{
			Name:    "msign_two_stages", // wallet key #1, then the imported "other" key
			Balance: balance,
			Files:   map[string]string{"raw.txt": raw},
			Args:    []string{"-raw", "raw.txt", "-p2sh", redeem},
			Check: func(r *Result) {
				m2s := r.File("multi2sign.txt")
				r1 := stage(r.T, map[string]string{"m2s.txt": m2s}, "-msign", P2KH[0], "-raw", "m2s.txt", "-txfn", "s1.txt")
				tx1 := r1.Tx("s1.txt")
				for i := range tx1.TxIn {
					ms, _ := btc.NewMultiSigFromScript(tx1.TxIn[i].ScriptSig)
					if ms == nil || len(ms.Signatures) != 1 {
						r.T.Fatalf("after 1st signing input %d should have 1 signature", i)
					}
				}
				r2 := stage(r.T, map[string]string{"s1.txt": r1.File("s1.txt")}, "-msign", OtherAdrCompressed, "-raw", "s1.txt", "-txfn", "s2.txt")
				r2.Contains("TxID " + TxidMultisigDone)
				tx2 := r2.VerifyTx("s2.txt")
				r2.ExpectOutputs(tx2, false, ForeignP2KH+"=149990000")
			},
		},
		{
			Name:    "msign_same_key_twice_is_deduplicated",
			Balance: balance,
			Files:   map[string]string{"raw.txt": raw},
			Args:    []string{"-raw", "raw.txt", "-p2sh", redeem},
			Check: func(r *Result) {
				r1 := stage(r.T, map[string]string{"m2s.txt": r.File("multi2sign.txt")}, "-msign", P2KH[1], "-raw", "m2s.txt", "-txfn", "s1.txt")
				r2 := stage(r.T, map[string]string{"s1.txt": r1.File("s1.txt")}, "-msign", P2KH[1], "-raw", "s1.txt", "-txfn", "s2.txt", "-v")
				tx := r2.Tx("s2.txt")
				ms, _ := btc.NewMultiSigFromScript(tx.TxIn[0].ScriptSig)
				if ms == nil || len(ms.Signatures) != 1 {
					r.T.Errorf("expected exactly one signature after signing twice with the same key")
				}
			},
		},
		{
			Name:    "msign_unknown_key",
			Balance: balance,
			Files:   map[string]string{"raw.txt": raw},
			Args:    []string{"-raw", "raw.txt", "-p2sh", redeem},
			Check: func(r *Result) {
				r1 := runCase(r.T, &Case{Balance: balance, Files: map[string]string{"m2s.txt": r.File("multi2sign.txt")},
					Args: rfcArgs("-msign", ForeignP2KH, "-raw", "m2s.txt", "-txfn", "s1.txt")})
				r1.Contains("You do not know a key for address " + ForeignP2KH)
				if r1.Exists("s1.txt") {
					r.T.Error("no file should be written")
				}
			},
		},
		{
			Name:    "raw_signs_multisig_with_all_wallet_keys", // -raw without -msign: keys #1 and #2 are both in the wallet
			Balance: balance,
			Files:   map[string]string{"raw.txt": raw},
			Args:    []string{"-raw", "raw.txt", "-p2sh", redeem},
			Check: func(r *Result) {
				r1 := stage(r.T, map[string]string{"m2s.txt": r.File("multi2sign.txt")}, "-raw", "m2s.txt", "-txfn", "done.txt")
				r1.Contains("TxID " + TxidMultisigInWal)
				if strings.Contains(r1.Both(), "WARNING") {
					r.T.Errorf("unexpected warning:\n%s", r1.Both())
				}
				tx := r1.VerifyTx("done.txt")
				ms, _ := btc.NewMultiSigFromScript(tx.TxIn[0].ScriptSig)
				if len(ms.Signatures) != 2 {
					r.T.Errorf("expected 2 signatures, got %d", len(ms.Signatures))
				}
			},
		},
		{
			Name:    "raw_multisig_extra_signatures", // 3 keys available, -xtramsigs keeps all three
			Balance: balance,
			Files:   map[string]string{"raw.txt": raw},
			Args:    []string{"-raw", "raw.txt", "-p2sh", redeem},
			Check: func(r *Result) {
				r1 := stage(r.T, map[string]string{"m2s.txt": r.File("multi2sign.txt")}, "-raw", "m2s.txt", "-txfn", "x.txt", "-xtramsigs")
				tx := r1.Tx("x.txt")
				ms, _ := btc.NewMultiSigFromScript(tx.TxIn[0].ScriptSig)
				if len(ms.Signatures) != 3 {
					r.T.Errorf("expected 3 signatures with -xtramsigs, got %d", len(ms.Signatures))
				}
				r2 := stage(r.T, map[string]string{"m2s.txt": r.File("multi2sign.txt")}, "-raw", "m2s.txt", "-txfn", "y.txt")
				tx = r2.VerifyTx("y.txt")
				ms, _ = btc.NewMultiSigFromScript(tx.TxIn[0].ScriptSig)
				if len(ms.Signatures) != 2 {
					r.T.Errorf("expected 2 signatures without -xtramsigs, got %d", len(ms.Signatures))
				}
			},
		},
		{
			Name:    "msign_output_decodes",
			Balance: balance,
			Files:   map[string]string{"raw.txt": raw},
			Args:    []string{"-raw", "raw.txt", "-p2sh", redeem},
			Check: func(r *Result) {
				r1 := stage(r.T, map[string]string{"m2s.txt": r.File("multi2sign.txt")}, "-raw", "m2s.txt", "-txfn", "done.txt")
				r2 := runCase(r.T, &Case{Balance: balance, Files: map[string]string{"done.txt": r1.File("done.txt")}, Args: []string{"-d", "done.txt"}})
				r2.Contains("TX IN cnt: 2")
				r2.Contains("TX OUT cnt: 1")
			},
		},
	})
}
