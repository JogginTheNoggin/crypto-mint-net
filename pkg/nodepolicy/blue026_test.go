package nodepolicy_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
)

// growing is a variable series of 8 mints in part 1, parts of 4, pools of
// 4: part 1 is pools 0 and 1, part k from 2 on is pool k. It returns the
// series info with its issuer witness, and a release of part k signed by
// signer.
type growing struct {
	s    *series.Series
	info nodepolicy.SeriesInfo
	a    issuer
}

func newGrowing(t *testing.T) growing {
	t.Helper()
	a := newIssuer("a")
	p := testutil.ExampleParams()
	p.Mints, p.PoolSize, p.Issuer = 8, 4, &a.addr
	p.SupplyType, p.PartSize = series.Variable, 4
	s := testutil.MustSeries(p)
	wit := nodepolicy.EncodeWitness(policy.SingleKeyWitness(a.signer, s.IssuerMessageTID()))
	return growing{s, nodepolicy.SeriesInfo{Params: p, IssuerWitness: wit}, a}
}

func (g growing) release(t *testing.T, k uint64, signer *policy.Ed25519Signer) series.Release {
	t.Helper()
	r, err := g.s.Unsigned(k)
	if err != nil {
		t.Fatal(err)
	}
	r.Witness = policy.SingleKeyWitness(signer, g.s.ReleaseMessage(r.ID(g.s.Hash())))
	return r
}

func (g growing) policy(rel *series.Release) nodepolicy.Policy {
	p := nodepolicy.Default()
	p.Issuers.Any = true
	si := g.info
	si.Release = rel
	p.Series = []nodepolicy.SeriesInfo{si}
	return p
}

// RED-072: follow, unfollow, export and import see the parts the kept
// release proves, as the running node does.
func TestFollowSeesReleasedParts(t *testing.T) {
	g := newGrowing(t)
	id := g.s.ID()
	rel := g.release(t, 3, g.a.signer)

	// At part 1 only, pool 3 and mint 13 are waiting, and the refusal
	// says so.
	p := g.policy(nil)
	for _, f := range []nodepolicy.Follow{{Series: &id, Pools: []uint64{3}}, {Series: &id, Mints: []uint64{13}}} {
		err := p.AddFollow(f)
		if err == nil || !strings.Contains(err.Error(), "waiting for a release") || !strings.Contains(err.Error(), "part 3") {
			t.Fatalf("at part 1: %v", err)
		}
	}
	// With part 3's release kept, they are followed.
	p = g.policy(&rel)
	if err := p.AddFollow(nodepolicy.Follow{Series: &id, Pools: []uint64{3}}); err != nil {
		t.Fatal(err)
	}
	if err := p.AddFollow(nodepolicy.Follow{Series: &id, Mints: []uint64{13}}); err != nil {
		t.Fatal(err)
	}
	if err := p.RemoveFollow(nodepolicy.Follow{Series: &id, Pools: []uint64{3}}); err != nil {
		t.Fatal(err)
	}

	// Its own export imports, into itself and into a node without it.
	p = g.policy(&rel)
	if err := p.AddFollow(nodepolicy.Follow{Series: &id, Pools: []uint64{3}}); err != nil {
		t.Fatal(err)
	}
	exp, err := nodepolicy.ExportSeries(p, id)
	if err != nil {
		t.Fatal(err)
	}
	for name, local := range map[string]nodepolicy.Policy{"itself": p, "another": nodepolicy.Default()} {
		res, plan, err := nodepolicy.Merge(local, exp, nodepolicy.MergeOptions{})
		if err != nil {
			t.Fatalf("into %s: %v", name, err)
		}
		if len(res.Series) != 1 || res.Series[0].Release == nil || res.Series[0].Release.Part != 3 {
			t.Fatalf("into %s: the release did not come with the series", name)
		}
		for _, fc := range plan.Follows {
			if fc.Pools != 4 {
				t.Fatalf("into %s: the plan counts %d pools, want 4", name, fc.Pools)
			}
		}
	}
}

// RED-072: unfollow of some pools of a variable series keeps it whole with
// exceptions, so the pools of later releases stay followed.
func TestUnfollowKeepsAVariableSeriesWhole(t *testing.T) {
	g := newGrowing(t)
	id := g.s.ID()
	rel := g.release(t, 3, g.a.signer)
	for _, take := range [][]uint64{{0}, {0, 1, 2}, {0, 1, 2, 3}} {
		p := g.policy(&rel)
		if err := p.AddFollow(nodepolicy.Follow{Series: &id}); err != nil {
			t.Fatal(err)
		}
		if err := p.RemoveFollow(nodepolicy.Follow{Series: &id, Pools: take}); err != nil {
			t.Fatal(err)
		}
		if len(p.Follow) != 1 || len(p.Follow[0].Pools) != 0 || !reflect.DeepEqual(p.Follow[0].Except, take) {
			t.Fatalf("unfollow %v: %+v", take, p.Follow)
		}
	}
}

// RED-073: a kept release that does not check is left out, the series is
// followed at part 1, a valid release replaces it, and an import does not
// bring it in.
func TestBadKeptReleaseIsReplaced(t *testing.T) {
	g := newGrowing(t)
	other := newIssuer("b")
	bad := g.release(t, 3, other.signer)
	p := g.policy(&bad)
	acc, ref := p.Accepted()
	if len(acc) != 1 || len(ref) != 0 || acc[0].ReleaseErr == nil || acc[0].Series.Released() != 1 {
		t.Fatalf("accepted %+v, refused %v", acc, ref)
	}
	good := g.release(t, 2, g.a.signer)
	res, err := p.AddRelease(good)
	if err != nil || !res.Added || p.Series[0].Release.Part != 2 {
		t.Fatalf("add-release over a bad kept one: %+v, %v", res, err)
	}

	merged, plan, err := nodepolicy.Merge(nodepolicy.Default(), g.policy(&bad), nodepolicy.MergeOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Series[0].Release != nil || len(plan.DroppedReleases) != 1 {
		t.Fatalf("import kept a bad release: %+v, %v", merged.Series[0].Release, plan.DroppedReleases)
	}
	replaced, plan, err := nodepolicy.Merge(nodepolicy.Default(), g.policy(&bad), nodepolicy.MergeOptions{Replace: true})
	if err != nil {
		t.Fatal(err)
	}
	if replaced.Series[0].Release != nil || len(plan.DroppedReleases) != 1 {
		t.Fatalf("replace kept a bad release: %+v", replaced.Series[0].Release)
	}
}

// RED-074: a forged release of a part already known is refused, not
// answered as fine; a genuine one is not added.
func TestAddReleaseChecksAKnownPart(t *testing.T) {
	g := newGrowing(t)
	kept := g.release(t, 3, g.a.signer)
	p := g.policy(&kept)
	forged := g.release(t, 2, newIssuer("b").signer)
	if _, err := p.AddRelease(forged); !errors.Is(err, series.ErrRelease) {
		t.Fatalf("a forged release of a known part: %v", err)
	}
	forged = kept
	forged.Previous[0] ^= 1
	if _, err := p.AddRelease(forged); !errors.Is(err, series.ErrRelease) {
		t.Fatalf("a changed release of the kept part: %v", err)
	}
	res, err := p.AddRelease(g.release(t, 2, g.a.signer))
	if err != nil || res.Added || res.Released != 3 {
		t.Fatalf("a genuine known release: %+v, %v", res, err)
	}
}

// RED-075: Save keeps the node's own record of the releases it checked,
// and Load uses an entry only when it matches the release kept.
func TestCheckedReleasesCache(t *testing.T) {
	g := newGrowing(t)
	dir := t.TempDir()
	path := filepath.Join(dir, nodepolicy.FileName)
	p := g.policy(nil)
	rel := g.release(t, 3, g.a.signer)
	if _, err := p.AddRelease(rel); err != nil {
		t.Fatal(err)
	}
	if err := nodepolicy.Save(path, p); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, nodepolicy.CheckedFileName))
	if err != nil {
		t.Fatal(err)
	}
	var list []struct {
		Series   hash.Hash `json:"series"`
		Part     uint64    `json:"part"`
		Previous hash.Hash `json:"previous"`
	}
	if err := json.Unmarshal(b, &list); err != nil || len(list) != 1 || list[0].Part != 3 || list[0].Previous != rel.Previous {
		t.Fatalf("cache %s: %v", b, err)
	}
	// A damaged cache costs nothing but time.
	if err := os.WriteFile(filepath.Join(dir, nodepolicy.CheckedFileName), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	q, err := nodepolicy.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	acc, _ := q.Accepted()
	if len(acc) != 1 || acc[0].Series.Released() != 3 || acc[0].ReleaseErr != nil {
		t.Fatalf("after a damaged cache: %+v", acc)
	}
}
