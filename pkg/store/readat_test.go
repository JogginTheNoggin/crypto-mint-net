package store_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/store"
)

// D172: every record has a location; ReadAt reads it back, also a record
// split into parts, the same locations come back from OpenFunc, and a
// compaction starts a new generation whose old locations are refused.
func TestReadAtLocations(t *testing.T) {
	defer store.SetPieceSize(16)()
	path := filepath.Join(t.TempDir(), "p.log")
	l, _, err := store.Open(path, store.Options{NoSync: true})
	if err != nil {
		t.Fatal(err)
	}
	recs := []store.Record{
		{Kind: store.KindEntry, Data: []byte("first entry")},
		{Kind: store.KindLayer, Data: bytes.Repeat([]byte{7}, 50)}, // in parts
		{Kind: store.KindTx, Data: []byte{}},
		{Kind: store.KindEntry, Data: bytes.Repeat([]byte{9}, 16)},
	}
	var locs []store.Loc
	for _, r := range recs {
		_, loc, err := l.AppendLaterAt(r)
		if err != nil {
			t.Fatal(err)
		}
		locs = append(locs, loc)
	}
	for i, loc := range locs {
		got, err := l.ReadAt(loc)
		if err != nil || got.Kind != recs[i].Kind || !bytes.Equal(got.Data, recs[i].Data) {
			t.Fatalf("record %d: %v %+v", i, err, got)
		}
	}
	// A location one byte off, or too long, is never read as a record.
	bad := locs[1]
	bad.Off++
	if _, err := l.ReadAt(bad); err == nil {
		t.Fatal("an offset one byte off was read")
	}
	bad = locs[0]
	bad.Len += 3
	if _, err := l.ReadAt(bad); err == nil {
		t.Fatal("a location too long was read")
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}

	var again []store.Loc
	l, _, err = store.OpenFunc(path, store.Options{NoSync: true}, func(ol *store.Log, r store.Record, loc store.Loc) error {
		again = append(again, loc)
		// A record already handed over reads back before Open returns.
		back, err := ol.ReadAt(again[0])
		if err != nil || len(again) == 1 && !bytes.Equal(back.Data, r.Data) {
			t.Fatalf("ReadAt during open: %v", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != len(locs) || l.Records() != len(locs) {
		t.Fatalf("%d locations on reopen", len(again))
	}
	for i := range locs {
		if again[i] != locs[i] {
			t.Fatalf("record %d: %+v, then %+v", i, locs[i], again[i])
		}
	}
	// Appending after a reopen continues at the right place.
	_, loc, err := l.AppendLaterAt(store.Record{Kind: store.KindTx, Data: []byte("after")})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := l.ReadAt(loc); err != nil || string(got.Data) != "after" {
		t.Fatalf("after reopen: %v", err)
	}

	// A compaction, streamed, may read the old file as it goes.
	newLocs, err := l.RewriteFunc(2, func(i int) (store.Record, error) {
		return l.ReadAt(locs[3-i])
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := l.ReadAt(locs[0]); !errors.Is(err, store.ErrStaleLoc) {
		t.Fatalf("an old generation's location: %v", err)
	}
	for i, want := range []store.Record{recs[3], recs[2]} {
		got, err := l.ReadAt(newLocs[i])
		if err != nil || got.Kind != want.Kind || !bytes.Equal(got.Data, want.Data) {
			t.Fatalf("after compaction %d: %v", i, err)
		}
	}
	_, loc, err = l.AppendLaterAt(store.Record{Kind: store.KindTx, Data: []byte("tail")})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := l.ReadAt(loc); err != nil || string(got.Data) != "tail" {
		t.Fatalf("append after compaction: %v", err)
	}
	// A changed byte on disk fails the checksum.
	l.Close()
	b, _ := os.ReadFile(path)
	b[newLocs[0].Off+6] ^= 1
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	l, _, err = store.OpenFunc(path, store.Options{NoSync: true}, func(*store.Log, store.Record, store.Loc) error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if _, err := l.ReadAt(store.Loc{Off: newLocs[0].Off, Len: newLocs[0].Len}); err == nil {
		t.Fatal("a corrupt record was read")
	}
}
