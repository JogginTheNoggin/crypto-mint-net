package series

import (
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
)

func variableForTest(t *testing.T) *Series {
	t.Helper()
	iss := hash.Hash{1}
	s, err := New(Params{HashID: 1, Mints: 8, Supply: 100, PoolSize: 4, PolicyTypes: []policy.TypeID{1}, KeyAlgs: []policy.AlgID{1},
		Issuer: &iss, SupplyType: Variable, PartSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	// Its own chain, not the one this process shares for the id
	// (ORANGE-035), so each test starts from no part id worked out.
	s.parts = &partChain{}
	return s
}

// naive works out part ids 1 to n one after another.
func naive(s *Series, n uint64) []hash.Hash {
	ids := make([]hash.Hash, n)
	prev := s.ID()
	for k := uint64(1); k <= n; k++ {
		prev = s.partID(k, prev)
		ids[k-1] = prev
	}
	return ids
}

// RED-070, D165: the chain keeps at most maxPartMarks ids, thinning its
// marks as it grows, and every id is the one the chain itself defines, in
// any order and from many callers at once.
func TestPartChainBoundedAndExact(t *testing.T) {
	defer func(a, b uint64, m int) { firstPartStride, partChunk, maxPartMarks = a, b, m }(firstPartStride, partChunk, maxPartMarks)
	firstPartStride, partChunk, maxPartMarks = 4, 64, 16
	s := variableForTest(t)
	const n = 5000
	want := naive(s, n)
	// Far out first: the marks thin out to stay within 16.
	if id, err := s.PartID(n); err != nil || id != want[n-1] {
		t.Fatalf("part %d: %v", n, err)
	}
	if len(s.parts.marks) > maxPartMarks || s.parts.stride <= 4 {
		t.Fatalf("%d marks at stride %d", len(s.parts.marks), s.parts.stride)
	}
	// Then every part, backwards, in order, and from several callers.
	for k := uint64(n); k >= 1; k-- {
		if id, _ := s.PartID(k); id != want[k-1] {
			t.Fatalf("part %d differs", k)
		}
	}
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for k := uint64(1 + g); k <= n; k += 37 {
				if id, _ := s.PartID(k); id != want[k-1] {
					t.Errorf("part %d differs", k)
					return
				}
			}
		}(g)
	}
	wg.Wait()
	// A fresh series asked from many callers at once, far out.
	f := variableForTest(t)
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			k := uint64(n - g*500)
			if id, _ := f.PartID(k); id != want[k-1] {
				t.Errorf("part %d differs", k)
			}
		}(g)
	}
	wg.Wait()
}

// RED-070: part 8 million out (Red's 30 million took 1.5 GB; the old chain held 256 MB for 8 million) costs time,
// not memory: the heap grows by at most the marks' 2 MiB.
func TestPartIDFarOutInBoundedMemory(t *testing.T) {
	if testing.Short() {
		t.Skip("8 million hashes")
	}
	s := variableForTest(t)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	if _, err := s.PartID(8_000_000); err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	if grew := int64(after.HeapAlloc) - int64(before.HeapAlloc); grew > 16<<20 {
		t.Fatalf("the heap grew by %d bytes", grew)
	}
	if len(s.parts.marks) > maxPartMarks {
		t.Fatalf("%d marks", len(s.parts.marks))
	}
	// While one call works far out, another for a part already known is
	// answered at once: the lock is held one chunk at a time.
	f := variableForTest(t)
	f.PartID(10)
	done := make(chan struct{})
	go func() { f.PartID(5_000_000); close(done) }()
	time.Sleep(50 * time.Millisecond)
	start := time.Now()
	f.PartID(9)
	waited := time.Since(start)
	select {
	case <-done:
		t.Skip("the long call ended before the check; too fast to tell")
	default:
	}
	// One chunk takes milliseconds, the whole call seconds. The bound is
	// loose so a loaded machine (or -race) does not fail it.
	if waited > time.Second {
		t.Fatalf("a known part waited %v behind a long call", waited)
	}
	<-done
}

// RED-075, D167: a release the node checked in an earlier process seeds
// the previous part's id, so its next check costs no chain; every other
// part is still worked out from the chain.
func TestTrustReleaseSeedsOnlyThatPart(t *testing.T) {
	iss := hash.Hash{1}
	// A series of its own id, with the chain the process shares for it.
	f, err := New(Params{HashID: 1, Mints: 8, Supply: 7777, PoolSize: 4, PolicyTypes: []policy.TypeID{1}, KeyAlgs: []policy.AlgID{1},
		Issuer: &iss, SupplyType: Variable, PartSize: 4})
	if err != nil {
		t.Fatal(err)
	}
	ids := naive(f, 50)
	seed := Release{Series: f.ID(), Part: 42, Previous: hash.Hash{9}}
	TrustRelease(seed)
	if got, _ := f.PartID(41); got != seed.Previous {
		t.Fatal("the seed was not used")
	}
	if !ReleaseChecked(seed) || ReleaseChecked(Release{Series: f.ID(), Part: 42, Previous: ids[40]}) {
		t.Fatal("ReleaseChecked")
	}
	for _, k := range []uint64{40, 43, 50} {
		if got, _ := f.PartID(k); got != ids[k-1] {
			t.Fatalf("part %d is not the chain's", k)
		}
	}
	// Once the chain itself has passed the seed, the chain answers.
	if got, _ := f.PartID(41); got != ids[40] {
		t.Fatal("the seed outlived the chain")
	}
}
