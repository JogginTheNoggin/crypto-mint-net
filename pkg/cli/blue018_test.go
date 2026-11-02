package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// RED-045, D150: a node bundle whose second node policy cannot be
// imported changes none of them, and says so.
func TestNodeBundleImportsAllOrNothing(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	cli(t, nil, "init", "-example", "-root", src, "-policy", "a", "-p2p", "off")
	cli(t, nil, "init", "-example", "-root", src, "-policy", "b", "-p2p", "off")
	bundle := filepath.Join(t.TempDir(), "all.json")
	runOut(t, "policy", "export", "-all", "-root", src, "-out", bundle)

	cli(t, nil, "init", "-root", dst, "-policy", "b", "-p2p", "off")
	damaged := filepath.Join(dst, "policies", "b", nodepolicy.FileName)
	if err := os.WriteFile(damaged, []byte("bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err := run([]string{"policy", "import", bundle, "-apply", "-root", dst}, &out)
	if err == nil || !strings.Contains(err.Error(), `node policy "b"`) || !strings.Contains(err.Error(), "nothing was changed") {
		t.Fatalf("import: %v\n%s", err, out.String())
	}
	if _, err := os.Stat(filepath.Join(dst, "policies", "a", nodepolicy.FileName)); !os.IsNotExist(err) {
		t.Fatalf("policy a was written before b failed: %v", err)
	}
}

// RED-043, D150: a policy import names its -settings flag in the usage.
func TestPolicyImportUsageNamesSettings(t *testing.T) {
	err := run([]string{"policy", "import"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "-settings") {
		t.Fatalf("usage: %v", err)
	}
}

// RED-045, D150: a running node takes any bundle poo policy import
// reads, including one over the 8 MiB the other endpoints take.
func TestRunningNodeTakesALargeImport(t *testing.T) {
	root := t.TempDir()
	cli(t, nil, "init", "-root", root, "-listen", "127.0.0.1:0", "-p2p", "off")
	p := nodepolicy.Default()
	for i := 0; i < 300000; i++ {
		p.P2P.Peers = append(p.P2P.Peers, fmt.Sprintf("198.51.%d.%d:9000", i/250%250, i%250+i/62500))
	}
	b := nodepolicy.NewBundle(nodepolicy.ScopePolicy, false, time.Now())
	b.Policies = []nodepolicy.BundlePolicy{{Policy: p}}
	var buf bytes.Buffer
	if err := b.Write(&buf); err != nil {
		t.Fatal(err)
	}
	if buf.Len() <= rpc.MaxBody || buf.Len() > nodepolicy.MaxBundleSize {
		t.Fatalf("bundle of %d bytes is not between the two caps", buf.Len())
	}
	file := filepath.Join(t.TempDir(), "big.json")
	if err := os.WriteFile(file, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	stop := startNode(t, filepath.Join(root, "policies", "default"))
	defer stop()
	var res []importResult
	cli(t, &res, "policy", "import", file, "-json", "-root", root)
	if len(res) != 1 || !res[0].Running || len(res[0].Plan.SkippedSettings) == 0 {
		t.Fatalf("import: %+v", res)
	}
}
