package peerport_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peerport"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

var (
	now = time.Unix(1_800_000_000, 0).UTC()
	sec = wire.SeriesSection(hash.Zero, hash.Hash{1})
)

// node returns an empty book and tracker.
func node(t *testing.T, lim peers.Limits) (*peers.Book, *tracking.Tracker) {
	t.Helper()
	b, err := peers.Open("", lim)
	if err != nil {
		t.Fatal(err)
	}
	tr, err := tracking.New(tracking.Config{}, "")
	if err != nil {
		t.Fatal(err)
	}
	return b, tr
}

// source is a book with a priority, a backup, a candidate and an
// archived peer, one blocked address and one blocked range.
func source(t *testing.T) ([]peers.Record, map[string]tracking.Status, map[string]time.Time) {
	t.Helper()
	b, tr := node(t, peers.Limits{})
	for _, a := range []string{"8.8.1.1:1", "8.9.1.1:1", "8.10.1.1:1", "8.11.1.1:1"} {
		if err := b.Add(a, []wire.Section{sec}, peers.FromRegistry, now); err != nil {
			t.Fatal(err)
		}
	}
	b.Verify("8.8.1.1:1", sec, peers.Priority, now)
	b.Verify("8.9.1.1:1", sec, peers.Backup, now)
	b.Archive("8.11.1.1:1")
	b.SetLabel("8.8.1.1:1", "fast one")
	tr.BlockFor("9.9.9.9", -1, now)
	tr.BlockRange("7.7.0.1:1", time.Hour, now)
	st := map[string]tracking.Status{}
	for _, k := range tr.Keys() {
		st[k] = tr.Status(k, now)
	}
	return b.Records(), st, tr.Ranges()
}

func roundTrip(t *testing.T, f peerport.File) peerport.File {
	t.Helper()
	var buf bytes.Buffer
	if err := f.Write(&buf); err != nil {
		t.Fatal(err)
	}
	g, err := peerport.Read(&buf)
	if err != nil {
		t.Fatalf("%v\n%s", err, buf.String())
	}
	return g
}

// D157: an export holds the categories asked for, or all, with labels and
// blocks, and never points or flags.
func TestExportCategories(t *testing.T) {
	recs, st, rng := source(t)
	all, err := peerport.Export(recs, st, rng, nil, now)
	if err != nil {
		t.Fatal(err)
	}
	all = roundTrip(t, all)
	if len(all.Peers) != 4 || len(all.Blocks) != 2 || all.Peers[2].Addr != "8.8.1.1:1" || all.Peers[2].Label != "fast one" {
		t.Fatalf("all: %+v", all)
	}
	pri, _ := peerport.Export(recs, st, rng, []string{"priority", "backup"}, now)
	if len(pri.Peers) != 2 || len(pri.Blocks) != 0 {
		t.Fatalf("priority and backup: %+v", pri)
	}
	bl, _ := peerport.Export(recs, st, rng, []string{peerport.Blocked}, now)
	if len(bl.Peers) != 0 || len(bl.Blocks) != 2 {
		t.Fatalf("blocked: %+v", bl)
	}
	if _, err := peerport.Export(recs, st, rng, []string{"friends"}, now); err == nil {
		t.Fatal("an unknown category was taken")
	}
	var buf bytes.Buffer
	all.Write(&buf)
	for _, not := range []string{"points", "flag", "node_id"} {
		if strings.Contains(buf.String(), not) {
			t.Fatalf("the export holds %q:\n%s", not, buf.String())
		}
	}
}

// D157, RED-053: a file with a control character, a bad address or an
// unknown field is refused whole.
func TestReadRefuses(t *testing.T) {
	for name, doc := range map[string]string{
		"escape in addr":  `{"kind":"poo-peers","format":1,"peers":[{"addr":"a\u001b[8A.example:1","policies":[{"section":"*/*/*/*"}]}]}`,
		"escape in label": `{"kind":"poo-peers","format":1,"peers":[{"addr":"a.example:1","policies":[{"section":"*/*/*/*"}],"label":"\u001b[J"}]}`,
		"no port":         `{"kind":"poo-peers","format":1,"peers":[{"addr":"a.example","policies":[{"section":"*/*/*/*"}]}]}`,
		"no policy":       `{"kind":"poo-peers","format":1,"peers":[{"addr":"a.example:1","policies":[]}]}`,
		"unknown field":   `{"kind":"poo-peers","format":1,"peers":[],"points":1}`,
		"block both":      `{"kind":"poo-peers","format":1,"peers":[],"blocks":[{"addr":"1.2.3.4","range":"v4:1.2","until":"2030-01-01T00:00:00Z"}]}`,
		"bad range":       `{"kind":"poo-peers","format":1,"peers":[],"blocks":[{"range":"v4:300.2","until":"2030-01-01T00:00:00Z"}]}`,
		"other kind":      `{"kind":"poo-node-policies","format":1}`,
		"newer format":    `{"kind":"poo-peers","format":2,"peers":[]}`,
	} {
		if _, err := peerport.Read(strings.NewReader(doc)); err == nil {
			t.Errorf("%s: taken", name)
		}
	}
	good := `{"kind":"poo-peers","format":1,"peers":[{"addr":"a.example:1","policies":[{"section":"*/*/*/*"}],"label":"ok"}],"blocks":[{"range":"v4:1.2","until":"2030-01-01T00:00:00Z"},{"addr":"1.2.3.4","until":"2030-01-01T00:00:00Z"}]}`
	if _, err := peerport.Read(strings.NewReader(good)); err != nil {
		t.Fatalf("a good file: %v", err)
	}
}

// D157: every imported address is unlisted, whatever the file says;
// blocks wait for Blocks and ranges for Ranges; nothing is unblocked or
// shortened; a listed address is never blocked; the book's guards hold.
func TestImportRules(t *testing.T) {
	recs, st, rng := source(t)
	f, _ := peerport.Export(recs, st, rng, nil, now)
	f.Peers = append(f.Peers,
		peerport.Peer{Addr: "192.168.1.5:1", Policies: []peerport.Policy{{Section: sec.Text()}}},
		peerport.Peer{Addr: "9.9.9.9:1", Policies: []peerport.Policy{{Section: sec.Text()}}},
	)
	f.Blocks = append(f.Blocks, peerport.Block{Addr: "6.6.6.6", Until: now.Add(time.Hour)}, peerport.Block{Addr: "5.5.5.5", Until: now.Add(-time.Hour)})
	f = roundTrip(t, f)

	b, tr := node(t, peers.Limits{})
	tr.BlockFor("9.9.9.9", time.Minute, now) // already blocked here
	tr.BlockFor("6.6.6.6", 2*time.Hour, now) // longer here than the file's
	n := peerport.Node{Book: b, Tracker: tr, Listed: []string{"8.9.1.1:1"}}

	plan, err := peerport.Import(f, peerport.Options{}, n, false, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Added) != 4 || len(plan.Blocks) != 0 || len(plan.Skipped) != 1 || !strings.Contains(plan.Skipped[0].Detail, "-blocks takes them") {
		t.Fatalf("default plan: %+v", plan)
	}
	refused := map[string]string{}
	for _, l := range plan.Refused {
		refused[l.Addr] = l.Detail
	}
	if !strings.Contains(refused["192.168.1.5:1"], "not a public address") || refused["9.9.9.9:1"] != "blocked here" {
		t.Fatalf("refused: %v", refused)
	}
	if len(b.Records()) != 0 {
		t.Fatal("a dry run changed the book")
	}

	if _, err := peerport.Import(f, peerport.Options{Ranges: true}, n, false, now); err == nil {
		t.Fatal("ranges without blocks")
	}
	plan, err = peerport.Import(f, peerport.Options{Blocks: true}, n, true, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range b.Records() {
		for _, p := range r.Policies {
			if p.State != peers.Unlisted {
				t.Fatalf("%s came in as %v", r.Addr, p.State)
			}
		}
	}
	if r, _ := b.Record("8.8.1.1:1"); r.Label != "fast one" {
		t.Fatalf("label: %+v", r)
	}
	if st := tr.Status("9.9.9.9", now); !st.Blocked || st.BlockedUntil.Before(now.Add(50*365*24*time.Hour)) {
		t.Fatalf("a block until unblocked: %+v", st)
	}
	if st := tr.Status("6.6.6.6", now); !st.BlockedUntil.Equal(now.Add(2 * time.Hour)) {
		t.Fatalf("a file shortened a block: %+v", st)
	}
	if _, ok := tr.Ranges()[peers.Range("7.7.0.1:1")]; ok {
		t.Fatal("a range block was taken without Ranges")
	}
	skipped := ""
	for _, l := range plan.Skipped {
		skipped += l.Addr + " " + l.Detail + "\n"
	}
	if !strings.Contains(skipped, "already ended") || !strings.Contains(skipped, "-ranges takes it") {
		t.Fatalf("skipped:\n%s", skipped)
	}

	// The same file again changes nothing; a range block with Ranges.
	plan, _ = peerport.Import(f, peerport.Options{Blocks: true, Ranges: true}, n, true, now)
	if len(plan.Added) != 0 || len(plan.Blocks) != 1 || !strings.HasPrefix(plan.Blocks[0].Addr, "range ") {
		t.Fatalf("second import: %+v", plan)
	}
	if _, ok := tr.Ranges()[peers.Range("7.7.0.1:1")]; !ok {
		t.Fatal("the range block was not taken")
	}

	// A listed address is never blocked by a file.
	b2, tr2 := node(t, peers.Limits{})
	g := peerport.File{Kind: peerport.Kind, Format: peerport.Format, Peers: []peerport.Peer{}, Blocks: []peerport.Block{{Addr: "8.9.1.1", Until: now.Add(time.Hour)}}}
	plan, _ = peerport.Import(g, peerport.Options{Blocks: true}, peerport.Node{Book: b2, Tracker: tr2, Listed: []string{"8.9.1.1:1"}}, true, now)
	if len(plan.Blocks) != 0 || tr2.Status("8.9.1.1", now).Blocked {
		t.Fatalf("a file blocked a listed address: %+v", plan)
	}
}

// D157: a full book or network range refuses more, and a digest changes
// with the options.
func TestImportGuards(t *testing.T) {
	b, tr := node(t, peers.Limits{MaxRecords: 2, PerRange: 1, MaxUnlisted: 2})
	f := peerport.File{Kind: peerport.Kind, Format: peerport.Format}
	for _, a := range []string{"8.8.1.1:1", "8.8.2.2:1", "8.9.1.1:1", "8.10.1.1:1"} {
		f.Peers = append(f.Peers, peerport.Peer{Addr: a, Policies: []peerport.Policy{{Section: sec.Text()}}})
	}
	plan, err := peerport.Import(f, peerport.Options{}, peerport.Node{Book: b, Tracker: tr}, true, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Added) != 2 || len(plan.Refused) != 2 || len(b.Records()) != 2 {
		t.Fatalf("guards: %+v", plan)
	}
	if peerport.Digest("d", peerport.Options{}, plan) == peerport.Digest("d", peerport.Options{Blocks: true}, plan) {
		t.Fatal("the digest does not cover the options")
	}
}
