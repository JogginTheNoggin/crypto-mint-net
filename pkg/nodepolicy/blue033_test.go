package nodepolicy_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
)

// BLUE-033 (RED-093 to RED-096): keys that fold to a setting are that
// setting, kept values are limited and written compacted, and an import
// keeps them.

// writeWith saves the default policy at a fresh path, then sets the raw
// keys given (a dotted path, one level of section at most) in the file.
func writeWith(t *testing.T, set map[string]json.RawMessage) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), nodepolicy.FileName)
	if err := nodepolicy.Save(path, nodepolicy.Default()); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for k, v := range set {
		sec, key, ok := strings.Cut(k, ".")
		if !ok {
			m[k] = v
			continue
		}
		var sm map[string]json.RawMessage
		if raw, ok := m[sec]; ok {
			json.Unmarshal(raw, &sm)
		}
		if sm == nil {
			sm = map[string]json.RawMessage{}
		}
		sm[key] = v
		m[sec], _ = json.Marshal(sm)
	}
	out, _ := json.Marshal(m)
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func load(t *testing.T, path string) nodepolicy.Policy {
	t.Helper()
	p, err := nodepolicy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// Red's file: ALLOW_REMOTE is read as allow_remote. Once the owner turns
// it off, it stays off.
func TestBlue033FoldedKeyIsTheSetting(t *testing.T) {
	path := writeWith(t, map[string]json.RawMessage{"rpc.ALLOW_REMOTE": json.RawMessage("true")})
	p := load(t, path)
	if !p.RPC.AllowRemote || len(p.UnknownSettings()) != 0 {
		t.Fatalf("allow_remote %v, unknown %v", p.RPC.AllowRemote, p.UnknownSettings())
	}
	if f := p.FoldedSettings(); !slices.Equal(f, []string{"rpc.ALLOW_REMOTE (read as allow_remote)"}) {
		t.Fatalf("folded %v", f)
	}
	p.RPC.AllowRemote = false
	if err := nodepolicy.Save(path, p); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "ALLOW_REMOTE") {
		t.Fatalf("the folded key was written back:\n%s", b)
	}
	if load(t, path).RPC.AllowRemote {
		t.Fatal("allow_remote came back")
	}
}

// Red's lookalike: liſten (U+017F) is p2p.listen, and the owner's new
// value is the one read back.
func TestBlue033UnicodeFoldedKey(t *testing.T) {
	path := writeWith(t, map[string]json.RawMessage{"p2p.li\u017ften": json.RawMessage(`"0.0.0.0:2222"`)})
	p := load(t, path)
	if p.P2P.Listen != "0.0.0.0:2222" || len(p.UnknownSettings()) != 0 {
		t.Fatalf("listen %q, unknown %v", p.P2P.Listen, p.UnknownSettings())
	}
	p.P2P.Listen = "127.0.0.1:3333"
	if err := nodepolicy.Save(path, p); err != nil {
		t.Fatal(err)
	}
	if got := load(t, path).P2P.Listen; got != "127.0.0.1:3333" {
		t.Fatalf("listen %q after save", got)
	}
}

// An unknown setting is still kept, and written on one line.
func TestBlue033KeptCompacted(t *testing.T) {
	deep := strings.Repeat("[", 10) + "1" + strings.Repeat("]", 10)
	path := writeWith(t, map[string]json.RawMessage{"future": json.RawMessage(deep), "p2p.later": json.RawMessage(`{"a": 1, "b": [1, 2]}`)})
	p := load(t, path)
	if u := p.UnknownSettings(); !slices.Equal(u, []string{"future", "p2p.later"}) {
		t.Fatalf("kept %v", u)
	}
	if err := nodepolicy.Save(path, p); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	for _, want := range []string{`"future": ` + deep, `"later": {"a":1,"b":[1,2]}`} {
		if !strings.Contains(string(b), want) {
			t.Fatalf("file lacks %s:\n%s", want, b)
		}
	}
	if u := load(t, path).UnknownSettings(); !slices.Equal(u, []string{"future", "p2p.later"}) {
		t.Fatalf("kept after save %v", u)
	}
}

// Red's 9,990-deep value is not kept, so the file does not grow; nor is
// a value over 64 KiB, nor a setting past the 256th.
func TestBlue033KeptLimits(t *testing.T) {
	set := map[string]json.RawMessage{
		"deep": json.RawMessage(strings.Repeat("[", 9990) + strings.Repeat("]", 9990)),
		"big":  json.RawMessage(`"` + strings.Repeat("x", nodepolicy.MaxKeptBytes) + `"`),
	}
	for i := range nodepolicy.MaxKeptSettings + 10 {
		set[fmt.Sprintf("k%04d", i)] = json.RawMessage("1")
	}
	path := writeWith(t, set)
	fi, _ := os.Stat(path)
	p := load(t, path)
	if n := len(p.UnknownSettings()); n != nodepolicy.MaxKeptSettings {
		t.Fatalf("%d kept", n)
	}
	d := strings.Join(p.UnknownDropped(), "\n")
	for _, want := range []string{"big (over 64 KiB)", "deep (nested over 64 deep)", "kept settings)"} {
		if !strings.Contains(d, want) {
			t.Fatalf("dropped lacks %q: %s", want, d)
		}
	}
	start := time.Now()
	if err := nodepolicy.Save(path, p); err != nil {
		t.Fatal(err)
	}
	took := time.Since(start)
	after, _ := os.Stat(path)
	if after.Size() > fi.Size() {
		t.Fatalf("node.json grew from %d to %d bytes", fi.Size(), after.Size())
	}
	if took > 5*time.Second {
		t.Fatalf("save took %s", took)
	}
	if !strings.HasSuffix(nodepolicy.NameSome(p.UnknownSettings(), 10), "and 246 more") {
		t.Fatal(nodepolicy.NameSome(p.UnknownSettings(), 10))
	}
}

// RED-096: an import that changes a setting keeps the kept settings.
func TestBlue033ImportKeepsUnknown(t *testing.T) {
	path := writeWith(t, map[string]json.RawMessage{"future": json.RawMessage("1"), "p2p.later": json.RawMessage("2")})
	local := load(t, path)
	file := nodepolicy.Default()
	file.P2P.MaxPeers = local.P2P.MaxPeers + 1
	for _, opt := range []nodepolicy.MergeOptions{{Settings: true}, {Full: true}, {Settings: true, Full: true}} {
		res, _, err := nodepolicy.Merge(local, file, opt)
		if err != nil {
			t.Fatal(err)
		}
		if res.P2P.MaxPeers != file.P2P.MaxPeers {
			t.Fatalf("%+v: the import changed nothing", opt)
		}
		if u := res.UnknownSettings(); !slices.Equal(u, []string{"future", "p2p.later"}) {
			t.Fatalf("%+v: kept %v", opt, u)
		}
	}
	res, _, _ := nodepolicy.Merge(local, file, nodepolicy.MergeOptions{Settings: true})
	if err := nodepolicy.Save(path, res); err != nil {
		t.Fatal(err)
	}
	if u := load(t, path).UnknownSettings(); !slices.Equal(u, []string{"future", "p2p.later"}) {
		t.Fatalf("after the import's save: %v", u)
	}
}
