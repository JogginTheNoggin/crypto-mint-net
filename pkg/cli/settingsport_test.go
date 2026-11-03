package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
)

// D155: a node's settings go to another machine without what it follows
// or its machine-local settings, the plan comes first and names what may
// not fit, and policy import and settings import each refuse the other's
// file.
func TestSettingsExportImport(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	cli(t, nil, "init", "-example", "-root", src, "-listen", "127.0.0.1:7111", "-p2p", "off", "-peer", "a.example:1", "-registry", "/old/home/reg.txt")
	cli(t, nil, "init", "-root", dst, "-p2p", "off")
	file := filepath.Join(t.TempDir(), "settings.json")
	said := runOut(t, "settings", "export", "-root", src, "-out", file)
	b, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), "7111") || !strings.Contains(string(b), `"scope": "settings"`) || strings.Contains(string(b), `"params"`) {
		t.Fatalf("export kept the RPC address or a series, or is not a settings bundle:\n%s", b)
	}
	// The registry file names the owner's folders: only -full keeps it,
	// and the export says so and what the file holds (RED-052, D156).
	if strings.Contains(string(b), "reg.txt") || !strings.Contains(said, "left out 1 registry") || !strings.Contains(said, "the file holds 1 peer address (p2p.peers)") {
		t.Fatalf("a plain export kept the registry file, or said:\n%s\n%s", said, b)
	}
	// The rest of this test carries the registry file: a full export.
	runOut(t, "settings", "export", "-root", src, "-full", "-out", file)

	dstPath := filepath.Join(dst, "policies", "default", nodepolicy.FileName)
	before, _ := os.ReadFile(dstPath)
	out := runOut(t, "settings", "import", file, "-root", dst)
	if !strings.Contains(out, "+ p2p.peers: a.example:1") || !strings.Contains(out, "read first") || !strings.Contains(out, "dry run") {
		t.Fatalf("plan:\n%s", out)
	}
	if !strings.Contains(out, "skipped, node settings (-full takes them): p2p.registries") {
		t.Fatalf("the registry file was not skipped with the right hint:\n%s", out)
	}
	if after, _ := os.ReadFile(dstPath); !bytes.Equal(before, after) {
		t.Fatal("a dry run changed node.json")
	}

	// With -full the file is taken, and the preview says it is not here,
	// until -map-path points it at this machine's.
	out = runOut(t, "settings", "import", file, "-full", "-root", dst)
	if !strings.Contains(out, "may not fit this machine: p2p.registries: /old/home/reg.txt: no such file here") {
		t.Fatalf("-full did not say the file is missing:\n%s", out)
	}
	here := t.TempDir()
	if err := os.WriteFile(filepath.Join(here, "reg.txt"), []byte("b.example:1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out = runOut(t, "settings", "import", file, "-full", "-map-path", "/old/home="+here, "-root", dst)
	if strings.Contains(out, "may not fit") || !strings.Contains(out, "+ p2p.registries: "+filepath.Join(here, "reg.txt")) {
		t.Fatalf("-map-path:\n%s", out)
	}
	dg := regexp.MustCompile(`import digest ([0-9a-f]{64})`).FindStringSubmatch(out)
	if dg == nil {
		t.Fatalf("no digest:\n%s", out)
	}
	// The digest covers the path map too.
	var o bytes.Buffer
	if err := run([]string{"settings", "import", file, "-full", "-apply", "-if-digest", dg[1], "-root", dst}, &o); err == nil || !strings.Contains(err.Error(), "not the one previewed") {
		t.Fatalf("a digest applied without its -map-path: %v\n%s", err, o.String())
	}
	runOut(t, "settings", "import", file, "-full", "-map-path", "/old/home="+here, "-apply", "-if-digest", dg[1], "-root", dst)
	p, err := nodepolicy.Load(dstPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Series) != 0 || len(p.Follow) != 0 || len(p.P2P.Peers) != 1 || len(p.P2P.Registries) != 1 || p.P2P.Listen != "off" {
		t.Fatalf("imported: %+v", p)
	}
	if out := runOut(t, "settings", "import", file, "-full", "-map-path", "/old/home="+here, "-root", dst); !strings.Contains(out, "nothing to change") {
		t.Fatalf("second import:\n%s", out)
	}

	// A whole node.json gives up its settings only.
	out = runOut(t, "settings", "import", filepath.Join(src, "policies", "default", nodepolicy.FileName), "-root", dst)
	if strings.Contains(out, "series") {
		t.Fatalf("a settings import of a node.json took what it follows:\n%s", out)
	}

	// Each import refuses the other's file.
	if err := run([]string{"policy", "import", file, "-root", dst}, &o); err == nil || !strings.Contains(err.Error(), "poo settings import") {
		t.Fatalf("policy import of a settings file: %v", err)
	}
	all := filepath.Join(t.TempDir(), "all.json")
	runOut(t, "policy", "export", "-all", "-root", src, "-out", all)
	if err := run([]string{"settings", "import", all, "-root", dst}, &o); err == nil || !strings.Contains(err.Error(), "several node policies") {
		t.Fatalf("settings import of a node bundle: %v", err)
	}
}

// D155: a running node's import names what may not fit its machine too.
func TestSettingsImportIntoRunningNode(t *testing.T) {
	src, root := t.TempDir(), t.TempDir()
	cli(t, nil, "init", "-root", src, "-p2p", "off", "-peer", "a.example:1")
	cli(t, nil, "init", "-root", root, "-listen", "127.0.0.1:0", "-p2p", "off")
	file := filepath.Join(t.TempDir(), "settings.json")
	runOut(t, "settings", "export", "-root", src, "-out", file)
	stop := startNode(t, filepath.Join(root, "policies", "default"))
	defer stop()
	out := runOut(t, "settings", "import", file, "-apply", "-root", root)
	if !strings.Contains(out, "running") || !strings.Contains(out, "applied: the node reloaded") {
		t.Fatalf("apply:\n%s", out)
	}
	p, err := nodepolicy.Load(filepath.Join(root, "policies", "default", nodepolicy.FileName))
	if err != nil || len(p.P2P.Peers) != 1 {
		t.Fatalf("running import: %v %+v", err, p.P2P)
	}
}

// A -map-path that cannot change anything says so: without -full no
// registry file is taken, and a map matching no registry file of the
// file is most likely mistyped (GREEN-045).
func TestMapPathNotes(t *testing.T) {
	src, dst := t.TempDir(), t.TempDir()
	cli(t, nil, "init", "-example", "-root", src, "-p2p", "off", "-registry", "/old/home/reg.txt")
	cli(t, nil, "init", "-root", dst, "-p2p", "off")
	file := filepath.Join(t.TempDir(), "settings.json")
	// A plain export leaves local registry files out (RED-052), so the
	// file that carries one is a full export.
	runOut(t, "settings", "export", "-full", "-root", src, "-out", file)

	out := runOut(t, "settings", "import", file, "-map-path", "/old/home=/new", "-root", dst)
	if !strings.Contains(out, "note: -map-path rewrites registry files, which only -full takes") || strings.Contains(out, "matches no registry file") {
		t.Fatalf("without -full:\n%s", out)
	}
	out = runOut(t, "settings", "import", file, "-full", "-map-path", "/old/hom=/new", "-root", dst)
	if !strings.Contains(out, "note: -map-path /old/hom=/new matches no registry file in the file") || strings.Contains(out, "only -full takes") {
		t.Fatalf("a mistyped map:\n%s", out)
	}
	if out = runOut(t, "settings", "import", file, "-full", "-map-path", "/old/home=/new", "-root", dst); strings.Contains(out, "note: -map-path") {
		t.Fatalf("a map that is used:\n%s", out)
	}
	if (nodepolicy.PathMap{From: "https://a", To: "/x"}).Maps("https://a/reg") {
		t.Error("a map rewrote a registry URL")
	}
}
