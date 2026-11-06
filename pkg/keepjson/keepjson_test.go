package keepjson

import (
	"encoding/json"
	"strings"
	"testing"
)

type sample struct {
	Format int    `json:"format"`
	Name   string `json:"name,omitempty"`
	Skip   int    `json:"-"`
}

func TestSplitJoinKeepsUnknown(t *testing.T) {
	raw := []byte(`{"format":1,"NAME":"x","later":{"a":[1, 2]},"b":"two"}`)
	k, err := Split(raw, sample{})
	if err != nil || len(k) != 2 || string(k["later"]) != `{"a":[1,2]}` || string(k["b"]) != `"two"` {
		t.Fatalf("split %v %v", k, err)
	}
	b, _ := json.Marshal(sample{Format: 1})
	out, err := Join(b, k)
	if err != nil || string(out) != `{"format":1,"b":"two","later":{"a":[1,2]}}` {
		t.Fatalf("join %s %v", out, err)
	}
	empty, err := Join([]byte(`{}`), Kept{"z": json.RawMessage(`0`)})
	if err != nil || string(empty) != `{"z":0}` {
		t.Fatalf("join into empty %s %v", empty, err)
	}
	// A key the struct now has is not written twice.
	twice, _ := Join([]byte(`{"b":1}`), k)
	if strings.Count(string(twice), `"b"`) != 1 {
		t.Fatalf("written twice: %s", twice)
	}
}

func TestSplitLimits(t *testing.T) {
	var many strings.Builder
	many.WriteString(`{"format":1`)
	for i := 0; i <= MaxKeys; i++ {
		many.WriteString(`,"k` + strings.Repeat("x", i) + `":0`)
	}
	many.WriteString(`}`)
	if _, err := Split([]byte(many.String()), sample{}); err == nil {
		t.Fatal("over MaxKeys kept")
	}
	big := `{"v":"` + strings.Repeat("a", MaxBytes) + `"}`
	if _, err := Split([]byte(big), sample{}); err == nil {
		t.Fatal("over MaxBytes kept")
	}
	deep := `{"v":` + strings.Repeat("[", MaxDepth+1) + strings.Repeat("]", MaxDepth+1) + `}`
	if _, err := Split([]byte(deep), sample{}); err == nil {
		t.Fatal("over MaxDepth kept")
	}
}
