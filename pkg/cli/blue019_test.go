package cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

// RED-047, D152: an -if-digest given empty or malformed is refused, not
// taken as left out, and -if-digest without -apply is refused.
func TestIfDigestEmptyIsRefused(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	cli(t, nil, "init", "-example", "-root", src, "-p2p", "off")
	cli(t, nil, "init", "-root", dst, "-p2p", "off")
	bundle := filepath.Join(t.TempDir(), "b.json")
	runOut(t, "policy", "export", "-root", src, "-out", bundle)
	for _, args := range [][]string{
		{"-apply", "-if-digest", ""},
		{"-apply", "-if-digest", "00"},
		{"-apply", "-if-digest=" + strings.Repeat("g", 64)},
		{"-if-digest", strings.Repeat("0", 64)},
	} {
		var out bytes.Buffer
		err := run(append([]string{"policy", "import", bundle, "-root", dst}, args...), &out)
		if err == nil {
			t.Errorf("%q was taken:\n%s", args, out.String())
		}
	}
	if out := runOut(t, "policy", "import", bundle, "-root", dst); !strings.Contains(out, "+ series") {
		t.Fatalf("still applied something:\n%s", out)
	}
}

// RED-051, D154: the digest binds the options and the plan too: the
// digest of a follow-only preview does not apply with -settings or -full,
// nor after node.json changed.
func TestIfDigestBindsOptionsAndPlan(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	cli(t, nil, "init", "-example", "-root", src, "-p2p", "off", "-peer", "a.example:1")
	cli(t, nil, "init", "-root", dst, "-p2p", "off")
	bundle := filepath.Join(t.TempDir(), "b.json")
	runOut(t, "policy", "export", "-root", src, "-out", bundle)
	var res []importResult
	cli(t, &res, "policy", "import", bundle, "-json", "-root", dst)
	d := res[0].Digest
	for _, extra := range [][]string{{"-settings"}, {"-full"}, {"-replace"}, {"-prefer-file"}} {
		err := run(append([]string{"policy", "import", bundle, "-apply", "-if-digest", d, "-root", dst}, extra...), &bytes.Buffer{})
		if err == nil || !strings.Contains(err.Error(), "not the import previewed") {
			t.Errorf("%v applied with the follow-only digest: %v", extra, err)
		}
	}
	runOut(t, "policy", "import", bundle, "-apply", "-if-digest", d, "-root", dst)
	// Applied: the same digest now plans nothing, so it no longer matches.
	if err := run([]string{"policy", "import", bundle, "-apply", "-if-digest", d, "-root", dst}, &bytes.Buffer{}); err == nil {
		t.Error("the digest applied again after node.json changed")
	}
}
