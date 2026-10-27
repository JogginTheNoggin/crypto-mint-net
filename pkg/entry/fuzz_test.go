package entry_test

import (
	"bytes"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// FuzzDecode checks that Decode never panics, that what it accepts
// re-encodes to the same bytes, and that checking it never panics.
func FuzzDecode(f *testing.F) {
	s := testutil.ExampleSeries()
	k := testutil.NewKeys(s)
	e, _, err := entry.Build(s, s.PoolID(1), []*tx.Tx{k.Claim(4, testutil.P("alice", 100)), k.Claim(5, testutil.P("bob", 100))}, 1)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(e.Encode())
	f.Add((&entry.Entry{Parent: s.PoolID(0)}).Encode())
	f.Fuzz(func(t *testing.T, b []byte) {
		got, err := entry.Decode(b)
		if err != nil {
			return
		}
		if !bytes.Equal(got.Encode(), b) {
			t.Fatal("re-encoding differs")
		}
		// Checking must not panic, whatever the bundle holds.
		entry.CheckStructure(s, 1, got)
	})
}
