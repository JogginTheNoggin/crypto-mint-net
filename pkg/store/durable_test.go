package store_test

import (
	"path/filepath"
	"sync"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/store"
)

// TestDurableGroupCommit appends from many goroutines, each waiting for
// its own record outside the caller's lock, while the log is rewritten and
// finally closed, and checks that every record is read back and no wait
// fails.
func TestDurableGroupCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pool.log")
	l, _, err := store.Open(path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex // the node's lock: every call but Durable runs under it
	var all []store.Record
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				rec := store.Record{Kind: store.KindTx, Data: []byte{byte(g), byte(i)}}
				mu.Lock()
				tk, err := l.AppendLater(rec)
				all = append(all, rec)
				if g == 0 && i == 25 {
					// Compact in the middle of the waits.
					if err := l.Rewrite(all); err != nil {
						t.Error(err)
					}
				}
				mu.Unlock()
				if err != nil {
					t.Error(err)
					return
				}
				if err := l.Durable(tk); err != nil {
					t.Error(err)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	mu.Lock()
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	mu.Unlock()
	l, op, err := store.Open(path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if len(op.Records) != len(all) {
		t.Fatalf("read back %d records, want %d", len(op.Records), len(all))
	}
}
