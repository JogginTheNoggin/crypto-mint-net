package store_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/store"
)

func BenchmarkAppend(b *testing.B) {
	for _, sync := range []bool{false, true} {
		b.Run(fmt.Sprintf("sync=%v", sync), func(b *testing.B) {
			l, _, err := store.Open(filepath.Join(b.TempDir(), "pool.log"), store.Options{NoSync: !sync})
			if err != nil {
				b.Fatal(err)
			}
			defer l.Close()
			rec := store.Record{Kind: store.KindLayer, Data: make([]byte, 40)}
			b.SetBytes(int64(len(rec.Data)))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := l.Append(rec); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkOpen reads back a log of 10,000 records, the first step of a
// node's start-up replay.
func BenchmarkOpen(b *testing.B) {
	path := filepath.Join(b.TempDir(), "pool.log")
	l, _, err := store.Open(path, store.Options{NoSync: true})
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < 10000; i++ {
		l.Append(store.Record{Kind: store.KindEntry, Data: make([]byte, 300)})
	}
	l.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		l, o, err := store.Open(path, store.Options{NoSync: true})
		if err != nil || len(o.Records) != 10000 {
			b.Fatal(err, len(o.Records))
		}
		l.Close()
	}
}
