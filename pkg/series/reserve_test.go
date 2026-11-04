package series_test

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

var updateReserve = flag.Bool("update-reserve", false, "rewrite testdata/reserve.json (never for a released series)")

// reserveVector is the pinned test vector of the default reserve series
// (D170): its params as JSON, their canonical byte form and its id. Black
// checks the id against the document; this test checks the code against
// all three, so a change to the params, to their byte form or to the id
// rule shows up here.
type reserveVector struct {
	Note    string        `json:"note"`
	Params  series.Params `json:"params"`
	Encoded string        `json:"encoded_params"`
	ID      string        `json:"series_id"`
	Pools   uint64        `json:"pools"`
}

func TestReserveSeriesPinned(t *testing.T) {
	s, err := series.New(series.Reserve())
	if err != nil {
		t.Fatal(err)
	}
	got := reserveVector{
		Note:    "The default reserve series (D170). Never edit: every value is inside the series id.",
		Params:  s.Params(),
		Encoded: hex.EncodeToString(s.Params().Encode()),
		ID:      s.ID().String(),
		Pools:   s.PoolCount(),
	}
	b, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, '\n')
	if *updateReserve {
		if err := os.WriteFile("testdata/reserve.json", b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile("testdata/reserve.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, want) {
		t.Fatalf("the reserve series changed:\n%s\nwant:\n%s", b, want)
	}
	if got.ID != series.ReserveID || !series.IsReserve(s.ID()) {
		t.Fatalf("ReserveID %s, built %s", series.ReserveID, got.ID)
	}
	var v reserveVector
	if err := json.Unmarshal(want, &v); err != nil {
		t.Fatal(err)
	}
	s2, err := series.New(v.Params)
	if err != nil || s2.ID() != s.ID() {
		t.Fatalf("the vector's params do not build the id: %v", err)
	}
}

// The reserve series is what the document says.
func TestReserveSeriesValues(t *testing.T) {
	p := series.Reserve()
	if p.Mints != 32768 || p.PoolSize != 32 || p.Supply != 1e14 || p.MinEntryStrength != 20 ||
		p.MinLayerStrength != 24 || p.EntryHashCredit || p.Issuer != nil || p.MaxWitness != 131072 ||
		p.SupplyType != series.Fixed {
		t.Fatalf("%+v", p)
	}
	if len(p.KeyAlgs) != 5 || p.KeyAlgs[4] != policy.AlgSLHDSASHA2256s {
		t.Fatalf("keys %v", p.KeyAlgs)
	}
	// Red's table: the largest k under the ceiling.
	want := map[policy.AlgID]int{policy.AlgEd25519: 16, policy.AlgMLDSA65: 16, policy.AlgMLDSA87: 16, policy.AlgSLHDSASHA2128s: 16, policy.AlgSLHDSASHA2256s: 4}
	for id, k := range want {
		a, _ := policy.Default().Alg(id)
		if got := policy.MaxK(a, int(p.MaxWitness)); got != k {
			t.Errorf("%s: largest k %d, want %d", a.Name(), got, k)
		}
	}
	// Spendability agrees: 16-of-16 ML-DSA-87 and 4-of-4 SLH-DSA-256s can be
	// made, 5-of-5 SLH-DSA-256s cannot.
	s, _ := series.New(p)
	keys := func(alg policy.AlgID, n int) []policy.Key {
		a, _ := policy.Default().Alg(alg)
		out := make([]policy.Key, n)
		for i := range out {
			pub := make([]byte, a.PublicKeySize())
			pub[0] = byte(i)
			out[i] = policy.Key{Alg: alg, Public: pub}
		}
		return out
	}
	for _, c := range []struct {
		alg  policy.AlgID
		n    int
		okay bool
	}{{policy.AlgMLDSA87, 16, true}, {policy.AlgSLHDSASHA2128s, 16, true}, {policy.AlgSLHDSASHA2256s, 4, true}, {policy.AlgSLHDSASHA2256s, 5, false}} {
		err := s.Registry().CheckPolicy(s, policy.ThresholdParams{K: uint16(c.n), Keys: keys(c.alg, c.n)}.Policy())
		if (err == nil) != c.okay {
			t.Errorf("%d of %d alg %d: %v", c.n, c.n, c.alg, err)
		}
	}
}
