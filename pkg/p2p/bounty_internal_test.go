package p2p

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/bounty"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// D196: the hello lists "bounty" only when the policy takes part, and the
// list stays sorted, as a hello's features must be.
func TestBountyFeatureOnlyWhenOn(t *testing.T) {
	off := &Host{}
	if wire.Has(off.features(), wire.FeatureBounty, 1) {
		t.Fatal("the feature is listed with the market off")
	}
	b, err := bounty.Open(bounty.Config{Take: true}, "")
	if err != nil {
		t.Fatal(err)
	}
	on := &Host{cfg: Config{Bounty: b}}
	fs := on.features()
	if !wire.Has(fs, wire.FeatureBounty, 1) || len(fs) != len(wire.OwnFeatures)+1 {
		t.Fatalf("features %+v", fs)
	}
	if err := (wire.Hello{Protocol: 1, Features: fs}).Check(); err != nil {
		t.Fatalf("the hello with the feature: %v", err)
	}
	if len(wire.OwnFeatures) != 4 {
		t.Fatal("features() changed the build's list")
	}
}
