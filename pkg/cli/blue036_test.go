package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// BLUE-036 (RED-104, GUI-032, GUI-GREEN-061): poon sign shows the series,
// the hash and each input spent, and refuses a file whose series is not
// its transactions'; a key file's value is never echoed; a watch-only pay
// with no -out records nothing.
func TestBlue036ColdKeySteps(t *testing.T) {
	createIterations = 1
	defer func() { createIterations = 0 }()
	root := t.TempDir()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", "127.0.0.1:0"}, r...)...)
	stop := startNode(t, filepath.Join(root, "policies", "default"))
	defer stop()

	keyFile := seedFile(t, strings.Repeat("c3", 32))
	var k struct {
		PublicKey string `json:"public_key"`
	}
	cli(t, &k, "keygen", "-seed-file", keyFile)
	t.Setenv("POON_PASSPHRASE", "")
	wfile := filepath.Join(root, "watch.json")
	ww := append([]string{"-wallet", wfile}, r...)
	wcmd := func(v any, sub string, args ...string) {
		t.Helper()
		cli(t, v, append(append([]string{"wallet", sub}, ww...), args...)...)
	}
	werr := func(sub string, args ...string) error {
		t.Helper()
		_, err := runErr(append(append([]string{"wallet", sub}, ww...), args...)...)
		return err
	}
	wcmd(nil, "create", "-watch-only")
	wcmd(nil, "policy", "-name", "cold", "-k", "1", "-key", "ed25519:"+k.PublicKey)
	cli(t, nil, append([]string{"claim", "-seed-file", keyFile, "-serial", "5"}, r...)...)
	var b struct{ Layer struct{ Switched bool } }
	cli(t, &b, append([]string{"build", "-pool", "1", "-work", "4000"}, r...)...)
	wcmd(nil, "watch", "-serial", "5")

	// passwd refuses before it asks for a new passphrase.
	if err := werr("passwd"); err == nil || !strings.Contains(err.Error(), "no passphrase to change") {
		t.Fatalf("passwd on a watch-only wallet: %v", err)
	}

	// Pay with no -out: refused, nothing recorded or held.
	before, _ := os.ReadFile(wfile)
	bob := strings.Repeat("b0", 32)
	var bk struct{ Address string }
	cli(t, &bk, "keygen", "-seed-file", seedFile(t, bob), "-hash", "sha256")
	if err := werr("pay", "-to", bk.Address, "-amount", "30"); err == nil || !strings.Contains(err.Error(), "nothing was recorded") {
		t.Fatalf("watch-only pay with no -out: %v", err)
	}
	if after, _ := os.ReadFile(wfile); !bytes.Equal(before, after) {
		t.Fatal("the refused pay changed the wallet file")
	}
	var bal struct{ Total uint64 }
	wcmd(&bal, "balance")
	if bal.Total != 100 {
		t.Fatalf("balance after the refused pay %d, want 100", bal.Total)
	}

	// With -out, poon sign shows the series, hash and inputs.
	u := filepath.Join(t.TempDir(), "pay.json")
	runOut(t, append(append([]string{"wallet", "pay"}, ww...), "-to", bk.Address, "-amount", "30", "-out", u)...)
	var file map[string]any
	data, _ := os.ReadFile(u)
	json.Unmarshal(data, &file)
	var shown SignReport
	cli(t, &shown, "sign", u, "-key-file", keyFile, "-show")
	if shown.Series.String() != file["series"] || shown.Hash == "" || len(shown.Legs) != 1 || len(shown.Legs[0].Spends) != 1 || !strings.Contains(shown.Legs[0].Spends[0], ":") {
		t.Fatalf("sign -show %+v", shown)
	}

	// A file whose series is not its transactions' is refused.
	file["series"] = strings.Repeat("ab", 32)
	bad, _ := json.Marshal(file)
	badFile := filepath.Join(t.TempDir(), "bad.json")
	os.WriteFile(badFile, bad, 0o600)
	if _, err := runErr("sign", badFile, "-key-file", keyFile, "-show"); err == nil || !strings.Contains(err.Error(), "but the payment says series") {
		t.Fatalf("a file with another series: %v", err)
	}

	// A key pasted where its file goes is never printed back.
	secret := strings.Repeat("5e", 32)
	for _, args := range [][]string{
		{"sign", u, "-key-file", secret},
		{"sign", u, "-key-file", secret, "-raw"},
		{"keygen", "-seed-file", secret},
		{"keygen", "-raw", "-seed-file", secret},
		{"claim", "-seed-file", secret, "-serial", "6", "-root", root},
	} {
		_, err := runErr(args...)
		if err == nil || strings.Contains(err.Error(), secret) || strings.Contains(err.Error(), "5e5e") {
			t.Fatalf("%v: %v", args[:2], err)
		}
		if !strings.Contains(err.Error(), "does not exist") {
			t.Fatalf("%v: the error does not say why: %v", args[:2], err)
		}
	}
}

// RED-107: entries before the naming fields are read in a second pass
// over the file, not copied to a temporary file; a pipe, which cannot be
// read twice, is refused. RED-108: header keys after the entries reach
// the result, so the dry run checks the range against them.
func TestBlue036LedgerFileOrder(t *testing.T) {
	const fields = `"kind":"poon-ledger","format":1,"series":"0000000000000000000000000000000000000000000000000000000000000001","pool":7`
	const items = `[{"height":1,"item":"AQI="},{"height":2,"item":"AwQ="}]`
	early := `{"entries":` + items + `,` + fields + `}`
	var got []string
	h, err := readLedgerFile(strings.NewReader(early), func(ledgerFile) error { return nil }, func(it ledgerFileItem) error {
		got = append(got, string(it.Item))
		return nil
	})
	if err != nil || h.Pool != 7 || len(got) != 2 {
		t.Fatalf("seekable, entries first: %+v %q %v", h, got, err)
	}
	pipe := struct{ io.Reader }{strings.NewReader(early)}
	if _, err := readLedgerFile(pipe, func(ledgerFile) error { return nil }, func(ledgerFileItem) error { return nil }); err == nil || !strings.Contains(err.Error(), "cannot be read twice") {
		t.Fatalf("a pipe, entries first: %v", err)
	}
	// From a file opened at an offset, the second pass starts where the
	// first did.
	f, _ := os.CreateTemp(t.TempDir(), "ledger")
	f.WriteString("junk" + early)
	f.Seek(4, io.SeekStart)
	got = nil
	if _, err := readLedgerFile(f, func(ledgerFile) error { return nil }, func(it ledgerFileItem) error {
		got = append(got, string(it.Item))
		return nil
	}); err != nil || len(got) != 2 {
		t.Fatalf("a file at an offset: %q %v", got, err)
	}
	f.Close()

	late := `{` + fields + `,"entries":[{"height":1,"item":"AQI="}],"from":1,"to":999,"height":999}`
	h, err = readLedgerFile(strings.NewReader(late), func(ledgerFile) error { return nil }, func(ledgerFileItem) error { return nil })
	if err != nil || h.From != 1 || h.To != 999 {
		t.Fatalf("keys after the entries: %+v %v", h, err)
	}
	var out bytes.Buffer
	format := "json"
	o := &outputFlags{format: &format, wide: new(bool)}
	err = ledgerDryRun(strings.NewReader(late), t.TempDir(), o, &out)
	if !errors.Is(err, ErrDryRunProblems) || !strings.Contains(out.String(), "ends at 999") || strings.Contains(out.String(), "starts at") {
		t.Fatalf("dry run with keys after the entries: %v\n%s", err, out.String())
	}
}
