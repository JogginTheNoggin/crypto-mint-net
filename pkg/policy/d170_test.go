package policy

import (
	"bytes"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
)

// D170, Red's condition 3: decoding a witness needs no series. It is bound
// only by the bytes received, so a witness larger than any old cap decodes
// the same everywhere, and a node that has not built the series can still
// frame it; only Check, which needs the series, judges its size.
func TestDecodeWitnessNeedsNoSeries(t *testing.T) {
	w := Witness{Policy: Policy{Type: 99, Params: bytes.Repeat([]byte{1}, 100_000)}, Data: bytes.Repeat([]byte{2}, 200_000)}
	e := encoding.NewWriter(w.Size())
	w.Encode(e)
	r := encoding.NewReader(e.Bytes())
	got := DecodeWitness(r)
	if err := r.Finish(); err != nil || got.Size() != w.Size() {
		t.Fatalf("%v, %d bytes", err, got.Size())
	}
	// A length the bytes cannot hold fails without allocating it.
	b := e.Bytes()[:1000]
	r = encoding.NewReader(b)
	DecodeWitness(r)
	if r.Err() == nil {
		t.Fatal("a truncated witness decoded")
	}
	// Check applies the series' ceiling before anything else.
	if err := Default().Check(sha(t), crossRules{max: 1000}, w, [32]byte{}, [32]byte{}); err != ErrTooLarge {
		t.Fatalf("over the ceiling: %v", err)
	}
}

// D170, Red's condition 2: a threshold signature is read at its key's
// exact size, and a wrong count or index fails before any signature byte
// is read.
func TestThresholdSignatureAtExactSize(t *testing.T) {
	s := newSigner(t, AlgEd25519, "exact")
	p := ThresholdParams{K: 1, Keys: []Key{{AlgEd25519, s.Public()}}}
	msg := Message([32]byte{1})
	good := ThresholdData{Sigs: []Sig{{0, s.Sign(msg)}}}.Encode()
	if err := Default().types[TypeThreshold].Satisfy(Default(), p.Encode(), good, msg); err != nil {
		t.Fatal(err)
	}
	long := ThresholdData{Sigs: []Sig{{0, append(s.Sign(msg), 0)}}}.Encode()
	for name, data := range map[string][]byte{
		"long":        long,
		"two sigs":    ThresholdData{Sigs: []Sig{{0, s.Sign(msg)}, {1, s.Sign(msg)}}}.Encode(),
		"bad index":   ThresholdData{Sigs: []Sig{{1, s.Sign(msg)}}}.Encode(),
		"length only": ThresholdData{Sigs: []Sig{{0, nil}}}.Encode()[:10],
	} {
		if err := Default().types[TypeThreshold].Satisfy(Default(), p.Encode(), data, msg); err == nil {
			t.Errorf("%s: satisfied", name)
		}
	}
	// A huge length prefix on a signature is refused at the algorithm's size.
	w := encoding.NewWriter(16)
	w.Count(1).U16(0).U32(1 << 30)
	if _, err := decodeThresholdDataFor(Default(), p, w.Bytes()); err == nil {
		t.Fatal("a 1 GiB signature length was taken")
	}
}
