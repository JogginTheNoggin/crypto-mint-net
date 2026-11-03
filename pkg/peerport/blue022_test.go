package peerport_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peerport"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
)

// RED-058, D158: an import takes at most the book's places for imported
// addresses, and the plan says the addresses will be dialled and how many
// places they take.
func TestImportTakesAShareOfTheBook(t *testing.T) {
	b, tr := node(t, peers.Limits{MaxRecords: 40, PerRange: 16, MaxUnlisted: 10})
	f := peerport.File{Kind: peerport.Kind, Format: peerport.Format}
	for i := 0; i < 30; i++ {
		f.Peers = append(f.Peers, peerport.Peer{Addr: fmt.Sprintf("8.%d.1.1:1", i), Policies: []peerport.Policy{{Section: sec.Text()}}})
	}
	plan, err := peerport.Import(f, peerport.Options{}, peerport.Node{Book: b, Tracker: tr}, true, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Added) != 10 || len(plan.Refused) != 20 || !strings.Contains(plan.Refused[0].Detail, "at most 10 imported addresses") {
		t.Fatalf("plan %+v", plan)
	}
	notes := strings.Join(plan.Notes, "\n")
	if !strings.Contains(notes, "tries these addresses when it is short of peers") || !strings.Contains(notes, "take 10 of the 10 places") || !strings.Contains(notes, "peer remove -imported") {
		t.Fatalf("notes:\n%s", notes)
	}
	if b.Unlisted() != 10 {
		t.Fatalf("%d imported", b.Unlisted())
	}
}

// RED-060, D158: a listed host name in capitals is protected from a
// file's range block of the name.
func TestListedHostNameInCapitalsIsProtected(t *testing.T) {
	b, tr := node(t, peers.Limits{})
	f := peerport.File{Kind: peerport.Kind, Format: peerport.Format, Blocks: []peerport.Block{{Range: "host:boot.example.com", Until: now.Add(time.Hour)}}}
	plan, err := peerport.Import(f, peerport.Options{Blocks: true, Ranges: true}, peerport.Node{Book: b, Tracker: tr, Listed: []string{"Boot.Example.com:8646"}}, true, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Blocks) != 0 || tr.Status("boot.example.com:8646", now).Blocked {
		t.Fatalf("a file blocked a listed name: %+v", plan)
	}
	if !strings.Contains(strings.Join(plan.Notes, " "), "by its name only") {
		t.Fatalf("notes %v", plan.Notes)
	}
}

// RED-059, D158: the plan says how much of IPv4 its range blocks cover.
func TestPlanCountsRangeBlocks(t *testing.T) {
	b, tr := node(t, peers.Limits{})
	f := peerport.File{Kind: peerport.Kind, Format: peerport.Format}
	for i := 0; i < 656; i++ {
		f.Blocks = append(f.Blocks, peerport.Block{Range: fmt.Sprintf("v4:%d.%d", 100+i/256, i%256), Until: now.Add(time.Hour)})
	}
	plan, err := peerport.Import(f, peerport.Options{Blocks: true, Ranges: true}, peerport.Node{Book: b, Tracker: tr}, false, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(plan.Notes, " "), "656 blocks, 656 of them network ranges; the IPv4 ranges cover 1.00% of IPv4") {
		t.Fatalf("notes %v", plan.Notes)
	}
}

// RED-062, D158: ImportIf applies only the plan whose digest it was
// given, from the same call.
func TestImportIfAppliesOnlyThePreviewedPlan(t *testing.T) {
	b, tr := node(t, peers.Limits{})
	f := peerport.File{Kind: peerport.Kind, Format: peerport.Format, Digest: "d", Peers: []peerport.Peer{{Addr: "8.8.1.1:1", Policies: []peerport.Policy{{Section: sec.Text()}}}}}
	n := peerport.Node{Book: b, Tracker: tr}
	plan, err := peerport.Import(f, peerport.Options{}, n, false, now)
	if err != nil {
		t.Fatal(err)
	}
	dg := peerport.Digest(f.Digest, peerport.Options{}, plan)
	// The book changes after the preview: the plan is another one.
	if err := b.Add("8.8.1.1:1", nil, peers.FromPeerList, now); err != nil {
		t.Fatal(err)
	}
	if _, err := peerport.ImportIf(f, peerport.Options{}, n, dg, now); !errors.Is(err, peerport.ErrNotPreviewed) {
		t.Fatalf("applied another plan: %v", err)
	}
	if r, _ := b.Record("8.8.1.1:1"); len(r.Policies) != 0 {
		t.Fatalf("changed the book: %+v", r)
	}
	plan, _ = peerport.Import(f, peerport.Options{}, n, false, now)
	if _, err := peerport.ImportIf(f, peerport.Options{}, n, peerport.Digest(f.Digest, peerport.Options{}, plan), now); err != nil {
		t.Fatal(err)
	}
	if r, _ := b.Record("8.8.1.1:1"); len(r.Policies) != 1 {
		t.Fatalf("not applied: %+v", r)
	}
}
