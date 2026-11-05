package rpc

import (
	"path/filepath"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
)

// RED-094: status loads node.json again only when the file changed.
func TestBlue033StatusCachesPolicy(t *testing.T) {
	path := filepath.Join(t.TempDir(), nodepolicy.FileName)
	p := nodepolicy.Default()
	if err := nodepolicy.Save(path, p); err != nil {
		t.Fatal(err)
	}
	loads := 0
	old := loadPolicy
	loadPolicy = func(path string) (nodepolicy.Policy, error) { loads++; return old(path) }
	t.Cleanup(func() { loadPolicy = old })
	var c policyCache
	for range 5 {
		if _, ok := c.load(path); !ok {
			t.Fatal("not loaded")
		}
	}
	if loads != 1 {
		t.Fatalf("%d loads of an unchanged file", loads)
	}
	p.NoSync = !p.NoSync
	if err := nodepolicy.Save(path, p); err != nil {
		t.Fatal(err)
	}
	got, ok := c.load(path)
	if !ok || got.NoSync != p.NoSync || loads != 2 {
		t.Fatalf("a changed file was not loaded again (loads %d, no_sync %v)", loads, got.NoSync)
	}
}
