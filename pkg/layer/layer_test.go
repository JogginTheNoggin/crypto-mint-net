package layer

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

func sha() hash.Func {
	f, _ := hash.Lookup(hash.SHA256)
	return f
}

func TestGrindKeepsBest(t *testing.T) {
	h := sha()
	e := h([]byte("entry"))
	best, s := Grind(h, e, 0, 4000)
	if best.Strength(h) != s || s < 6 {
		t.Fatalf("best strength %d, recomputed %d", s, best.Strength(h))
	}
	// No nonce in the range beats it.
	for n := uint64(0); n < 4000; n++ {
		if (Layer{Entry: e, Nonce: n}).Strength(h) > s {
			t.Fatal("Grind missed a stronger layer")
		}
	}
	if best.Credit(h).String() == "0" {
		t.Fatal("credit is at least 1")
	}
}

func TestLayerBoundToEntry(t *testing.T) {
	h := sha()
	a, b := h([]byte("a")), h([]byte("b"))
	l := Layer{Entry: a, Nonce: 7}
	if l.Hash(h) == (Layer{Entry: b, Nonce: 7}).Hash(h) {
		t.Fatal("layer hash must depend on the entry")
	}
}

func TestEncodeDecode(t *testing.T) {
	h := sha()
	l := Layer{Entry: h([]byte("e")), Nonce: 1 << 40}
	b := l.Encode()
	if len(b) != Size {
		t.Fatal("size")
	}
	back, err := Decode(b)
	if err != nil || back != l {
		t.Fatal("round trip")
	}
	if _, err := Decode(b[:10]); err == nil {
		t.Fatal("short must fail")
	}
}

func TestGrindTo(t *testing.T) {
	h := sha()
	e := h([]byte("x"))
	l, ok := GrindTo(h, e, 0, 8, 1<<20)
	if !ok || l.Strength(h) < 8 {
		t.Fatal("GrindTo failed")
	}
	if _, ok := GrindTo(h, e, 0, 200, 10); ok {
		t.Fatal("impossible strength found")
	}
}
