# Gocoin wallet regression tests

This folder contains a high level regression test suite for the `wallet` tool.

Rather than calling wallet functions from Go, the suite **executes the real
wallet binary** the way a user would. Every test case:

1. creates a fresh temporary directory,
2. populates it with the files the case needs (`wallet.cfg`, `.secret`,
   `.others`, a `balance/` folder, raw transactions, batch files, ...),
3. runs the wallet there with the given command line switches,
4. compares the exit code, the stdout/stderr and the files the wallet
   produced against the expected results.

Because the whole path from command line parsing to the files written on
disk is exercised, the suite catches regressions in any part of the tool.
It runs in about 1-2 seconds and needs no network, no node and no user
interaction (the wallet's prompts are answered by the suite itself).

## Running

The suite is a normal Go test package:

```
go test ./wallet/regtest/                      # everything (builds the wallet first)
go test ./wallet/regtest/ -v                   # list every case
go test ./wallet/regtest/ -run TestSend        # one group
go test ./wallet/regtest/ -run TestSend/send_batch_file -v   # one case
```

A note on Go's test result cache: the suite does not import the wallet code
(the wallet is a `package main`) - it builds the wallet and runs it as a
separate process, so the wallet's sources are not a part of what the test
binary is built from and, by itself, `go test` would not notice that they
have changed and would replay a cached PASS. To prevent that, the harness
stats every `.go` file of the wallet and of all the packages it depends on
(`trackSources()` in `harness_test.go`); Go records those accesses and
invalidates the cached result whenever one of the files changes. So a
`(cached)` result means that neither the tests nor the wallet's sources have
changed since the last run. With `GOCOIN_WALLET_BIN` the cache depends on
the given binary instead. `-count=1` still forces a run at any time.

Options specific to this suite:

| Option | Meaning |
|---|---|
| `-update` | Regenerate the golden files in `testdata/golden/` from the current wallet output. Review the resulting `git diff` carefully: it is the change in behaviour you are accepting. |
| `-keep` | Do not delete the temporary work directories. Their paths are printed with `-v`, so you can inspect the config, balance folder and output files of a failed case, or re-run the wallet there by hand. |
| `GOCOIN_WALLET_BIN=/path/to/wallet` (environment) | Test the given prebuilt executable instead of building one from `../`. Useful for checking a release binary or a build with different flags. |
| `GOCOIN_REGTEST_INTERACTIVE=1` (environment) | Run the interactive cases (`TestInteractive`) on systems where they are skipped by default - see below. |

## Groups of tests

| Test | What it covers |
|---|---|
| `TestList` | Key generation and `-l`/`-list` for type-3 and type-4 (HD) wallets: mainnet, testnet, litecoin, every `atype`, `hdpath` variants of known wallets, `bip39=12..24`, `bip39=-1` (mnemonic as the seed, public BIP39/BIP44/BIP84 test vectors), `hdsubs`, `scrypt`, `-xprv`, `-words` (round-trip: the printed words must regenerate the same wallet), `-q`, `-v`. |
| `TestSeedInput` | All the ways the seed reaches the wallet: `.secret` file, `secret=` path, `seed=` prefix and `-is`, `-stdin`, empty / maximum length / too long / non-printable seeds. |
| `TestConfig` | Locating the config file (`-cfg`, `--cfg=`, `GOCOIN_WALLET_CONFIG`, precedence), parsing of every config key, value errors, command line switches overriding config values, `fee`/`apply2bal`/`rfc6979`/`minsig` from the config. |
| `TestOthers` | Importing private keys from `.others`: compressed and uncompressed keys, labels, comments, invalid lines, keys of the wrong network, `others=` path. |
| `TestDump` | `-dump <address>` (by P2KH, P2SH-segwit, bech32 and taproot address), `-dump *`, `-pub`, unknown and invalid addresses. |
| `TestSignMessage` | `-sign` with `-msg`, `-hash` and a message on stdin, compressed and uncompressed keys, litecoin and testnet, `-sign` combined with `-send`. |
| `TestBalance` | Loading the `balance/` folder and showing the balance: known, multisig and unspendable outputs, taproot, testnet, corrupt `unspent.txt`, missing files. |
| `TestSend` | Building and signing transactions with `-send` and `-batch`: every input type (P2PKH, P2SH-P2WPKH, P2WPKH, P2TR), every output type, change selection, `-change`, `-f`, `-fee`, `-msg` (OP_RETURN), `-useallinputs`, `-locktime`, `-txver`, `-seq`, `-txfn`, `-a=false`, `-minsig`, updating of the `balance/` folder, and all the error paths (insufficient funds, bad addresses, wrong network, ...). |
| `TestRawTx` | `-raw` (hex file, binary file and hex on the command line), missing keys / inputs, and `-d` decoding. |
| `TestMultisig` | The full multisig flow: `-p2sh` (with and without `-input`), `-msign` by several parties in turn, signing with all wallet keys at once, de-duplication and ordering of signatures, `-xtramsigs`. |
| `TestInteractive` | The wallet's prompts, answered through stdin: entering and re-entering the seed password, saving it to disk (`.secret` or `secret=` path), `-1`, `-p`, the BIP39 password (`-p39`), and the `-prompt` transaction confirmation (accepted and rejected). |
| `TestEncrypt` | `-encrypt`/`-decrypt` round trips for type-3 and type-4 wallets, wrong password, wrong wallet type, malformed input. |
| `TestErrors` | Command line and config validation errors (`-h`, unknown switches, unsupported wallet types, bad `hdpath`/`atype`/`fee`/`scrypt`, `-p` with `-stdin`, ...). |

## Test vectors

All keys derive from two **fixed seeds** defined in `fixtures_test.go`:

* `qwerty12345` - the default seed, used for type-3 and type-4 wallets.
  The expected addresses include the vectors from the original
  `wallet/wallet_test.go`.
* `abandon abandon ... about` - the standard BIP39 test mnemonic, used with
  `bip39=-1`. Its root xprv and the first BIP44/BIP84 addresses are the
  publicly documented ones, which anchors the HD implementation to an
  external reference.

Two standalone WIF keys (one compressed, one uncompressed) are used for the
`.others` file and as the third party of the 2-of-3 multisig.

Never change these seeds: every expected value in the suite derives from them.

### Balance folder

The wallet reads previous outputs from `balance/`. The suite fabricates
them: `Utxo` records describe outputs paying to the wallet's own addresses,
and `writeBalance()` builds deterministic "funding" transactions from them
(they do not exist on any chain, the wallet only needs their outputs) and
writes `balance/unspent.txt` and `balance/<txid>.tx`. Because the funding
transactions are deterministic, so are their IDs and everything derived from
them.

### Deterministic and randomized outputs

Cases that sign use `-rfc6979` (or `rfc6979=true`), which makes ECDSA
signatures deterministic, so the resulting transaction IDs and message
signatures are compared **exactly** against constants in the test files.

Some outputs are randomized by design and cannot be compared byte for byte:
Schnorr (taproot) signatures, `-minsig` re-signing without RFC6979, and the
AES-GCM nonce of `-encrypt`. Those cases verify the result instead:

* `Result.VerifyTx()` runs every input of a produced transaction through the
  gocoin script engine (`lib/script`) with the standard verification flags,
* `VerifyMessageSig()` recovers the public key from a signed message and
  compares it with the signing address,
* encryption is tested by a decrypt round trip.

`VerifyTx()` is also called on the deterministic cases, so a change that
keeps the TxID but breaks a signature is still caught.

### Golden files

Longer outputs (address listings, `-dump *`, decoded transactions) are
compared with files in `testdata/golden/`. Line endings are normalized
before the comparison, so the files may be checked out with either LF or
CRLF. `-update` rewrites them.

Note that `wallet/.gitignore` ignores `*.txt`; the `.gitignore` in this
folder re-includes `testdata/golden/*.txt`.

## Adding a test case

Cases are plain table entries. Add one to the relevant `Test*` function:

```go
{
    Name:    "send_custom_fee",
    Balance: balanceP2KH,                       // builds balance/ from Utxo records
    Cfg:     "rfc6979=true\n",                   // content of wallet.cfg ("" = no file)
    Args:    []string{"-send", ForeignP2KH + "=0.5", "-fee", "0.00123", "-txfn", "tx.txt"},
    Exit:    0,                                  // expected exit code
    Out:     []string{"Transaction data stored in tx.txt"},   // substrings of stdout+stderr
    Check: func(r *Result) {
        tx := r.VerifyTx("tx.txt")               // script-verify every input
        r.ExpectOutputs(tx, false, ForeignP2KH+"=50000000", P2KH[0]+"=49877000")
    },
},
```

The `Case` struct (in `harness_test.go`) documents every available input
(`Seed`, `SeedFile`, `NoSeed`, `Others`, `Files`, `Balance`, `Stdin`, `Env`,
`CfgFile`, ...) and expectation (`Exit`, `Out`, `NotOut`, `Golden`,
`GoldenFiles`, `FileEquals`, `FileExists`, `FileMissing`, `Check`).

`Check` receives a `Result` with the work directory, captured output and
helpers such as `File()`, `Exists()`, `Contains()`, `Tx()`, `TxID()`,
`VerifyTx()` and `ExpectOutputs()`. A `Check` can also call `runCase()`
to run the wallet again with a different setup and compare the two results
(see the multisig and `-words` round-trip cases).

To get a new expected value, write the case with a placeholder, run it, and
copy the value the wallet printed - after making sure it is right.

A case that documents a bug not yet fixed can be kept in the tables with
`KnownIssue: "description"`; it is then skipped (visibly, with `-v`) instead
of failing. Clear the field when the wallet is fixed.

## Interactive cases

`TestInteractive` covers the wallet's prompts. The harness starts the wallet
with a stdin pipe, watches its stdout, and each time a prompt appears (all
prompts end with `: ` and no newline) writes the next answer from the case's
`Prompts` list. Answers are written one at a time on purpose: the wallet
reads the password with a single `Read()`, so several lines written at once
would be swallowed together.

Whether this works depends on how the wallet reads the password on the
system:

* **Linux, macOS**: the wallet reads the password from stdin whenever stdin is
  not a terminal, so the cases run by default.
* **Windows**: the wallet reads the password with `_getch()` from the console.
  The stdin fallback (`lib/others/sys/hidepass_windows.go`: when stdin is not
  a console, read it as a plain line) makes a redirected stdin work, but the
  cases are skipped there by default until that has been confirmed; set
  `GOCOIN_REGTEST_INTERACTIVE=1` to run them.

Skipped cases show up as `SKIP` with `-v`, so a run never silently loses
coverage.

## Requirements

* Go (the suite builds the wallet with `go build ..`).
* Nothing else: no network, no bitcoin node and no terminal. The seed
  comes from a file, from `-stdin`, or - in `TestInteractive` - from the
  prompt answers fed by the suite.
