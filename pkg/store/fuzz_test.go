package store_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/store"
)

// FuzzOpen opens a log file of arbitrary bytes: Open must never panic,
// must keep only whole records, and the cut must leave a file that opens
// again to the same records.
func FuzzOpen(f *testing.F) {
	dir := f.TempDir()
	seed := filepath.Join(dir, "seed.log")
	l, _, err := store.Open(seed, store.Options{NoSync: true})
	if err != nil {
		f.Fatal(err)
	}
	l.Append(store.Record{Kind: store.KindTx, Data: []byte("tx")})
	l.Append(store.Record{Kind: store.KindLayer, Data: make([]byte, 40)})
	l.Close()
	b, _ := os.ReadFile(seed)
	f.Add(b)
	f.Add(b[:len(b)-3])
	f.Fuzz(func(t *testing.T, b []byte) {
		path := filepath.Join(t.TempDir(), "pool.log")
		if err := os.WriteFile(path, b, 0o600); err != nil {
			t.Fatal(err)
		}
		l, first, err := store.Open(path, store.Options{NoSync: true})
		if err != nil {
			return
		}
		l.Close()
		if first.Cut < 0 || first.Cut > int64(len(b)) {
			t.Fatalf("cut %d of %d bytes", first.Cut, len(b))
		}
		l, again, err := store.Open(path, store.Options{NoSync: true})
		if err != nil {
			t.Fatal(err)
		}
		l.Close()
		if again.Cut != 0 || len(again.Records) != len(first.Records) {
			t.Fatalf("reopened: %d records, cut %d; first %d records", len(again.Records), again.Cut, len(first.Records))
		}
	})
}
