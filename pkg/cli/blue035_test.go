package cli

import (
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
		"unknown key, large value":        {`{` + fields + `,"note":`, `,` + entries + `}`, `no field "note"`},
		"large known value":               {`{` + fields + `,"exported":`, `,` + entries + `}`, "over 4096 bytes"},
		"large value before entries come": {`{"exported":`, `,` + fields + `,` + entries + `}`, "over 4096 bytes"},
	} {
		src := newLongValue(tc.prefix, 64<<20, tc.suffix)
		err := readLedgerFile(src, head, item)
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
	} {
		if err := readLedgerFile(strings.NewReader(src), head, item); err == nil || !strings.Contains(err.Error(), "appears twice") {
			t.Fatalf("%s: %v", src, err)
		}
	}
}
