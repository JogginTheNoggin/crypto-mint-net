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
