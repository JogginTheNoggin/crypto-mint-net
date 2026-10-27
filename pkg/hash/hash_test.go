package hash

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestStrength(t *testing.T) {
	cases := []struct {
		hex  string
		want int
	}{
		{"ff00000000000000000000000000000000000000000000000000000000000000", 0},
		{"8000000000000000000000000000000000000000000000000000000000000000", 0},
		{"7f00000000000000000000000000000000000000000000000000000000000000", 1},
		{"000bd07500000000000000000000000000000000000000000000000000000000", 12},
		{"0020236d00000000000000000000000000000000000000000000000000000000", 10},
		{"00022798ffffffffffffffffffffffffffffffffffffffffffffffffffffffff", 14},
		{"0000000000000000000000000000000000000000000000000000000000000001", 255},
		{"0000000000000000000000000000000000000000000000000000000000000000", 256},
	}
	for _, c := range cases {
		if got := MustParse(c.hex).Strength(); got != c.want {
			t.Errorf("Strength(%s) = %d, want %d", c.hex[:8], got, c.want)
		}
	}
}

func TestSHA256KnownVector(t *testing.T) {
	f, err := Lookup(SHA256)
	if err != nil {
		t.Fatal(err)
	}
	// SHA-256("abc"), split across parts to check concatenation.
	got := f([]byte("a"), []byte("bc"))
	want := MustParse("ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad")
	if got != want {
		t.Fatalf("got %s", got)
	}
}

func TestUnknown(t *testing.T) {
	if _, err := Lookup(999); !errors.Is(err, ErrUnknown) {
		t.Fatal("want ErrUnknown")
	}
}

func TestOrderAndText(t *testing.T) {
	a := MustParse("0100000000000000000000000000000000000000000000000000000000000000")
	b := MustParse("0200000000000000000000000000000000000000000000000000000000000000")
	if !a.Less(b) || b.Less(a) || a.Compare(a) != 0 {
		t.Fatal("ordering wrong")
	}
	if a.Short() != "01000000" {
		t.Fatal(a.Short())
	}
	js, _ := json.Marshal(a)
	var back Hash
	if err := json.Unmarshal(js, &back); err != nil || back != a {
		t.Fatal("json round trip failed")
	}
	if _, err := Parse("zz"); err == nil {
		t.Fatal("want error")
	}
	if _, err := Parse("00"); err == nil {
		t.Fatal("want length error")
	}
}

// Every hash in the list against a published answer for "abc", split across
// parts to check concatenation.
func TestKnownAnswers(t *testing.T) {
	want := map[ID]string{
		SHA256:     "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		SHA512_256: "53048e2681941ef99b2e29b76b4c7dabe4c2d0c634fc6d46e0e2f13107e7af23",
		SHA3_256:   "3a985da74fe225b2045c172d6bd390bd855f086e3e9d525b46bfe24511431532",
		BLAKE2b256: "bddd813c634239723171ef3fee98579b94964e3bb1cb3e427262c8c068d52319",
		BLAKE3:     "6437b3ac38465133ffb63b75273a8db548c558465d79db03fd359c6cd5bd9d85",
	}
	if len(IDs()) != len(want) {
		t.Fatalf("%d hashes known, %d tested", len(IDs()), len(want))
	}
	for _, id := range IDs() {
		f, err := Lookup(id)
		if err != nil {
			t.Fatal(err)
		}
		if got := f([]byte("a"), []byte("bc")); got != MustParse(want[id]) {
			t.Errorf("%s: got %s", id.Name(), got)
		}
		if back, ok := ByName(id.Name()); !ok || back != id {
			t.Errorf("%s: name does not map back", id.Name())
		}
	}
}
