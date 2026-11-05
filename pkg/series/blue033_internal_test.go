package series

import (
	"encoding/binary"
	"sync"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// GUI ORANGE-024 (BLUE-033): the variable series set grows in place, so
// adding the most series a node follows costs about one insert each.
func TestBlue033VariableSetGrowsInPlace(t *testing.T) {
	id := func(i int) hash.Hash {
		var h hash.Hash
		binary.BigEndian.PutUint64(h[:], uint64(i)|1<<63)
		h[31] = 0xb3
		return h
	}
	const n = 4096
	start := time.Now()
	var wg sync.WaitGroup
	for w := range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := w; i < n; i += 4 {
				addVariableID(id(i))
				IsVariableID(id(i)) // reads run beside the adds
			}
		}()
	}
	wg.Wait()
	if d := time.Since(start); d > 5*time.Second {
		t.Fatalf("adding %d ids took %s", n, d)
	}
	for i := range n {
		if !IsVariableID(id(i)) {
			t.Fatalf("id %d missing", i)
		}
	}
	if IsVariableID(id(n + 1)) {
		t.Fatal("an id never added reads as variable")
	}
}

func BenchmarkBlue033AddVariableIDs(b *testing.B) {
	for b.Loop() {
		for i := range 4096 {
			var h hash.Hash
			binary.BigEndian.PutUint64(h[:], uint64(i))
			h[30] = 0xbe
			addVariableID(h)
		}
	}
}
