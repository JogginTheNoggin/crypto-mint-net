package encoding

import (
	"bytes"
	"errors"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	w := NewWriter(0)
	w.U8(7).U16(0x0102).U32(0x03040506).U64(0x0708090a0b0c0d0e).Bool(true).Bool(false).
		VarBytes([]byte("abc")).Count(2).Raw([]byte{0xff, 0xee})

	want := []byte{
		7,
		1, 2,
		3, 4, 5, 6,
		7, 8, 9, 10, 11, 12, 13, 14,
		1, 0,
		0, 0, 0, 3, 'a', 'b', 'c',
		0, 0, 0, 2,
		0xff, 0xee,
	}
	if !bytes.Equal(w.Bytes(), want) {
		t.Fatalf("encoding mismatch:\n got %x\nwant %x", w.Bytes(), want)
	}

	r := NewReader(w.Bytes())
	if r.U8() != 7 || r.U16() != 0x0102 || r.U32() != 0x03040506 || r.U64() != 0x0708090a0b0c0d0e {
		t.Fatal("integer mismatch")
	}
	if !r.Bool() || r.Bool() {
		t.Fatal("bool mismatch")
	}
	if string(r.VarBytes(0)) != "abc" {
		t.Fatal("varbytes mismatch")
	}
	if r.Count(0, 1) != 2 {
		t.Fatal("count mismatch")
	}
	if !bytes.Equal(r.Raw(2), []byte{0xff, 0xee}) {
		t.Fatal("raw mismatch")
	}
	if err := r.Finish(); err != nil {
		t.Fatal(err)
	}
}

func TestShortInput(t *testing.T) {
	r := NewReader([]byte{0, 0})
	r.U32()
	if !errors.Is(r.Err(), ErrShort) {
		t.Fatalf("want ErrShort, got %v", r.Err())
	}
	// Errors are sticky.
	r.U8()
	if !errors.Is(r.Finish(), ErrShort) {
		t.Fatal("error not sticky")
	}
}

func TestTrailing(t *testing.T) {
	r := NewReader([]byte{1, 2})
	r.U8()
	if !errors.Is(r.Finish(), ErrTrailing) {
		t.Fatal("want ErrTrailing")
	}
}

func TestInvalidBool(t *testing.T) {
	r := NewReader([]byte{2})
	r.Bool()
	if r.Err() == nil {
		t.Fatal("want error for boolean 2")
	}
}

func TestVarBytesLimit(t *testing.T) {
	w := NewWriter(0).VarBytes(make([]byte, 10))
	r := NewReader(w.Bytes())
	r.VarBytes(5)
	if r.Err() == nil {
		t.Fatal("want limit error")
	}
	// A length claiming more than the input is refused without allocating.
	r = NewReader([]byte{0xff, 0xff, 0xff, 0xff})
	r.VarBytes(0)
	if !errors.Is(r.Err(), ErrShort) {
		t.Fatalf("want ErrShort, got %v", r.Err())
	}
}

func TestCountLimits(t *testing.T) {
	r := NewReader(NewWriter(0).Count(1000).Bytes())
	r.Count(0, 4)
	if !errors.Is(r.Err(), ErrShort) {
		t.Fatal("count larger than input must fail")
	}
	r = NewReader(NewWriter(0).Count(3).Raw(make([]byte, 30)).Bytes())
	r.Count(2, 1)
	if r.Err() == nil {
		t.Fatal("count above max must fail")
	}
}
