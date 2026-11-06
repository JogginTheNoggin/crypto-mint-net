package cli

import (
	"fmt"
	"io"
	"strings"
	"testing"
)

// BLUE-035 (RED-102): a ledger file's header takes only the keys format 1
// defines, each value bounded as it is read, and each key once.

// longValue is a reader giving prefix, then a JSON string of n bytes, then
// suffix, counting what is read.
type longValue struct {
	r    io.Reader
	read int64
}

func newLongValue(prefix string, n int64, suffix string) *longValue {
	body := io.LimitReader(repeatReader('A'), n)
	return &longValue{r: io.MultiReader(strings.NewReader(prefix+`"`), body, strings.NewReader(`"`+suffix))}
}

func (l *longValue) Read(p []byte) (int, error) {
	n, err := l.r.Read(p)
	l.read += int64(n)
	return n, err
}

type repeatReader byte

func (b repeatReader) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(b)
	}
	return len(p), nil
}

func TestBlue035LedgerHeaderBounded(t *testing.T) {
	const fields = `"kind":"poo-ledger","format":1,"series":"0000000000000000000000000000000000000000000000000000000000000001","pool":7`
	const entries = `"entries":[{"height":1,"item":"AQI="}]`
	head := func(ledgerFile) error { return nil }
	item := func(ledgerFileItem) error { return nil }
	for name, tc := range map[string]struct {
		prefix, suffix, want string
	}{
		"unknown key, large value":        {`{` + fields + `,"note":`, `,` + entries + `}`, "over 4096 bytes"},
		"large key":                       {`{` + fields + `,`, `:1,` + entries + `}`, "over 4096 bytes"},
		"large known value":               {`{` + fields + `,"exported":`, `,` + entries + `}`, "over 4096 bytes"},
		"large value before entries come": {`{"exported":`, `,` + fields + `,` + entries + `}`, "over 4096 bytes"},
	} {
		src := newLongValue(tc.prefix, 64<<20, tc.suffix)
		_, err := readLedgerFile(src, head, item)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%s: %v", name, err)
		}
		if src.read > 1<<20 {
			t.Fatalf("%s: read %d bytes before refusing", name, src.read)
		}
	}
	for _, src := range []string{
		`{` + fields + `,` + entries + `,` + entries + `}`,
		`{` + entries + `,` + fields + `,` + entries + `}`,
		`{` + fields + `,"pool":8,` + entries + `}`,
		`{` + fields + `,"note":1,"note":2,` + entries + `}`,
	} {
		if _, err := readLedgerFile(strings.NewReader(src), head, item); err == nil || !strings.Contains(err.Error(), "appears twice") {
			t.Fatalf("%s: %v", src, err)
		}
	}
}

// W-20 (D192): a header field a later release adds is read under the same
// bound and dropped, so this release still imports the file; at most
// MaxLedgerUnknownKeys of them.
func TestLedgerHeaderSkipsUnknownKeys(t *testing.T) {
	const fields = `"kind":"poo-ledger","format":1,"series":"0000000000000000000000000000000000000000000000000000000000000001","pool":7`
	const entries = `"entries":[{"height":1,"item":"AQI="}]`
	var got ledgerFile
	n := 0
	src := `{"signer":{"key":"abc","sig":[1,2,3]},` + fields + `,"note":"from a later release",` + entries + `}`
	_, err := readLedgerFile(strings.NewReader(src), func(h ledgerFile) error { got = h; return nil }, func(ledgerFileItem) error { n++; return nil })
	if err != nil || got.Pool != 7 || n != 1 {
		t.Fatalf("unknown fields: %v %+v %d", err, got, n)
	}
	var many strings.Builder
	many.WriteString(`{` + fields)
	for i := 0; i <= MaxLedgerUnknownKeys; i++ {
		fmt.Fprintf(&many, `,"x%d":0`, i)
	}
	many.WriteString(`,` + entries + `}`)
	_, err = readLedgerFile(strings.NewReader(many.String()), func(ledgerFile) error { return nil }, func(ledgerFileItem) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "fields format 1 does not define") {
		t.Fatalf("too many unknown fields: %v", err)
	}
}
