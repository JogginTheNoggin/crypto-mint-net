package nodepolicy_test

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
)

// p2p.announce_version takes release (the default) or none, hides the
// release in hellos when none, and stays on its machine (D186).
func TestAnnounceVersion(t *testing.T) {
	p := nodepolicy.Default()
	if p2p.FromPolicy(p).HideRelease {
		t.Fatal("hidden by default")
	}
	for _, v := range []string{"", "release", "none"} {
		p.P2P.AnnounceVersion = v
		if err := p.Validate(); err != nil {
			t.Fatalf("%q: %v", v, err)
		}
	}
	if !p2p.FromPolicy(p).HideRelease {
		t.Fatal("none does not hide it")
	}
	p.P2P.AnnounceVersion = "name"
	if p.Validate() == nil {
		t.Fatal("took name")
	}
	p.P2P.AnnounceVersion = "none"
	if s := nodepolicy.ExportPolicy(p, false); s.P2P.AnnounceVersion != "" {
		t.Fatal("export kept a machine-local setting")
	}
}
