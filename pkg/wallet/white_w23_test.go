package wallet_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wallet"
)

// W-24 (D195): a field a later release added to the wallet file survives a
// save by this one.
func TestWalletKeepsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	if _, err := wallet.Create(path, "pw", wallet.Options{Iterations: 1, Gap: 2}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	var m map[string]json.RawMessage
	must(t, json.Unmarshal(b, &m))
	m["later_release"] = json.RawMessage(`{"labels":["a","b"]}`)
	b, _ = json.MarshalIndent(m, "", "  ")
	must(t, os.WriteFile(path, b, 0o600))

	w, err := wallet.Open(path)
	must(t, err)
	must(t, w.Unlock("pw"))
	must(t, w.Rekey("pw2"))
	must(t, w.Save())
	b, _ = os.ReadFile(path)
	m = nil
	must(t, json.Unmarshal(b, &m))
	if compact(m["later_release"]) != `{"labels":["a","b"]}` {
		t.Fatalf("the later field after a save: %s\n%s", m["later_release"], b)
	}
	if _, err := wallet.Open(path); err != nil {
		t.Fatalf("reopen: %v", err)
	}
}

// W-23 (D195): the payment file names its kind and format; one with an
// unknown top-level or leg field reads and writes them back unchanged; a
// later format and another kind are refused; a file with neither reads.
func TestPaymentFileKindFormatAndUnknownFields(t *testing.T) {
	const legs = `"legs":[{"serial":5,"tx":"00","inputs":[{"policy":"00"}],"leg_note":{"x":1}}]`
	src := `{"kind":"poo-payment","format":1,"series":"` + strings.Repeat("0", 64) + `","hash":"sha256","later":[1,2],` + legs + `}`
	var u wallet.Unsigned
	must(t, json.Unmarshal([]byte(src), &u))
	if u.Kind != wallet.PaymentFileKind || u.Format != wallet.PaymentFormat || len(u.Legs) != 1 {
		t.Fatalf("read %+v", u)
	}
	b, err := json.MarshalIndent(&u, "", "  ")
	must(t, err)
	var back map[string]json.RawMessage
	must(t, json.Unmarshal(b, &back))
	var leg []map[string]json.RawMessage
	must(t, json.Unmarshal(back["legs"], &leg))
	if compact(back["later"]) != `[1,2]` || compact(leg[0]["leg_note"]) != `{"x":1}` {
		t.Fatalf("unknown fields not written back:\n%s", b)
	}

	old := strings.Replace(src, `"kind":"poo-payment","format":1,`, ``, 1)
	if err := json.Unmarshal([]byte(old), &u); err != nil {
		t.Fatalf("a file from before W-23: %v", err)
	}
	two := strings.Replace(src, `"format":1`, `"format":2`, 1)
	if err := json.Unmarshal([]byte(two), &u); err == nil || !strings.Contains(err.Error(), "format 2") {
		t.Fatalf("format 2: %v", err)
	}
	other := strings.Replace(src, `"poo-payment"`, `"poo-ledger"`, 1)
	if err := json.Unmarshal([]byte(other), &u); err == nil || !strings.Contains(err.Error(), "not a payment") {
		t.Fatalf("another kind: %v", err)
	}
}

func compact(b []byte) string {
	var c bytes.Buffer
	if json.Compact(&c, b) != nil {
		return string(b)
	}
	return c.String()
}
