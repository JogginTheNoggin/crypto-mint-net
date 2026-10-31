package cli

import (
	"bytes"
	"flag"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

func render(t *testing.T, v any, args ...string) string {
	t.Helper()
	fs := flag.NewFlagSet("t", flag.ContinueOnError)
	o := addOutput(fs)
	if err := fs.Parse(args); err != nil {
		t.Fatal(err)
	}
	if err := o.check(); err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	if err := o.write(&b, v); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

type tRow struct {
	ID    hash.Hash `json:"id"`
	N     int       `json:"n"`
	In    struct{ A, B int }
	Tags  []string `json:"tags"`
	Items []tRow   `json:"items,omitempty"`
	Opt   *int     `json:"opt,omitempty"`
}

type tReply struct {
	Name  string `json:"name"`
	Stats struct {
		X int `json:"x"`
	} `json:"stats"`
	Empty map[string]int `json:"empty"`
	Rows  []tRow         `json:"rows"`
}

func TestTableOutput(t *testing.T) {
	var id hash.Hash
	for i := range id {
		id[i] = byte(i * 7)
	}
	one := 1
	r := tReply{Name: "pool", Empty: map[string]int{}, Rows: []tRow{
		{ID: id, N: 1, Tags: []string{"a", "b"}, Items: []tRow{{}}},
		{ID: id, N: 2, Opt: &one},
	}}
	r.Stats.X = 7
	got := render(t, r, "-o", "table")
	short := id.String()[:8] + ".." + id.String()[60:]
	want := "name     pool\n" +
		"stats.x  7\n" +
		"empty    -\n" +
		"\nrows:\n" +
		"ID              N  IN.A  IN.B  TAGS  ITEMS  OPT\n" +
		short + "  1  0     0     a,b   [1]    -\n" +
		short + "  2  0     0     -     -      1\n"
	if got != want {
		t.Fatalf("got\n%s\nwant\n%s", got, want)
	}
	if wide := render(t, r, "-o", "table", "-wide"); !strings.Contains(wide, id.String()) {
		t.Fatalf("-wide shortened a hash:\n%s", wide)
	}
	if js := render(t, r); !strings.HasPrefix(js, "{\n") || !strings.Contains(js, id.String()) {
		t.Fatalf("default is not JSON:\n%s", js)
	}
	if got := render(t, []tRow{}, "-o", "table"); got != "(none)\n" {
		t.Fatalf("empty list: %q", got)
	}
}

func TestTableOutputBadFormat(t *testing.T) {
	err := run([]string{"status", "-o", "yaml", "-dir", t.TempDir()}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "-o must be json or table") {
		t.Fatalf("got %v", err)
	}
}

// TestEveryReadCommandTakesO checks the read commands added in GREEN-012
// take -o and refuse an unknown form before doing anything.
func TestEveryReadCommandTakesO(t *testing.T) {
	t.Setenv("POO_HOME", t.TempDir())
	for _, p := range [][]string{
		{"status"}, {"pools"}, {"ledger"}, {"pending"}, {"unspent"}, {"tx"}, {"address"},
		{"holdings"}, {"peers"}, {"tracking"}, {"chases"}, {"payment"},
		{"wallet", "addresses"}, {"wallet", "balance"}, {"wallet", "status"},
	} {
		if commandFlags(p)["o"] == nil {
			t.Errorf("poo %s has no -o", strings.Join(p, " "))
			continue
		}
		err := run(append(p, "-o", "yaml"), &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "-o must be") {
			t.Errorf("poo %s -o yaml: %v", strings.Join(p, " "), err)
		}
	}
}

// A job row like poo grind jobs prints: a plan of six scalar fields and a
// list of pools spread into columns, and a rate kept to two decimals
// (GREEN-018).
func TestTableNestedAndRates(t *testing.T) {
	type plan struct {
		Hash    string `json:"hash"`
		Device  string `json:"device"`
		Kind    string `json:"kind"`
		Threads int    `json:"threads"`
		Batch   int    `json:"batch"`
		Auto    bool   `json:"auto"`
	}
	type job struct {
		ID    int     `json:"id"`
		Plan  plan    `json:"plan"`
		Pools []int   `json:"pools"`
		Rate  float64 `json:"rate"`
		Tries uint64  `json:"tries"`
	}
	rows := []job{{ID: 1, Plan: plan{"sha256d", "cpu", "cpu", 4, 1024, true}, Pools: []int{1, 2}, Rate: 1234567.891234, Tries: 99}}
	got := render(t, rows, "-o", "table")
	for _, w := range []string{"PLAN.DEVICE", "PLAN.THREADS", "1,2", "1234567.89", "99"} {
		if !strings.Contains(got, w) {
			t.Errorf("table lacks %q:\n%s", w, got)
		}
	}
	if strings.Contains(got, "{6}") || strings.Contains(got, "891234") {
		t.Errorf("table still counts the plan or prints every digit:\n%s", got)
	}
	if wide := render(t, rows, "-o", "table", "-wide"); !strings.Contains(wide, "1234567.891234") {
		t.Errorf("-wide rounds the rate:\n%s", wide)
	}
}

// A tx:TID target is shortened like a hash, and a time to the nanosecond
// is printed to the second, unless -wide (GREEN-020).
func TestTablePrefixedHashAndTime(t *testing.T) {
	tid := strings.Repeat("ab", 32)
	row := []map[string]string{{"target": "tx:" + tid, "started": "2026-10-30T10:24:40.112586194Z"}}
	got := render(t, row, "-o", "table")
	for _, w := range []string{"tx:abababab..abab", "2026-10-30T10:24:40Z"} {
		if !strings.Contains(got, w) {
			t.Errorf("table lacks %q:\n%s", w, got)
		}
	}
	if wide := render(t, row, "-o", "table", "-wide"); !strings.Contains(wide, tid) || !strings.Contains(wide, ".112586194Z") {
		t.Errorf("-wide shortens:\n%s", wide)
	}
}
