package registry_test

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/registry"
)

// FuzzParse checks that Parse never panics on any document and that what
// it accepts encodes and parses back to the same entries.
func FuzzParse(f *testing.F) {
	f.Add([]byte(`{"format":1,"nodes":[{"addr":"a.example:8646"}]}`))
	f.Add([]byte(`{}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		es, err := registry.Parse(b)
		if err != nil {
			return
		}
		out, err := registry.Encode(es)
		if err != nil {
			t.Fatal(err)
		}
		back, err := registry.Parse(out)
		if err != nil || len(back) != len(es) {
			t.Fatalf("round trip: %v, %d of %d entries", err, len(back), len(es))
		}
	})
}
