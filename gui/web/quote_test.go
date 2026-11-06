package web

import (
	"encoding/json"
	"os/exec"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/gui/shell"
)

// TestQuoteMatchesShell checks the page's quote (api.js), which shows the
// command a view runs and types it into the terminal, quotes exactly as
// shell.Quote does, for both grammars. It needs node and is skipped
// without it.
func TestQuoteMatchesShell(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found")
	}
	args := []string{"status", "x y", "it's", "", `a\"b`, `C:\dir\`, `\\srv\"x\\`, "a&b", "a | b; $HOME `id` * ? [x] ~ # & < > ( ) ^ %", "é ü", "-root=/a b"}
	in, _ := json.Marshal(args)
	out, err := exec.Command(node, "testdata/quote.mjs", "static/api.js", string(in)).Output()
	if err != nil {
		t.Fatalf("node: %v", err)
	}
	var got [][3]string
	if err := json.Unmarshal(out, &got); err != nil || len(got) != len(args) {
		t.Fatalf("node printed %s: %v", out, err)
	}
	for _, c := range got {
		for i, g := range []shell.Grammar{shell.POSIX, shell.Windows} {
			if want := g.Quote([]string{c[0]}); "poon "+c[1+i] != want {
				t.Errorf("%v: api.js quotes %q as %s, shell.Quote as %s", g, c[0], c[1+i], want)
			}
		}
	}
}
