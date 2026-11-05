package nodepolicy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"
)

// insertOneByOne is withUnknown as D184 built it: one decode and encode
// of the whole policy per kept setting. TestWithUnknownMatchesOneByOne
// checks the one-pass version against it.
func insertOneByOne(b []byte, extras []extra) []byte {
	var ins func(obj []byte, path []string, val json.RawMessage) ([]byte, bool)
	ins = func(obj []byte, path []string, val json.RawMessage) ([]byte, bool) {
		var m map[string]json.RawMessage
		if json.Unmarshal(obj, &m) != nil || m == nil {
			return nil, false
		}
		k := path[0]
		if len(path) == 1 {
			if _, ok := m[k]; ok {
				return nil, false
			}
			m[k] = val
		} else {
			child, ok := m[k]
			if !ok || bytes.Equal(bytes.TrimSpace(child), []byte("null")) {
				child = json.RawMessage("{}")
			}
			nc, ok := ins(child, path[1:], val)
			if !ok {
				return nil, false
			}
			m[k] = nc
		}
		out, err := json.Marshal(m)
		return out, err == nil
	}
	for _, e := range extras {
		if nb, ok := ins(b, e.path, e.val); ok {
			b = nb
		}
	}
	return b
}

// TestWithUnknownMatchesOneByOne puts random kept settings back into an
// encoded policy, at random depths, some under keys this release writes,
// under lists, under null and under missing objects, and checks the bytes
// equal those of the one-setting-at-a-time version (ORANGE-042).
func TestWithUnknownMatchesOneByOne(t *testing.T) {
	p := Default()
	p.RPC.Listen = "off"
	base, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	json.Unmarshal(base, &top)
	var known []string
	for k := range top {
		known = append(known, k)
	}
	r := rand.New(rand.NewSource(42))
	key := func() string {
		switch r.Intn(4) {
		case 0:
			return known[r.Intn(len(known))] // written by this release
		case 1:
			return "listed" // made a list below
		case 2:
			return "nothing" // made null below
		}
		return fmt.Sprintf("future_%d", r.Intn(6))
	}
	for round := 0; round < 300; round++ {
		b := base
		if round%2 == 0 {
			var m map[string]json.RawMessage
			json.Unmarshal(base, &m)
			m["listed"] = json.RawMessage(`[1,2]`)
			m["nothing"] = json.RawMessage(`null`)
			b, _ = json.Marshal(m)
		}
		// collectUnknown never gives one path twice, nor a path and a
		// path inside it.
		var extras []extra
		var paths []string
		for i := r.Intn(12); i > 0; i-- {
			path := []string{key()}
			for d := r.Intn(3); d > 0; d-- {
				path = append(path, key())
			}
			id := strings.Join(path, "/") + "/"
			clash := false
			for _, o := range paths {
				clash = clash || strings.HasPrefix(o, id) || strings.HasPrefix(id, o)
			}
			if clash {
				continue
			}
			paths = append(paths, id)
			extras = append(extras, extra{path: path, val: json.RawMessage(fmt.Sprintf(`{"v":%d}`, i))})
		}
		want := insertOneByOne(b, extras)
		got := withUnknown(b, extras)
		if !bytes.Equal(got, want) {
			t.Fatalf("round %d, extras %v:\n got %s\nwant %s", round, extras, got, want)
		}
	}
}

// Saving many kept settings stays linear: 10,000 of them took 56 s
// estimated (3,000 measured at 5 s) one at a time, and now take
// milliseconds (ORANGE-042).
func TestWithUnknownManyIsLinear(t *testing.T) {
	p := Default()
	p.RPC.Listen = "off"
	base, _ := json.Marshal(p)
	var extras []extra
	for i := 0; i < 10000; i++ {
		path := []string{fmt.Sprintf("future_%05d", i)}
		if i%2 == 1 {
			path = []string{"p2p", fmt.Sprintf("future_%05d", i)}
		}
		extras = append(extras, extra{path: path, val: json.RawMessage(`{"on":true}`)})
	}
	st := time.Now()
	out := withUnknown(base, extras)
	if d := time.Since(st); d > 5*time.Second {
		t.Fatalf("10,000 kept settings took %v", d)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(out, &m); err != nil || len(m) < 5000 {
		t.Fatalf("%d top-level keys, %v", len(m), err)
	}
}
