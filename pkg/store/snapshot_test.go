package store

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// D178: OpenFuncAt reads only the records after where Ready says a
// snapshot ends, counting those before; Tag changes when the bytes it
// covers do, and refuses an offset past the end.
func TestOpenFuncAtAndTag(t *testing.T) {
	path := filepath.Join(t.TempDir(), "1.log")
	l, _, err := Open(path, Options{NoSync: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range []string{"a", "b", "c", "d"} {
		if err := l.Append(Record{Kind: KindTx, Data: []byte(b)}); err != nil {
			t.Fatal(err)
		}
	}
	// The end of the second record.
	mid := int64(len(magic)) + 2*framedSize(1)
	tag, err := Tag(path, mid)
	if err != nil {
		t.Fatal(err)
	}
	l.Close()
	var got []string
	l, _, err = OpenFuncAt(path, Options{NoSync: true}, From{Ready: func(*Log) (int64, int, error) { return mid, 2, nil }},
		func(_ *Log, r Record, _ Loc) error {
			got = append(got, string(r.Data))
			return nil
		})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "c" || got[1] != "d" || l.Records() != 4 {
		t.Fatalf("read %v, %d records", got, l.Records())
	}
	l.Close()
	if again, _ := Tag(path, mid); again != tag {
		t.Fatal("tag changed with nothing changed")
	}
	raw, _ := os.ReadFile(path)
	raw[len(magic)+5] ^= 1 // inside the first record
	os.WriteFile(path, raw, 0o600)
	if changed, _ := Tag(path, mid); changed == tag {
		t.Fatal("tag did not change with the bytes it covers")
	}
	if _, err := Tag(path, int64(len(raw))+1); !errors.Is(err, ErrStaleLoc) {
		t.Fatalf("past the end: %v", err)
	}
	if _, _, err := OpenFuncAt(path, Options{NoSync: true}, From{Ready: func(*Log) (int64, int, error) { return int64(len(raw)) + 9, 0, nil }},
		func(*Log, Record, Loc) error { return nil }); !errors.Is(err, ErrStaleLoc) {
		t.Fatalf("Ready past the end: %v", err)
	}
}
