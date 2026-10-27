package series

import (
	"bytes"
	"testing"
)

// FuzzDecodeParams checks that DecodeParams never panics, that what it
// accepts re-encodes to the same bytes, and that building a series from
// it never panics.
func FuzzDecodeParams(f *testing.F) {
	f.Add(base().Encode())
	f.Add([]byte{FormatVersion})
	f.Fuzz(func(t *testing.T, b []byte) {
		p, err := DecodeParams(b)
		if err != nil {
			return
		}
		if !bytes.Equal(p.Encode(), b) {
			t.Fatal("re-encoding differs")
		}
		New(p)
	})
}
