package vectors

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
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

// v1Digest is the SHA-256 of testdata/vectors.json for byte form v1, which
// is frozen (decision D76). -update cannot change it: a new byte form is a
// new format version beside this one, never an edit of it.
const v1Digest = "307c8f212aa4a098166320d148e06462c83c50855d328f4a8c69ec2535710b5b"

func TestByteFormV1Frozen(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(b)); got != v1Digest {
		t.Fatalf("testdata/vectors.json has digest %s, want %s: byte form v1 is frozen (docs/protocol/encoding.md, Stability)", got, v1Digest)
	}
}

// The variable supply vectors (format version 2, D163) live in a file of
// their own, so the frozen version 1 file never changes.
func TestGoldenVariable(t *testing.T) {
	got, err := json.MarshalIndent(ComputeVariable(), "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	path := filepath.Join("testdata", "variable.json")
	if *update {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run go test ./pkg/vectors -update to create it)", err)
	}
	if string(got) != string(want) {
		t.Fatalf("variable supply vectors changed; record it in docs/decisions.md and run go test ./pkg/vectors -update.\n got:\n%s\nwant:\n%s", got, want)
	}
}
