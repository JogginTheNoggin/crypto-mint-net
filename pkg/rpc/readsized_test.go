package rpc

import (
	"bytes"
	"runtime"
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

// ORANGE-026: a body stated past the hint ends at exactly its stated
// size. Starting at the hint, a body just over a power of two was copied
// whole once more for its last bytes: about 3x its size set aside over
// the read, now about 2x.
func TestReadSizedEndsAtStated(t *testing.T) {
	const hint = 64 << 10
	const stated = 1<<20 + 100
	data := bytes.Repeat([]byte("a"), stated)
	runtime.GC()
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	got, err := readSized(bytes.NewReader(data), stated, hint)
	runtime.ReadMemStats(&m1)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatalf("read %d bytes, %v", len(got), err)
	}
	if cap(got) != stated+1 {
		t.Fatalf("ended with room for %d, want %d", cap(got), stated+1)
	}
	if a := m1.TotalAlloc - m0.TotalAlloc; a > stated*9/4 {
		t.Fatalf("set aside %d bytes over the read of %d, want at most %d", a, stated, stated*9/4)
	}
}
