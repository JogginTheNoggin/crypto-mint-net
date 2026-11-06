package cli

import (
	"errors"
	"strings"
	"testing"
)

// readLedgerFile gives the header first and then every entry in order,
// whether the entries come after the header fields (as export writes
// them, read as they stream) or before them (held, then given), and
// refuses what is not a ledger file (ORANGE-043).
func TestReadLedgerFileStreams(t *testing.T) {
	const items = `[{"height":1,"item":"AQI="},{"height":2,"item":"AwQ="},{"height":3,"item":"BQY="}]`
	const fields = `"kind":"poo-ledger","format":1,"series":"` + "0000000000000000000000000000000000000000000000000000000000000001" + `","pool":7`
	read := func(src string) (h ledgerFile, got []string, beforeHead int, err error) {
		headed := false
		_, err = readLedgerFile(strings.NewReader(src), func(f ledgerFile) error {
			h, headed = f, true
			return nil
		}, func(it ledgerFileItem) error {
			if !headed {
				beforeHead++
			}
			got = append(got, string(it.Item))
			return nil
		})
		return
	}
	for _, src := range []string{
		`{` + fields + `,"entries":` + items + `}`,
		`{"entries":` + items + `,` + fields + `}`,
		`{"kind":"poo-ledger","entries":` + items + `,"format":1,"series":"0000000000000000000000000000000000000000000000000000000000000001","pool":7}`,
	} {
		h, got, before, err := read(src)
		if err != nil || h.Kind != LedgerFileKind || h.Format != 1 || h.Pool != 7 || before != 0 {
			t.Fatalf("%s: header %+v, %d items before it, %v", src, h, before, err)
		}
		if strings.Join(got, ",") != "\x01\x02,\x03\x04,\x05\x06" {
			t.Fatalf("%s: items %q", src, got)
		}
	}
	if _, got, _, err := read(`{` + fields + `,"entries":null}`); err != nil || len(got) != 0 {
		t.Fatalf("null entries: %q %v", got, err)
	}
	for _, src := range []string{``, `[]`, `{"kind":`, `{` + fields + `,"entries":{}}`, `{` + fields + `,"entries":[{"item":5}]}`, `{` + fields + `,"entries":[]`} {
		if _, _, _, err := read(src); err == nil || !strings.Contains(err.Error(), "not a ledger file") {
			t.Fatalf("%q: %v", src, err)
		}
	}
	// An error from the header or an item stops the read.
	stop := errors.New("stop")
	if _, err := readLedgerFile(strings.NewReader(`{`+fields+`,"entries":`+items+`}`), func(ledgerFile) error { return stop }, nil); !errors.Is(err, stop) {
		t.Fatalf("head error: %v", err)
	}
	n := 0
	if _, err := readLedgerFile(strings.NewReader(`{`+fields+`,"entries":`+items+`}`), func(ledgerFile) error { return nil }, func(ledgerFileItem) error {
		n++
		return stop
	}); !errors.Is(err, stop) || n != 1 {
		t.Fatalf("item error: %v after %d", err, n)
	}
}
