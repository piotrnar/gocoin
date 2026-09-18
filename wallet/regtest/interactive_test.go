package regtest

import "testing"

// TestInteractive covers the wallet's prompts: entering (and re-entering)
// the seed password, saving it to disk, the BIP39 password (-p39) and the
// transaction confirmation (-prompt). The answers are fed through stdin,
// one line per prompt. See interactiveSupported() for where these run.
func TestInteractive(t *testing.T) {
	Run(t, []Case{
		{
			Name:        "password_entered_twice",
			NoSeed:      true,
			Args:        []string{"-l", "-n", "4"},
			Prompts:     []string{TestSeed, TestSeed, "n"},
			Out:         []string{"Seed file .secret not found", "Enter your wallet's seed password:", "Re-enter the seed password (to be sure):", "Save the password on disk"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_p2kh"},
			FileMissing: []string{".secret"},
		},
		{
			Name:        "password_saved_to_disk",
			NoSeed:      true,
			Args:        []string{"-l", "-n", "4"},
			Prompts:     []string{TestSeed, TestSeed, "y"},
			Out:         []string{"The seed password has been stored in .secret"},
			FileEquals:  map[string]string{".secret": TestSeed},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_p2kh"},
		},
		{
			Name:        "password_saved_to_custom_secret_file",
			NoSeed:      true,
			Cfg:         "secret=my.secret\nkeycnt=4\n",
			Args:        []string{"-l"},
			Prompts:     []string{TestSeed, TestSeed, "y"},
			Out:         []string{"Seed file my.secret not found", "The seed password has been stored in my.secret"},
			FileEquals:  map[string]string{"my.secret": TestSeed},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_p2kh"},
		},
		{
			Name:        "password_mismatch",
			NoSeed:      true,
			Args:        []string{"-l", "-n", "4"},
			Prompts:     []string{TestSeed, "something else"},
			Out:         []string{"two passwords entered by the user do not match"},
			FileMissing: []string{"wallet.txt", ".secret"},
		},
		{
			Name:        "password_empty",
			NoSeed:      true,
			Args:        []string{"-l", "-n", "4"},
			Prompts:     []string{""},
			Out:         []string{"empty seed password entered by the user"},
			FileMissing: []string{"wallet.txt"},
		},
		{
			Name:        "single_ask",
			NoSeed:      true,
			Args:        []string{"-l", "-1", "-n", "4"},
			Prompts:     []string{TestSeed, "n"},
			NotOut:      []string{"Re-enter"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_p2kh"},
		},
		{
			Name:        "force_ask_ignores_secret_file", // -p: .secret exists but is ignored, no offer to save
			Seed:        "wrong password in the file",
			Args:        []string{"-l", "-p", "-n", "4"},
			Prompts:     []string{TestSeed, TestSeed},
			NotOut:      []string{"Save the password on disk"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_p2kh"},
			FileEquals:  map[string]string{".secret": "wrong password in the file"},
		},
		{
			Name:    "no_reask_outside_generation_mode", // re-entering only happens with -l, -words or -xprv
			NoSeed:  true,
			Args:    []string{"-dump", P2KH[0], "-n", "1"},
			Prompts: []string{TestSeed},
			Out:     []string{"Private encoded: " + WIF[0]},
			NotOut:  []string{"Re-enter", "Save the password"},
		},
		{
			Name:    "reask_with_xprv",
			NoSeed:  true,
			Args:    []string{"-type", "4", "-xprv"},
			Prompts: []string{TestSeed, TestSeed, "n"},
			Out:     []string{"Re-enter the seed password", "Root: xprv"},
		},
		{
			Name:        "seed_prefix_applies_to_typed_password",
			NoSeed:      true,
			Cfg:         "seed=prefix-\nkeycnt=4\n",
			Args:        []string{"-l", "-1"},
			Prompts:     []string{TestSeed, "n"},
			GoldenFiles: map[string]string{"wallet.txt": "wallet_type3_seedprefix"},
		},

		// BIP39 password (-p39) - "TREZOR" passphrase vector from the BIP39 spec
		{
			Name:    "bip39_password",
			Seed:    Mnemonic,
			Args:    []string{"-type", "4", "-bip39", "-1", "-xprv", "-p39"},
			Prompts: []string{"TREZOR"},
			Out:     []string{"Enter the BIP39 password:", "Root: xprv9s21ZrQH143K3h3fDYiay8mocZ3afhfULfb5GX8kCBdno77K4HiA15Tg23wpbeF1pLfs1c5SPmYHrEpTuuRhxMwvKDwqdKiGJS9XFKzUsAF"},
		},
		{
			Name:    "bip39_password_empty",
			Seed:    Mnemonic,
			Args:    []string{"-type", "4", "-bip39", "-1", "-xprv", "-p39"},
			Prompts: []string{""},
			Out:     []string{"You entered empty password"},
			NotOut:  []string{"Root:"},
		},
		{
			Name:    "bip39_password_with_typed_mnemonic",
			NoSeed:  true,
			Args:    []string{"-type", "4", "-bip39", "-1", "-xprv", "-p39", "-1"},
			Prompts: []string{Mnemonic, "n", "TREZOR"}, // the save question comes before the BIP39 password
			Out:     []string{"Root: xprv9s21ZrQH143K3h3fDYiay8mocZ3afhfULfb5GX8kCBdno77K4HiA15Tg23wpbeF1pLfs1c5SPmYHrEpTuuRhxMwvKDwqdKiGJS9XFKzUsAF"},
		},

		// -prompt: confirm the signed transaction before it is written
		{
			Name:    "send_prompt_confirmed",
			Balance: balanceP2KH,
			Args:    rfcArgs("-send", ForeignP2KH+"=0.5", "-prompt", "-txfn", "tx.txt"),
			Prompts: []string{"y"},
			Out:     []string{"TX OUT cnt: 2", "Do you confirm creating the signed transaction file (tx.txt)?", "Transaction data stored in tx.txt"},
			Check:   func(r *Result) { r.VerifyTx("tx.txt") },
		},
		{
			Name:        "send_prompt_rejected", // an invalid answer is asked again, "n" aborts with exit code 2
			Balance:     balanceP2KH,
			Args:        rfcArgs("-send", ForeignP2KH+"=0.5", "-prompt", "-txfn", "tx.txt"),
			Prompts:     []string{"maybe", "n"},
			Exit:        2,
			Out:         []string{"Aborted (exit code 2)"},
			FileMissing: []string{"tx.txt"},
			Check: func(r *Result) {
				if got := r.File("balance/unspent.txt"); got != fundingTx(r.T, 1, balanceP2KH).Hash.String()+"-0\n" {
					r.T.Error("balance must stay untouched after an aborted send")
				}
			},
		},
		{
			Name:    "send_prompt_from_cfg",
			Balance: balanceP2KH,
			Cfg:     "prompt=true\nrfc6979=true\n",
			Args:    []string{"-send", ForeignP2KH + "=0.5", "-txfn", "tx.txt"},
			Prompts: []string{"y"},
			Out:     []string{"Do you confirm creating the signed transaction file (tx.txt)?"},
			Check:   func(r *Result) { r.VerifyTx("tx.txt") },
		},
	})
}
