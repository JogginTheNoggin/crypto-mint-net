package series_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// RED-071, D165: a variable series' JSON leaves "hash" out, so a poo from
// before D163 finds no hash and refuses it; this poo reads it, and D163's
// first form, back to the same series. A fixed series' JSON is unchanged.
func TestVariableJSONIsUnreadableToOlderReaders(t *testing.T) {
	p := testutil.ExampleParams()
	fixed, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	type old struct { // the fields a poo from before D163 knows
		HashID hash.ID `json:"hash"`
		Mints  uint64  `json:"mints"`
	}
	if !strings.HasPrefix(string(fixed), `{"hash":`) {
		t.Fatalf("a fixed series changed form: %s", fixed)
	}
	iss := hash.Hash{1}
	p.SupplyType, p.PartSize, p.Issuer = series.Variable, 4, &iss
	v, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var o old
	if err := json.Unmarshal(v, &o); err != nil || o.HashID != 0 || o.Mints != p.Mints {
		t.Fatalf("an older reader finds hash %d in %s (%v)", o.HashID, v, err)
	}
	var back series.Params
	if err := json.Unmarshal(v, &back); err != nil || back.HashID != p.HashID || back.SupplyType != series.Variable {
		t.Fatalf("read back %+v: %v", back, err)
	}
	a, _ := series.New(p)
	b, err := series.New(back)
	if err != nil || a.ID() != b.ID() {
		t.Fatalf("another series read back: %v", err)
	}
	// D163's first form, with "hash", still reads.
	first := strings.Replace(string(v), `"variable_hash"`, `"hash"`, 1)
	var f series.Params
	if err := json.Unmarshal([]byte(first), &f); err != nil || f.HashID != p.HashID {
		t.Fatalf("the first form: %v", err)
	}
	// A key this poo does not know is an error, not ignored.
	for _, bad := range []string{
		strings.Replace(string(v), `"mints"`, `"supply_curve":7,"mints"`, 1),
		strings.Replace(string(fixed), `"mints"`, `"later_rule":true,"mints"`, 1),
	} {
		var x series.Params
		if err := json.Unmarshal([]byte(bad), &x); !errors.Is(err, series.ErrUnknownKey) {
			t.Fatalf("an unknown key: %v", err)
		}
	}
	// variable_hash on a fixed series, or two hashes that differ.
	for _, bad := range []string{
		strings.Replace(string(fixed), `"hash"`, `"variable_hash"`, 1),
		strings.Replace(string(v), `"variable_hash":1`, `"variable_hash":1,"hash":2`, 1),
	} {
		var x series.Params
		if err := json.Unmarshal([]byte(bad), &x); !errors.Is(err, series.ErrInvalid) {
			t.Fatalf("%s: %v", bad, err)
		}
	}
}
