package store_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/store"
)

func recs(n int) []store.Record {
	out := make([]store.Record, n)
	for i := range out {
		out[i] = store.Record{Kind: store.Kind(1 + i%3), Data: []byte(fmt.Sprintf("record %d", i))}
	}
	return out
}

func open(t *testing.T, path string) (*store.Log, store.Opened) {
	t.Helper()
	l, op, err := store.Open(path, store.Options{})
	if err != nil {
		t.Fatal(err)
	}
	return l, op
}

func same(t *testing.T, got, want []store.Record) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%d records, want %d", len(got), len(want))
	}
	for i := range got {
		if got[i].Kind != want[i].Kind || !bytes.Equal(got[i].Data, want[i].Data) {
			t.Fatalf("record %d differs", i)
		}
	}
}

func TestAppendAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a", "b.log")
	l, op := open(t, path)
	if len(op.Records) != 0 || op.Cut != 0 {
		t.Fatalf("new log: %+v", op)
	}
	want := recs(10)
	want = append(want, store.Record{Kind: store.KindTx}) // empty data is fine
	for _, r := range want {
		if err := l.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	if l.Records() != len(want) {
		t.Fatalf("Records = %d", l.Records())
	}
	if err := l.Close(); err != nil {
		t.Fatal(err)
	}
	l, op = open(t, path)
	defer l.Close()
	same(t, op.Records, want)
	if op.Cut != 0 {
		t.Fatalf("cut %d bytes from a clean log", op.Cut)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
}

// Every way a crash can cut the last record leaves all earlier records.
func TestTornTail(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.log")
	l, _ := open(t, path)
	want := recs(4)
	for _, r := range want {
		l.Append(r)
	}
	l.Close()
	full, _ := os.ReadFile(path)
	last := 9 + len(want[3].Data)
	for cut := 1; cut < last; cut++ {
		p := filepath.Join(dir, fmt.Sprintf("cut%d.log", cut))
		os.WriteFile(p, full[:len(full)-cut], 0o600)
		l, op := open(t, p)
		same(t, op.Records, want[:3])
		if op.Cut != int64(last-cut) {
			t.Fatalf("cut %d: dropped %d bytes, want %d", cut, op.Cut, last-cut)
		}
		// The log is usable after the cut.
		if err := l.Append(want[3]); err != nil {
			t.Fatal(err)
		}
		l.Close()
		l, op = open(t, p)
		same(t, op.Records, want)
		l.Close()
	}
}

func TestCorruptRecordStopsReplay(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.log")
	l, _ := open(t, path)
	want := recs(3)
	for _, r := range want {
		l.Append(r)
	}
	l.Close()
	b, _ := os.ReadFile(path)
	// Flip a byte in the second record's data.
	b[8+9+len(want[0].Data)+5+1] ^= 0xff
	os.WriteFile(path, b, 0o600)
	l, op := open(t, path)
	defer l.Close()
	same(t, op.Records, want[:1])
	if op.Cut == 0 {
		t.Fatal("corruption not reported")
	}
}

func TestHugeLengthIsNotAllocated(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.log")
	b := append([]byte("PoO-log\x01"), 1, 0xff, 0xff, 0xff, 0xff)
	os.WriteFile(path, b, 0o600)
	l, op := open(t, path)
	defer l.Close()
	if len(op.Records) != 0 || op.Cut != 5 {
		t.Fatalf("%+v", op)
	}
}

func TestNotALog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.log")
	os.WriteFile(path, []byte("something else entirely"), 0o600)
	if _, _, err := store.Open(path, store.Options{}); !errors.Is(err, store.ErrNotLog) {
		t.Fatalf("err = %v", err)
	}
}

func TestRewrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.log")
	l, _ := open(t, path)
	for _, r := range recs(20) {
		l.Append(r)
	}
	keep := recs(5)
	if err := l.Rewrite(keep); err != nil {
		t.Fatal(err)
	}
	if l.Records() != 5 {
		t.Fatalf("Records = %d", l.Records())
	}
	extra := store.Record{Kind: store.KindLayer, Data: []byte("after")}
	if err := l.Append(extra); err != nil {
		t.Fatal(err)
	}
	l.Close()
	if _, err := os.Stat(path + ".tmp"); !os.IsNotExist(err) {
		t.Fatal("temporary file left behind")
	}
	l, op := open(t, path)
	defer l.Close()
	same(t, op.Records, append(keep, extra))
}

func TestNoSync(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.log")
	l, _, err := store.Open(path, store.Options{NoSync: true})
	if err != nil {
		t.Fatal(err)
	}
	l.Append(recs(1)[0])
	l.Close()
	l, op := open(t, path)
	defer l.Close()
	same(t, op.Records, recs(1))
}

func TestKindString(t *testing.T) {
	for k, want := range map[store.Kind]string{store.KindEntry: "entry", store.KindLayer: "layer", store.KindTx: "tx", 9: "kind(9)"} {
		if k.String() != want {
			t.Errorf("%d: %q", k, k.String())
		}
	}
}

// A record over MaxRecordSize is kept: no item is too large to store.
func TestTooLarge(t *testing.T) {
	path := filepath.Join(t.TempDir(), "p.log")
	l, _ := open(t, path)
	big := store.Record{Kind: 1, Data: make([]byte, store.MaxRecordSize+1)}
	big.Data[len(big.Data)-1] = 7
	if err := l.Append(big); err != nil {
		t.Fatal(err)
	}
	l.Close()
	l, op := open(t, path)
	defer l.Close()
	same(t, op.Records, []store.Record{big})
}

// A record larger than one on disk may be is written in parts and read
// back whole; a crash that leaves only some of its parts loses the record,
// and the log is cut before them (D117).
func TestLargeRecordInParts(t *testing.T) {
	defer store.SetPieceSize(4)()
	path := filepath.Join(t.TempDir(), "p.log")
	l, _ := open(t, path)
	want := []store.Record{
		{Kind: store.KindEntry, Data: []byte("a record of many pieces")},
		{Kind: store.KindTx, Data: []byte("abcd")},
		{Kind: store.KindLayer, Data: []byte("abcdefgh")},
	}
	for _, r := range want {
		if err := l.Append(r); err != nil {
			t.Fatal(err)
		}
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(l.Close())
	l, op := open(t, path)
	same(t, op.Records, want)
	must(l.Close())

	// Drop the last piece of the last record: its parts are cut off.
	info, err := os.Stat(path)
	must(err)
	must(os.Truncate(path, info.Size()-(9+4)))
	l, op = open(t, path)
	same(t, op.Records, want[:2])
	if op.Cut == 0 {
		t.Fatal("the torn record's parts were not cut")
	}
	must(l.Append(want[2]))
	must(l.Close())
	l, op = open(t, path)
	same(t, op.Records, want)
	must(l.Close())
}
