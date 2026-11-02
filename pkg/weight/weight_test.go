package weight

import "testing"

func TestCreditAndSum(t *testing.T) {
	// Appendix A.2 of the design: own weights 4,096 / 5,120 / 4,096 / 5,120.
	e1 := Credit(12)
	e2 := Credit(12).Add(Credit(10))
	e3 := Credit(12)
	e4 := Credit(10).Add(Credit(12))
	total := e1.Add(e2).Add(e3).Add(e4)
	if total.String() != "18432" {
		t.Fatalf("total = %s", total)
	}
	if got := total.Sub(e1); got.String() != "14336" {
		t.Fatalf("after E1 = %s", got)
	}
}

func TestLargeCredit(t *testing.T) {
	c := Credit(256)
	if _, ok := c.Uint64(); ok {
		t.Fatal("2^256 must not fit in uint64")
	}
	if !c.Add(c).Greater(c) {
		t.Fatal("sum must grow")
	}
}

func TestImmutability(t *testing.T) {
	a := FromUint64(5)
	b := a.Add(FromUint64(3))
	if a.String() != "5" || b.String() != "8" {
		t.Fatal("Add changed its operand")
	}
}

func TestZeroValue(t *testing.T) {
	var w Weight
	if !w.IsZero() || w.Cmp(Zero) != 0 || w.String() != "0" || len(w.Bytes()) != 0 {
		t.Fatal("zero value wrong")
	}
	if !FromUint64(1).Greater(w) {
		t.Fatal("1 > 0")
	}
}

func TestBytesRoundTrip(t *testing.T) {
	for _, w := range []Weight{Zero, FromUint64(1), FromUint64(1 << 40), Credit(200)} {
		back, err := FromBytes(w.Bytes())
		if err != nil || back.Cmp(w) != 0 {
			t.Fatalf("round trip of %s failed", w)
		}
	}
	if _, err := FromBytes([]byte{0, 1}); err == nil {
		t.Fatal("non-minimal encoding must fail")
	}
}

func TestSubNegativePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("want panic")
		}
	}()
	FromUint64(1).Sub(FromUint64(2))
}

func TestText(t *testing.T) {
	var w Weight
	if err := w.UnmarshalText([]byte("12345")); err != nil || w.String() != "12345" {
		t.Fatal("text round trip")
	}
	if err := w.UnmarshalText([]byte("-1")); err == nil {
		t.Fatal("negative must fail")
	}
}
