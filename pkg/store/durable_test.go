package store_test

import (
	"errors"
	"os"
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

// RED-010: a failed fsync stops the log. A later sync that succeeds does
// not make the records of the failed one durable, and the log takes no
// more appends.
func TestFailedSyncIsLatched(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pool.log")
	l, _, err := store.Open(path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	t1, err := l.AppendLater(store.Record{Kind: store.KindTx, Data: []byte{1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Durable(t1); err != nil {
		t.Fatal(err)
	}
	t2, _ := l.AppendLater(store.Record{Kind: store.KindTx, Data: []byte{2}})
	t3, _ := l.AppendLater(store.Record{Kind: store.KindTx, Data: []byte{3}})
	// The first sync fails and every later one would succeed, as on Linux
	// after a writeback error.
	calls := 0
	restore := store.SetFileSync(func(f *os.File) error {
		calls++
		if calls == 1 {
			return errors.New("injected I/O error")
		}
		return f.Sync()
	})
	defer restore()
	if err := l.Durable(t2); !errors.Is(err, store.ErrFailed) {
		t.Fatalf("failed sync: %v", err)
	}
	if err := l.Durable(t3); !errors.Is(err, store.ErrFailed) {
		t.Fatalf("a waiter behind the failed sync was told %v", err)
	}
	if calls != 1 {
		t.Fatalf("the failed sync was retried: %d calls", calls)
	}
	if err := l.Durable(t1); err != nil {
		t.Fatalf("a record synced before the failure: %v", err)
	}
	if _, err := l.AppendLater(store.Record{Kind: store.KindTx, Data: []byte{4}}); !errors.Is(err, store.ErrFailed) {
		t.Fatalf("append after a failed sync: %v", err)
	}
	if err := l.Rewrite(nil); !errors.Is(err, store.ErrFailed) {
		t.Fatalf("rewrite after a failed sync: %v", err)
	}
	if err := l.Close(); !errors.Is(err, store.ErrFailed) {
		t.Fatalf("close after a failed sync: %v", err)
	}
}
