package rpc

import (
	"bytes"
	"strings"
	"testing"
	"testing/iotest"
)

// TestReadSized: the entry body's buffer grows toward the stated length
// as bytes arrive. A false length sets aside at most the hint, or twice
// what came; an honest one ends at its size, past the hint as well
// (D147).
func TestReadSized(t *testing.T) {
	const hint = 64
	for _, c := range []struct {
		name         string
		sent, stated int64
		maxCap       int
	}{
		{"honest, under the hint", 40, 40, 513},
		{"honest, past the hint", 5000, 5000, 5001},
		{"unknown length", 5000, -1, 2 * 5000},
		{"states a lot, sends little", 10, 1 << 40, 513},
		{"states a lot, sends past the hint", 3000, 1 << 40, 2 * 3000},
	} {
		data := bytes.Repeat([]byte("ab"), int(c.sent)/2)
		got, err := readSized(iotest.OneByteReader(bytes.NewReader(data)), c.stated, hint)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if !bytes.Equal(got, data) {
			t.Fatalf("%s: read %d bytes, want %d", c.name, len(got), len(data))
		}
		if cap(got) > c.maxCap {
			t.Fatalf("%s: set aside %d bytes for %d, want at most %d", c.name, cap(got), c.sent, c.maxCap)
		}
	}
	if _, err := readSized(iotest.ErrReader(iotest.ErrTimeout), 10, hint); err == nil {
		t.Fatal("a failed read was not reported")
	}
	if got, _ := readSized(strings.NewReader(""), 0, hint); len(got) != 0 {
		t.Fatalf("an empty body read %q", got)
	}
}
