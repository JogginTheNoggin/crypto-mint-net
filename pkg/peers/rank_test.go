package peers_test

import (
	"reflect"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
)

func ranked(id, rng string, cat peers.Category, points float64) peers.Ranked {
	return peers.Ranked{ID: id, Range: rng, Outbound: true, Category: cat, Points: points, Fair: points, MayPriority: true}
}

func TestPlace(t *testing.T) {
	s := peers.Slots{Priority: 2, Backup: 1, PriorityPerRange: 1, PerRange: 2, InboundPriority: 1}
	var in []peers.Ranked
	add := func(r peers.Ranked) peers.Category {
		r.Category = s.Place(in, r)
		in = append(in, r)
		return r.Category
	}
	if c := add(ranked("a", "r1", 0, 0)); c != peers.CatPriority {
		t.Fatalf("first peer %v", c)
	}
	// The same range may not hold two priority peers.
	if c := add(ranked("b", "r1", 0, 0)); c != peers.CatBackup {
		t.Fatalf("second from one range %v", c)
	}
	// Two listed peers from r1 already: no room for a third.
	if c := add(ranked("c", "r1", 0, 0)); c != peers.CatCandidate {
		t.Fatalf("third from one range %v", c)
	}
	flagged := ranked("d", "r2", 0, 0)
	flagged.MayPriority = false
	if c := s.Place(in, flagged); c != peers.CatCandidate {
		t.Fatalf("flagged peer with backup full: %v", c)
	}
	inbound := ranked("e", "r3", 0, 0)
	inbound.Outbound = false
	if c := add(inbound); c != peers.CatPriority {
		t.Fatalf("inbound peer %v", c)
	}
	// Priority and backup are full now.
	if c := add(ranked("f", "r4", 0, 0)); c != peers.CatCandidate {
		t.Fatalf("no slot: %v", c)
	}
}

func TestReviewSwapsByMargin(t *testing.T) {
	s := peers.Slots{Priority: 2, Backup: 2, Margin: 1}
	rs := []peers.Ranked{
		ranked("p1", "r1", peers.CatPriority, 5),
		ranked("p2", "r2", peers.CatPriority, 2),
		ranked("b1", "r3", peers.CatBackup, 2.5), // within the margin of p2
		ranked("b2", "r4", peers.CatBackup, 1),
	}
	if got := s.Review(rs); len(got) != 0 {
		t.Fatalf("swapped within the margin: %v", got)
	}
	rs[2].Fair = 3.5
	got := s.Review(rs)
	want := map[string]peers.Category{"p2": peers.CatBackup, "b1": peers.CatPriority}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("review %v, want %v", got, want)
	}
	// Only fair points count for the swap: requests a backup is never sent
	// cannot help a priority peer keep its place.
	rs[1].Points = 100
	if got := s.Review(rs); !reflect.DeepEqual(got, want) {
		t.Fatalf("review on all points: %v", got)
	}
}

func TestReviewFlagsAndFreeSlots(t *testing.T) {
	s := peers.Slots{Priority: 2, Backup: 1, Margin: 1}
	flagged := ranked("p1", "r1", peers.CatPriority, 9)
	flagged.MayPriority = false
	rs := []peers.Ranked{
		flagged,
		ranked("b1", "r2", peers.CatBackup, 1),
		ranked("c1", "r3", peers.CatCandidate, 3),
		ranked("c2", "r4", peers.CatCandidate, 0),
	}
	got := s.Review(rs)
	// p1 loses priority; both priority slots fill, best first (c1 then b1);
	// p1 keeps the backup slot over c2 with more points.
	want := map[string]peers.Category{"p1": peers.CatBackup, "c1": peers.CatPriority, "b1": peers.CatPriority}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("review %v, want %v", got, want)
	}
}

func TestReviewPartialAndOutbound(t *testing.T) {
	s := peers.Slots{Priority: 1, Backup: 1, Margin: 1, PartialPenalty: 2}
	part := ranked("a", "r1", peers.CatCandidate, 3)
	part.Partial = true
	in := ranked("b", "r2", peers.CatCandidate, 2)
	got := s.Review([]peers.Ranked{part, in})
	if got["b"] != peers.CatPriority || got["a"] != peers.CatBackup {
		t.Fatalf("partial coverage ranked first: %v", got)
	}
	// On equal points, the peer this node dialled itself goes first.
	x := ranked("x", "r1", peers.CatCandidate, 1)
	x.Outbound = false
	y := ranked("y", "r2", peers.CatCandidate, 1)
	got = s.Review([]peers.Ranked{x, y})
	if got["y"] != peers.CatPriority {
		t.Fatalf("outbound not preferred: %v", got)
	}
}
