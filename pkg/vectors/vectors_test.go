package vectors

import (
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/vectors.json")

func TestGolden(t *testing.T) {
	got, err := json.MarshalIndent(Compute(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "vectors.json")
	if *update {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./pkg/vectors -update to create it)", err)
	}
	if string(got) != string(want) {
		t.Fatalf("test vectors changed. An encoding or hash changed, which changes every id.\n"+
			"If this is intended, record it in docs/decisions.md and run go test ./pkg/vectors -update.\n got:\n%s\nwant:\n%s", got, want)
	}
}
