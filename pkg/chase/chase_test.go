package chase_test

import (
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/chase"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
)

var t0 = time.Unix(1_800_000_000, 0)

func sender(id, rng string, c peers.Category) chase.Sender {
	return chase.Sender{ID: id, Range: rng, Cat: c}
}

func h(b byte) *hash.Hash { x := hash.Hash{b}; return &x }

func TestGroupingAndOneOpenPerSender(t *testing.T) {
	q := chase.New(chase.Config{})
	a, _ := q.Offer("p", sender("a", "r1", peers.CatBackup), h(1), t0)
	b, _ := q.Offer("p", sender("b", "r2", peers.CatBackup), h(1), t0)
	if a != b {
		t.Fatal("the same entry hash made two groups")
	}
	// Two independent listed senders corroborate.
	if g := q.Groups("p", t0); len(g) != 1 || g[0].Evidence != chase.Corroborated {
		t.Fatalf("groups %+v", g)
	}
	// A newer stated weight from a replaces its older one.
	c, _ := q.Offer("p", sender("a", "r1", peers.CatBackup), h(2), t0)
	gs := q.Groups("p", t0)
	if c == a || len(gs) != 2 {
		t.Fatalf("groups after a moved: %d", len(gs))
	}
	for _, g := range gs {
		if g.ID == a && (len(g.Senders) != 1 || g.Evidence != chase.WordOnly) {
			t.Fatalf("old group kept a: %+v", g)
		}
	}
	// Stated weights with no hash are grouped by sender.
	x, _ := q.Offer("p", sender("x", "r3", peers.CatCandidate), nil, t0)
	y, _ := q.Offer("p", sender("x", "r3", peers.CatCandidate), nil, t0)
	if x != y {
		t.Fatal("one sender without hashes made two groups")
	}
	// Other pools are separate.
	if z, _ := q.Offer("q", sender("x", "r3", peers.CatCandidate), nil, t0); z == x {
		t.Fatal("pools share a group")
	}
}

func TestOrderByEvidenceNotWeight(t *testing.T) {
	q := chase.New(chase.Config{Chases: 10, PerPool: 10, Unlisted: 10})
	// Unlisted senders in many ranges do not corroborate by default.
	for i, r := range []string{"u1", "u2", "u3"} {
		q.Offer("p", sender("u"+r, r, peers.CatUnlisted), h(byte(10+i)), t0)
	}
	q.Offer("p", sender("u0", "u9", peers.CatUnlisted), h(10), t0)
	word, _ := q.Offer("p", sender("b", "r1", peers.CatBackup), h(1), t0.Add(time.Second))
	prio, _ := q.Offer("p", sender("p", "r2", peers.CatPriority), h(2), t0.Add(2*time.Second))
	work, _ := q.Offer("p", sender("w", "r3", peers.CatUnlisted), h(3), t0.Add(3*time.Second))
	q.WorkChecked(work)
	now := t0.Add(4 * time.Second)
	gs := q.Groups("p", now)
	if gs[0].ID != work || gs[1].ID != prio || gs[2].ID != word {
		t.Fatalf("order: %v", ids(gs))
	}
	for _, g := range gs[2:] {
		if g.Evidence != chase.WordOnly {
			t.Fatalf("unlisted senders corroborated: %+v", g)
		}
	}
	// A word-only backup group past its max delay goes to the front of
	// its level, ahead of older unlisted groups.
	late := t0.Add(40 * time.Second)
	gs = q.Groups("p", late)
	if gs[2].ID != word {
		t.Fatalf("late order: %v", ids(gs))
	}
	if q2 := chase.New(chase.Config{UnlistedCount: true}); true {
		q2.Offer("p", sender("a", "r1", peers.CatUnlisted), h(1), t0)
		q2.Offer("p", sender("b", "r2", peers.CatUnlisted), h(1), t0)
		if g := q2.Groups("p", t0); g[0].Evidence != chase.Corroborated {
			t.Fatal("UnlistedCount ignored")
		}
	}
}

func ids(gs []chase.Group) []uint64 {
	var out []uint64
	for _, g := range gs {
		out = append(out, g.ID)
	}
	return out
}

func TestNextLimitsAndBackoff(t *testing.T) {
	q := chase.New(chase.Config{Chases: 2, PerPool: 1, Unlisted: 1, Backoff: time.Minute, MaxBackoff: 4 * time.Minute})
	g1, _ := q.Offer("p", sender("a", "r1", peers.CatPriority), h(1), t0)
	q.Offer("p", sender("b", "r2", peers.CatBackup), h(2), t0)
	q.Offer("q", sender("u", "r3", peers.CatUnlisted), h(3), t0)
	q.Offer("r", sender("v", "r4", peers.CatUnlisted), h(4), t0)
	j, ok := q.Next(t0)
	if !ok || j.Group.ID != g1 || j.Sender.ID != "a" {
		t.Fatalf("first job %+v", j)
	}
	// Pool p has its one chase; the unlisted part of the share allows one.
	j2, ok := q.Next(t0)
	if !ok || j2.Group.Pool == "p" || j2.Sender.Cat != peers.CatUnlisted {
		t.Fatalf("second job %+v", j2)
	}
	if _, ok := q.Next(t0); ok {
		t.Fatal("ran more than the limit")
	}
	// Led nowhere: a is backed off and leaves the group, which ends.
	q.Finish(g1, chase.Nowhere, nil, t0)
	if _, off := q.BackedOff("a", t0.Add(30*time.Second)); !off {
		t.Fatal("not backed off")
	}
	q.Finish(j2.Group.ID, chase.Nowhere, nil, t0)
	j3, ok := q.Next(t0)
	if !ok || j3.Sender.ID != "b" {
		t.Fatalf("third job %+v", j3)
	}
	q.Finish(j3.Group.ID, chase.Gone, nil, t0)
	// Clear the remaining unlisted group out of the way.
	for j, ok := q.Next(t0); ok; j, ok = q.Next(t0) {
		q.Finish(j.Group.ID, chase.Gone, nil, t0)
	}
	// While a is backed off its new group waits; then it runs.
	g, _ := q.Offer("p", sender("a", "r1", peers.CatPriority), h(5), t0.Add(30*time.Second))
	if _, ok := q.Next(t0.Add(30 * time.Second)); ok {
		t.Fatal("a backed-off sender ran a chase")
	}
	if w := q.Wait(t0.Add(30 * time.Second)); w != 30*time.Second {
		t.Fatalf("wait %v", w)
	}
	if j, ok := q.Next(t0.Add(2 * time.Minute)); !ok || j.Group.ID != g {
		t.Fatal("backoff did not end")
	}
	// Its backoff grows with each failure in a row.
	q.Finish(g, chase.Nowhere, nil, t0.Add(2*time.Minute))
	if until, _ := q.BackedOff("a", t0.Add(2*time.Minute)); !until.Equal(t0.Add(4 * time.Minute)) {
		t.Fatalf("second backoff until %v", until.Sub(t0))
	}
	// A success resets it.
	g, _ = q.Offer("p", sender("a", "r1", peers.CatPriority), h(6), t0.Add(5*time.Minute))
	if j, ok := q.Next(t0.Add(5 * time.Minute)); !ok || j.Group.ID != g {
		t.Fatalf("job %+v %v", j, ok)
	}
	q.Finish(g, chase.Switched, nil, t0.Add(5*time.Minute))
	if _, off := q.BackedOff("a", t0.Add(5*time.Minute)); off {
		t.Fatal("success kept the backoff")
	}
}

func TestFalseBranchAndKnownBad(t *testing.T) {
	q := chase.New(chase.Config{})
	g, _ := q.Offer("p", sender("a", "r1", peers.CatBackup), h(1), t0)
	q.Offer("p", sender("b", "r2", peers.CatBackup), h(1), t0)
	if _, ok := q.Next(t0); !ok {
		t.Fatal("no job")
	}
	q.Finish(g, chase.False, []hash.Hash{{1}, {7}}, t0)
	if len(q.Groups("p", t0)) != 0 {
		t.Fatal("false group kept")
	}
	if !q.IsBad(hash.Hash{7}, t0) || q.IsBad(hash.Hash{7}, t0.Add(25*time.Hour)) {
		t.Fatal("known-bad list")
	}
	if _, ok := q.Offer("p", sender("c", "r3", peers.CatPriority), h(1), t0); ok {
		t.Fatal("a known-bad hash started a group")
	}
}

func TestConfirmMergeAndDrop(t *testing.T) {
	q := chase.New(chase.Config{})
	g, _ := q.Offer("p", sender("b", "r1", peers.CatBackup), h(1), t0)
	o, _ := q.Offer("p", sender("c", "r2", peers.CatCandidate), h(9), t0)
	q.Confirm(g, sender("p1", "r5", peers.CatPriority), false)
	gs := q.Groups("p", t0)
	if gs[0].Evidence != chase.WordOnly || len(gs[0].Denied) != 1 {
		t.Fatalf("denial: %+v", gs[0])
	}
	q.Confirm(g, sender("p2", "r6", peers.CatPriority), true)
	j, ok := q.Next(t0)
	if !ok || j.Group.ID != g || j.Sender.ID != "p2" || j.Group.Evidence != chase.Corroborated {
		t.Fatalf("after confirming %+v", j)
	}
	// The chase learns the branch also holds 9: the groups merge.
	q.AddHashes(g, []hash.Hash{{2}, {9}})
	if gs := q.Groups("p", t0); len(gs) != 1 || len(gs[0].Senders) != 3 {
		t.Fatalf("merge: %+v", gs)
	}
	out := q.Finish(g, chase.Switched, nil, t0)
	if len(out.Denied) != 1 || out.Denied[0] != "p1" {
		t.Fatalf("finished group %+v", out)
	}
	_ = o
	// Drop takes a sender out of waiting groups.
	q.Offer("p", sender("d", "r7", peers.CatBackup), h(3), t0)
	q.Drop("d")
	if len(q.Groups("p", t0)) != 0 {
		t.Fatal("drop kept the group")
	}
}

func TestCapPerPool(t *testing.T) {
	q := chase.New(chase.Config{GroupsPerPool: 2})
	q.Offer("p", sender("u1", "r1", peers.CatUnlisted), h(1), t0)
	q.Offer("p", sender("u2", "r2", peers.CatUnlisted), h(2), t0)
	if _, ok := q.Offer("p", sender("u3", "r3", peers.CatUnlisted), h(3), t0.Add(time.Second)); ok {
		t.Fatal("a newer word-only unlisted group displaced an older one")
	}
	if _, ok := q.Offer("p", sender("p", "r4", peers.CatPriority), h(4), t0); !ok {
		t.Fatal("a priority group was refused")
	}
	if n := len(q.Groups("p", t0)); n != 2 {
		t.Fatalf("%d groups, cap 2", n)
	}
}
