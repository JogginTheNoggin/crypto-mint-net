package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDemoRuns(t *testing.T) {
	var out bytes.Buffer
	if err := run([]string{"demo", "-quiet"}, &out); err != nil {
		t.Fatalf("demo failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "selected again: true") {
		t.Fatalf("unexpected summary: %s", out.String())
	}
}

func TestKeygenDeterministic(t *testing.T) {
	seed := strings.Repeat("ab", 32)
	var a, b bytes.Buffer
	if err := run([]string{"keygen", "-seed", seed}, &a); err != nil {
		t.Fatal(err)
	}
	run([]string{"keygen", "-seed", seed}, &b)
	if a.String() != b.String() {
		t.Fatal("same seed gave different keys")
	}
	var m map[string]string
	if err := json.Unmarshal(a.Bytes(), &m); err != nil || len(m["address_sha256"]) != 64 {
		t.Fatalf("bad output %s", a.String())
	}
	if err := run([]string{"keygen", "-seed", "00"}, &a); err == nil {
		t.Fatal("short seed must fail")
	}
}

func TestSeriesInfo(t *testing.T) {
	var ex bytes.Buffer
	if err := run([]string{"series", "example"}, &ex); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "series.json")
	if err := os.WriteFile(path, ex.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	var info bytes.Buffer
	if err := run([]string{"series", "info", path}, &info); err != nil {
		t.Fatal(err)
	}
	var got struct {
		ID    string `json:"series_id"`
		Pools []any  `json:"pools"`
	}
	if err := json.Unmarshal(info.Bytes(), &got); err != nil || len(got.Pools) != 2 || len(got.ID) != 64 {
		t.Fatalf("info %s", info.String())
	}
}

func TestUnknownCommand(t *testing.T) {
	if err := run([]string{"nope"}, &bytes.Buffer{}); err == nil {
		t.Fatal("want error")
	}
	var out bytes.Buffer
	run(nil, &out)
	if !strings.Contains(out.String(), "Usage") {
		t.Fatal("usage missing")
	}
}
