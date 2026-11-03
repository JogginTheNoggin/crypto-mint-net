package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
)

// RED-058, D158: poo peer remove -imported takes out every
// address known only from an imported file, and the import's preview
// names it and says the addresses will be dialled.
func TestPeersForgetImported(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	cli(t, nil, "init", "-root", src, "-p2p", "off")
	cli(t, nil, "init", "-root", dst, "-p2p", "off")
	seedBook(t, filepath.Join(src, "policies", "default"))
	file := filepath.Join(t.TempDir(), "peers.json")
	runOut(t, "peers", "export", "-root", src, "-out", file)
	if out := runOut(t, "peers", "import", file, "-root", dst); !strings.Contains(out, "note: once applied, the node tries these addresses when it is short of peers") || !strings.Contains(out, "poo peer remove -imported") {
		t.Fatalf("preview:\n%s", out)
	}
	runOut(t, "peers", "import", file, "-apply", "-root", dst)
	if out := runOut(t, "peer", "remove", "-imported", "-root", dst); !strings.Contains(out, `"imported": 2`) || !strings.Contains(out, `"forgotten": 2`) {
		t.Fatalf("forget: %s", out)
	}
	b, err := peers.Open(filepath.Join(dst, "policies", "default", peers.FileName), peers.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(b.Records()); n != 0 {
		t.Fatalf("%d records left", n)
	}
}

// RED-061, D158: a table cell from a file is escaped.
func TestTableEscapesText(t *testing.T) {
	if got := (tabler{}).cell(&jnode{text: "fr\x1b[2Kiend"}); got != `fr\x1b[2Kiend` {
		t.Fatalf("cell %q", got)
	}
}
