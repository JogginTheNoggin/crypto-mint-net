package layer

import (
	"bytes"
	"testing"
)

// FuzzDecode checks that Decode accepts exactly the Size-byte inputs and
// that what it accepts re-encodes to the same bytes.
func FuzzDecode(f *testing.F) {
	f.Add(make([]byte, Size))
	f.Add([]byte{1})
	f.Fuzz(func(t *testing.T, b []byte) {
		l, err := Decode(b)
		if (err == nil) != (len(b) == Size) {
			t.Fatalf("%d bytes: err %v", len(b), err)
		}
		if err == nil && !bytes.Equal(l.Encode(), b) {
			t.Fatal("re-encoding differs")
		}
	})
}
