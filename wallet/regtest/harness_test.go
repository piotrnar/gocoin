// Package regtest is a high level regression test suite for the gocoin wallet.
//
// Every test case executes the real wallet executable in a fresh temporary
// directory populated with a config file, seed file, .others file, balance/
// folder and any other input files the case needs. The process' exit code,
// stdout/stderr and the files it produces are then compared against the
// expected values (embedded in the test tables or in testdata/golden/*.txt).
//
//	go test ./wallet/regtest/                       run everything (builds the wallet first)
//	go test ./wallet/regtest/ -run 'TestSend/'      run one group of cases
//	go test ./wallet/regtest/ -v -run 'TestList/bip39_12_words'
//	go test ./wallet/regtest/ -update               regenerate the golden files
//	go test ./wallet/regtest/ -keep                 keep the work directories (paths are logged)
//	GOCOIN_WALLET_BIN=/path/to/wallet go test ./wallet/regtest/    test a prebuilt executable
//	GOCOIN_REGTEST_INTERACTIVE=1 go test ./wallet/regtest/        run the interactive cases also on Windows
//
// Interactive cases (the ones answering the wallet's prompts through stdin)
// run on Linux and macOS and are skipped on other systems unless
// GOCOIN_REGTEST_INTERACTIVE is set.
package regtest

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

var (
	updateGolden = flag.Bool("update", false, "regenerate golden files in testdata/golden")
	keepWork     = flag.Bool("keep", false, "keep the temporary work directories")
	walletBin    string
)

const (
	defaultSeedFile   = ".secret"
	defaultOthersFile = ".others"
	defaultCfgFile    = "wallet.cfg"
	caseTimeout       = 60 * time.Second
)

func TestMain(m *testing.M) {
	flag.Parse()
	walletBin = os.Getenv("GOCOIN_WALLET_BIN")
	if walletBin == "" {
		dir, err := os.MkdirTemp("", "gocoin-wallet-bin-")
		if err != nil {
			fmt.Println("MkdirTemp:", err)
			os.Exit(1)
		}
		walletBin = filepath.Join(dir, "wallet")
		if os.PathSeparator == '\\' {
			walletBin += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", walletBin, "..")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			fmt.Println("cannot build the wallet:", err)
			os.Exit(1)
		}
		defer os.RemoveAll(dir)
	} else if abs, err := filepath.Abs(walletBin); err == nil {
		walletBin = abs
	}
	os.Exit(m.Run())
}

// Case describes one execution of the wallet and the expectations about it.
type Case struct {
	Name string

	// Inputs
	Cfg         string            // content of the config file ("" = do not create one)
	CfgFile     string            // name of the config file (default "wallet.cfg")
	Args        []string          // command line switches
	Seed        string            // content of the seed file (default TestSeed)
	NoSeed      bool              // do not create the seed file at all
	SeedFile    string            // name of the seed file (default ".secret")
	Others      string            // content of the .others file ("" = do not create one)
	Files       map[string]string // extra files to create: relative path -> content
	Balance     []Utxo            // build the balance/ folder from these outputs
	Stdin       string            // written to stdin, then stdin is closed
	Prompts     []string          // written to stdin line by line, each one after the wallet prints a prompt (Interactive)
	Env         map[string]string // extra environment variables
	NoPar       bool              // do not run in parallel with other cases
	Interactive bool              // answers wallet prompts (implied by Prompts) - see interactiveSupported()

	// KnownIssue marks a case that documents the intended behaviour but is
	// known to fail on the current code. It is skipped (visibly, with -v)
	// instead of run. Clear the field once the wallet is fixed.
	KnownIssue string

	// Expectations
	Exit        int               // expected exit code
	Out         []string          // substrings that must appear in stdout+stderr
	NotOut      []string          // substrings that must not appear in stdout+stderr
	Golden      string            // compare stdout with testdata/golden/<Golden>.txt
	GoldenFiles map[string]string // work-dir file -> golden name
	FileEquals  map[string]string // work-dir file -> exact expected content
	FileExists  []string          // files that must exist after the run
	FileMissing []string          // files that must not exist after the run
	Check       func(r *Result)   // custom checks
}

// Result holds everything produced by one wallet execution.
type Result struct {
	T      *testing.T
	Case   *Case
	Dir    string
	Stdout string
	Stderr string
	Exit   int
}

// Both returns stdout and stderr concatenated.
func (r *Result) Both() string { return r.Stdout + r.Stderr }

// File returns the content of a file from the work directory.
func (r *Result) File(name string) string {
	d, err := os.ReadFile(filepath.Join(r.Dir, name))
	if err != nil {
		r.T.Fatalf("reading %s: %v", name, err)
	}
	return string(d)
}

// Exists tells whether a file exists in the work directory.
func (r *Result) Exists(name string) bool {
	_, err := os.Stat(filepath.Join(r.Dir, name))
	return err == nil
}

// Contains fails the test if the combined output does not contain s.
func (r *Result) Contains(s string) {
	r.T.Helper()
	if !strings.Contains(r.Both(), s) {
		r.T.Errorf("output does not contain %q\n--- output ---\n%s", s, r.Both())
	}
}

// Lines returns the non-empty lines of stdout that begin with prefix.
func (r *Result) Lines(prefix string) (res []string) {
	for _, l := range strings.Split(r.Stdout, "\n") {
		if strings.HasPrefix(l, prefix) {
			res = append(res, strings.TrimSpace(l))
		}
	}
	return
}

// Run executes all the cases as sub-tests of t.
func Run(t *testing.T, cases []Case) {
	for i := range cases {
		c := &cases[i]
		t.Run(c.Name, func(t *testing.T) {
			if c.KnownIssue != "" {
				t.Skip("known issue: " + c.KnownIssue)
			}
			if (c.Interactive || len(c.Prompts) > 0) && !interactiveSupported() {
				t.Skipf("interactive cases are not run on %s (set GOCOIN_REGTEST_INTERACTIVE=1 to force)", runtime.GOOS)
			}
			if !c.NoPar {
				t.Parallel()
			}
			r := runCase(t, c)
			check(r)
		})
	}
}

// interactiveSupported tells whether the wallet's password prompts can be
// answered through a stdin pipe on this system. On Linux and macOS the wallet
// reads the password from stdin whenever it is not a terminal. On Windows it
// needs the wallet to be built with the stdin fallback in hidepass_windows.go,
// so the cases are opt-in there.
func interactiveSupported() bool {
	if os.Getenv("GOCOIN_REGTEST_INTERACTIVE") != "" {
		return true
	}
	return runtime.GOOS == "linux" || runtime.GOOS == "darwin"
}

// syncBuf is a goroutine-safe bytes.Buffer (stdout is read while the wallet runs).
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	fn := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(fn), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fn, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func setupDir(t *testing.T, c *Case) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "gocoin-regtest-")
	if err != nil {
		t.Fatal(err)
	}
	if *keepWork {
		t.Logf("work dir: %s", dir)
	} else {
		t.Cleanup(func() { os.RemoveAll(dir) })
	}

	if !c.NoSeed {
		seedfn := c.SeedFile
		if seedfn == "" {
			seedfn = defaultSeedFile
		}
		seed := c.Seed
		if seed == "" {
			seed = TestSeed
		}
		write(t, dir, seedfn, seed)
	}
	if c.Others != "" {
		write(t, dir, defaultOthersFile, c.Others)
	}
	if c.Cfg != "" {
		cfgfn := c.CfgFile
		if cfgfn == "" {
			cfgfn = defaultCfgFile
		}
		write(t, dir, cfgfn, c.Cfg)
	}
	if len(c.Balance) > 0 {
		writeBalance(t, dir, c.Balance)
	}
	for name, content := range c.Files { // after Balance, so that a case can override balance/ files
		write(t, dir, name, content)
	}
	return dir
}

func runCase(t *testing.T, c *Case) *Result {
	t.Helper()
	dir := setupDir(t, c)

	cmd := exec.Command(walletBin, c.Args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOCOIN_WALLET_CONFIG=")
	for k, v := range c.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var stdout, stderr syncBuf
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	var stdin io.WriteCloser
	if len(c.Prompts) > 0 {
		var err error
		if stdin, err = cmd.StdinPipe(); err != nil {
			t.Fatal(err)
		}
	} else {
		cmd.Stdin = strings.NewReader(c.Stdin)
	}

	if err := cmd.Start(); err != nil {
		t.Fatalf("cannot start %s: %v", walletBin, err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	if stdin != nil {
		go answerPrompts(stdin, &stdout, c.Prompts, done)
	}

	var exit int
	select {
	case err := <-done:
		if ee, ok := err.(*exec.ExitError); ok {
			exit = ee.ExitCode()
		} else if err != nil {
			t.Fatalf("wallet execution failed: %v", err)
		}
	case <-time.After(caseTimeout):
		cmd.Process.Kill()
		t.Fatalf("timeout - the wallet did not finish in %s\n--- stdout ---\n%s\n--- stderr ---\n%s",
			caseTimeout, stdout.String(), stderr.String())
	}

	return &Result{T: t, Case: c, Dir: dir, Stdout: stdout.String(), Stderr: stderr.String(), Exit: exit}
}

// answerPrompts feeds the lines to the wallet's stdin one at a time, each
// after the wallet has printed a new prompt. All the wallet's prompts end
// with ": " and no newline, so a stdout that ends with ": " means the wallet
// is waiting for input. Writing everything at once would not work: a single
// Read() by the wallet could then swallow several lines.
func answerPrompts(stdin io.WriteCloser, stdout *syncBuf, lines []string, done chan error) {
	defer stdin.Close()
	prompts := 0
	for _, l := range lines {
		deadline := time.Now().Add(10 * time.Second)
		for {
			out := stdout.String()
			if n := strings.Count(out, ": "); n > prompts && strings.HasSuffix(out, ": ") {
				prompts = n
				break
			}
			if time.Now().After(deadline) || len(done) > 0 {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		if _, err := io.WriteString(stdin, l+"\n"); err != nil {
			return
		}
	}
}

func goldenPath(name string) string {
	return filepath.Join("testdata", "golden", name+".txt")
}

func compareGolden(t *testing.T, name, got string) {
	t.Helper()
	fn := goldenPath(name)
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(fn), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fn, []byte(got), 0644); err != nil {
			t.Fatal(err)
		}
		t.Logf("golden file %s updated", fn)
		return
	}
	want, err := os.ReadFile(fn)
	if err != nil {
		t.Fatalf("missing golden file %s (run with -update to create it)\n--- got ---\n%s", fn, got)
	}
	// golden files may have been checked out with CRLF line endings (git autocrlf on Windows)
	if w, g := normalizeEOL(string(want)), normalizeEOL(got); w != g {
		t.Errorf("mismatch against golden file %s\n%s", fn, diffLines(w, g))
	}
}

// normalizeEOL converts CRLF line endings to LF.
func normalizeEOL(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func diffLines(want, got string) string {
	wl := strings.Split(want, "\n")
	gl := strings.Split(got, "\n")
	var sb strings.Builder
	for i := 0; i < len(wl) || i < len(gl); i++ {
		var w, g string
		if i < len(wl) {
			w = wl[i]
		}
		if i < len(gl) {
			g = gl[i]
		}
		if w != g {
			fmt.Fprintf(&sb, "line %d:\n  want: %q\n  got:  %q\n", i+1, w, g)
		}
	}
	return sb.String()
}

// normalize replaces the work directory path in the output with a stable token.
func (r *Result) normalize(s string) string {
	return strings.ReplaceAll(s, r.Dir, "$WORK")
}

func check(r *Result) {
	t, c := r.T, r.Case
	t.Helper()

	if r.Exit != c.Exit {
		t.Errorf("exit code %d, expected %d\n--- stdout ---\n%s\n--- stderr ---\n%s", r.Exit, c.Exit, r.Stdout, r.Stderr)
	}
	for _, s := range c.Out {
		r.Contains(s)
	}
	for _, s := range c.NotOut {
		if strings.Contains(r.Both(), s) {
			t.Errorf("output must not contain %q\n--- output ---\n%s", s, r.Both())
		}
	}
	if c.Golden != "" {
		compareGolden(t, c.Golden, r.normalize(r.Stdout))
	}
	for fn, g := range c.GoldenFiles {
		if !r.Exists(fn) {
			t.Errorf("file %s was not created", fn)
			continue
		}
		compareGolden(t, g, r.normalize(r.File(fn)))
	}
	for fn, want := range c.FileEquals {
		if !r.Exists(fn) {
			t.Errorf("file %s was not created", fn)
			continue
		}
		if got := r.File(fn); normalizeEOL(got) != normalizeEOL(want) {
			t.Errorf("file %s content mismatch\n%s", fn, diffLines(want, got))
		}
	}
	for _, fn := range c.FileExists {
		if !r.Exists(fn) {
			t.Errorf("file %s was not created", fn)
		}
	}
	for _, fn := range c.FileMissing {
		if r.Exists(fn) {
			t.Errorf("file %s must not exist", fn)
		}
	}
	if c.Check != nil {
		c.Check(r)
	}
}
