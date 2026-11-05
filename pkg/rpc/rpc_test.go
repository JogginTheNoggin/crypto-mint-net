package rpc_test

import (
	"context"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/testutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

var P = testutil.P

type fixture struct {
	t    *testing.T
	dir  string
	s    *series.Series
	k    *testutil.Keys
	pool rpc.PoolRef
	n    *node.Node
	c    *rpc.Client
	srv  *httptest.Server
}

func writePolicy(t *testing.T, dir string, listen string) *series.Series {
	t.Helper()
	s := testutil.ExampleSeries()
	id := s.ID()
	p := nodepolicy.Default()
	p.Series = []nodepolicy.SeriesInfo{{Params: s.Params()}}
	p.Follow = []nodepolicy.Follow{{Series: &id, Pools: []uint64{1}}}
	p.RPC.Listen = listen
	if err := nodepolicy.Save(filepath.Join(dir, nodepolicy.FileName), p); err != nil {
		t.Fatal(err)
	}
	return s
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	s := writePolicy(t, dir, "off")
	n, err := node.Open(dir, node.Options{})
	if err != nil {
		t.Fatal(err)
	}
	tok, err := rpc.LoadOrCreateToken(dir)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(rpc.NewHandler(n, tok))
	f := &fixture{
		t: t, dir: dir, s: s, k: testutil.NewKeys(s), n: n, srv: srv,
		pool: rpc.PoolRef{Series: s.ID(), Pool: 1},
		c:    rpc.NewClient(strings.TrimPrefix(srv.URL, "http://"), tok),
	}
	t.Cleanup(func() { srv.Close(); n.Close() })
	return f
}

func code(err error) string {
	var e *rpc.Error
	if errors.As(err, &e) {
		return e.Code
	}
	return "not an rpc error: " + errorString(err)
}

func errorString(err error) string {
	if err == nil {
		return "nil"
	}
	return err.Error()
}

func TestPaymentFlow(t *testing.T) {
	f := newFixture(t)
	claim := f.k.Claim(4, P("alice", 100))
	r, err := f.c.SubmitTx(claim)
	if err != nil {
		t.Fatal(err)
	}
	if r.TID != claim.TID(f.s.Hash()) || r.Pool != 1 || !r.Forward {
		t.Fatalf("%+v", r)
	}
	if p, _ := f.c.Payment(f.s.ID(), 4, r.TID); !p.Pending || p.Selected || p.Tx != hex.EncodeToString(claim.Encode()) {
		t.Fatalf("payment %+v", p)
	}
	b, err := f.c.Build(f.pool, 0)
	if err != nil {
		t.Fatal(err)
	}
	// With no credit yet the entry ties with the empty ledger, so it is
	// held but not selected until a layer lands on it.
	if b.Entry == "" || b.Switched {
		t.Fatalf("build %+v", b)
	}
	w, err := f.c.Work(f.pool, b.Hash, 0, 2000)
	if err != nil {
		t.Fatal(err)
	}
	if w.Strength < 5 || w.Layer == "" || !w.Switched {
		t.Fatalf("work %+v", w)
	}
	p, err := f.c.Payment(f.s.ID(), 4, r.TID)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Selected || p.Entry == nil || *p.Entry != b.Hash || p.From == "0" {
		t.Fatalf("payment %+v", p)
	}
	hs, err := f.c.Holdings(f.s.ID(), 4)
	if err != nil {
		t.Fatal(err)
	}
	if len(hs) != 1 || hs[0].Address != f.k.Addr("alice") || hs[0].Amount != 100 {
		t.Fatalf("holdings %+v", hs)
	}
	st, err := f.c.Status()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Pools) != 1 || st.Pools[0].Tip != b.Hash || st.Pools[0].Height != 1 || st.Pools[0].Stored != 3 {
		t.Fatalf("status %+v", st)
	}
	// The node names its release (W-10, D184).
	if st.Release != node.RunningRelease() {
		t.Fatalf("release %+v, want %+v", st.Release, node.RunningRelease())
	}
	ss, err := f.c.Series()
	if err != nil || len(ss) != 1 || ss[0].ID != f.s.ID() || ss[0].Pools != 2 {
		t.Fatalf("series %+v %v", ss, err)
	}
}

func TestSubmitEntryAndLayer(t *testing.T) {
	f := newFixture(t)
	claim := f.k.Claim(5, P("bob", 100))
	e, h, err := entry.Build(f.s, f.s.PoolID(1), []*tx.Tx{claim}, 0)
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.c.SubmitEntry(f.pool, e)
	if err != nil {
		t.Fatal(err)
	}
	if r.Hash != h || r.Switched {
		t.Fatalf("%+v", r)
	}
	if _, err := f.c.SubmitEntry(f.pool, e); code(err) != rpc.CodeConflict {
		t.Fatalf("second copy: %v", err)
	}
	l, _ := layer.Grind(f.s.Hash(), h, 0, 100)
	lr, err := f.c.SubmitLayer(f.pool, l)
	if err != nil {
		t.Fatal(err)
	}
	if lr.Hash != l.Hash(f.s.Hash()) || !lr.Switched {
		t.Fatalf("%+v", lr)
	}
	if _, err := f.c.SubmitLayer(f.pool, layer.Layer{Entry: hash.Hash{9}}); code(err) != rpc.CodeNotFound {
		t.Fatalf("layer on unknown entry: %v", err)
	}
	// An entry whose parent is unknown.
	orphan, _, _ := entry.Build(f.s, hash.Hash{7}, nil, 0)
	if _, err := f.c.SubmitEntry(f.pool, orphan); code(err) != rpc.CodeNotFound {
		t.Fatalf("orphan: %v", err)
	}
	// An entry for the wrong pool.
	if _, err := f.c.SubmitEntry(rpc.PoolRef{Series: f.s.ID(), Pool: 0}, e); code(err) != rpc.CodeNotFound {
		t.Fatalf("unfollowed pool: %v", err)
	}
}

func TestErrors(t *testing.T) {
	f := newFixture(t)
	claim := f.k.Claim(4, P("alice", 100))
	f.c.SubmitTx(claim)
	if _, err := f.c.SubmitTx(claim); code(err) != rpc.CodeConflict {
		t.Fatalf("repeat: %v", err)
	}
	bad := f.k.Claim(6, P("alice", 99))
	if _, err := f.c.SubmitTx(bad); code(err) != rpc.CodeInvalid {
		t.Fatalf("bad claim: %v", err)
	}
	other := testutil.MustSeries(func() series.Params { p := testutil.ExampleParams(); p.Mints = 12; return p }())
	if _, err := f.c.SubmitTx(testutil.NewKeys(other).Claim(4, P("x", 100))); code(err) != rpc.CodeNotFound {
		t.Fatalf("unknown series: %v", err)
	}
	if _, err := f.c.Build(rpc.PoolRef{Series: f.s.ID(), Pool: 1}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := f.c.Build(f.pool, 0); code(err) != rpc.CodeNothingPending && code(err) != rpc.CodeConflict {
		t.Fatalf("empty build: %v", err)
	}
	if _, err := f.c.Work(f.pool, hash.Hash{1}, 0, 0); code(err) != rpc.CodeBadRequest {
		t.Fatalf("zero tries: %v", err)
	}
	// RED-006: a build may not ask for more than MaxBuildTries nonces.
	if _, err := f.c.Build(f.pool, rpc.MaxBuildTries+1); code(err) != rpc.CodeBadRequest {
		t.Fatalf("unbounded build: %v", err)
	}
	if _, err := f.c.Holdings(f.s.ID(), 99); code(err) != rpc.CodeBadRequest {
		t.Fatalf("serial out of range: %v", err)
	}
	if _, err := f.c.Holdings(hash.Hash{3}, 1); code(err) != rpc.CodeNotFound {
		t.Fatalf("unknown series: %v", err)
	}
	if _, err := f.c.Payment(f.s.ID(), 0, hash.Hash{}); code(err) != rpc.CodeNotFound {
		t.Fatalf("unfollowed pool: %v", err)
	}
}

func raw(t *testing.T, f *fixture, method, path, token, body string) int {
	t.Helper()
	req, _ := http.NewRequest(method, f.srv.URL+path, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	return resp.StatusCode
}

func TestAuthAndMalformedRequests(t *testing.T) {
	f := newFixture(t)
	tok, _ := rpc.LoadOrCreateToken(f.dir)
	if got := raw(t, f, "GET", "/v1/status", "", ""); got != http.StatusUnauthorized {
		t.Fatalf("no token: %d", got)
	}
	if got := raw(t, f, "GET", "/v1/status", strings.Repeat("0", 64), ""); got != http.StatusUnauthorized {
		t.Fatalf("wrong token: %d", got)
	}
	if got := raw(t, f, "GET", "/v1/status", tok, ""); got != http.StatusOK {
		t.Fatalf("right token: %d", got)
	}
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/v1/tx", "{"},
		{"POST", "/v1/tx", `{"tx":"zz"}`},
		{"POST", "/v1/tx", `{"tx":"00"}`},
		{"POST", "/v1/tx", `{"tx":"", "extra":1}`},
		{"POST", "/v1/tx", `{"tx":""} {}`},
		{"POST", "/v1/entry", `{"entry":"00"}`},
		{"POST", "/v1/layer", `{"layer":"00"}`},
		{"GET", "/v1/payment?series=xyz&serial=1&tid=00", ""},
		{"GET", "/v1/holdings?series=" + f.s.ID().String() + "&serial=x", ""},
	} {
		if got := raw(t, f, c.method, c.path, tok, c.body); got != http.StatusBadRequest {
			t.Errorf("%s %s %s: %d", c.method, c.path, c.body, got)
		}
	}
	if got := raw(t, f, "POST", "/v1/tx", tok, strings.Repeat(" ", rpc.MaxBody+1)); got != http.StatusBadRequest {
		t.Fatalf("huge body: %d", got)
	}
	if got := raw(t, f, "GET", "/v1/entry", tok, ""); got != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method: %d", got)
	}
}

func TestToken(t *testing.T) {
	dir := t.TempDir()
	a, err := rpc.LoadOrCreateToken(dir)
	if err != nil || len(a) != 64 {
		t.Fatalf("%q %v", a, err)
	}
	b, _ := rpc.LoadOrCreateToken(dir)
	if a != b {
		t.Fatal("token changed")
	}
	info, _ := os.Stat(filepath.Join(dir, rpc.TokenFile))
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
	os.WriteFile(filepath.Join(dir, rpc.TokenFile), []byte("short"), 0o600)
	if _, err := rpc.LoadOrCreateToken(dir); err == nil {
		t.Fatal("bad token file accepted")
	}
}

func TestServeAndDial(t *testing.T) {
	dir := t.TempDir()
	s := writePolicy(t, dir, "127.0.0.1:0")
	n, err := node.Open(dir, node.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() { done <- rpc.Serve(ctx, n, func(a string) { ready <- a }) }()
	select {
	case <-ready:
	case err := <-done:
		t.Fatal(err)
	case <-time.After(10 * time.Second):
		t.Fatal("server did not start")
	}
	c, err := rpc.Dial(dir)
	if err != nil {
		t.Fatal(err)
	}
	k := testutil.NewKeys(s)
	if _, err := c.SubmitTx(k.Claim(7, P("z", 100))); err != nil {
		t.Fatal(err)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, rpc.AddrFile)); !os.IsNotExist(err) {
		t.Fatal("rpc.addr left behind")
	}
	if _, err := rpc.Dial(dir); err == nil {
		t.Fatal("dialed a stopped node")
	}
	if st, _ := n.Engine().Status(engine.PoolKey{Series: s.ID(), Index: 1}); st.Pending != 1 {
		t.Fatalf("pending %d", st.Pending)
	}
}

func TestServeOff(t *testing.T) {
	dir := t.TempDir()
	writePolicy(t, dir, "off")
	n, err := node.Open(dir, node.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer n.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := rpc.Serve(ctx, n, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, rpc.TokenFile)); !os.IsNotExist(err) {
		t.Fatal("token created with the interface off")
	}
}

func TestPeers(t *testing.T) {
	f := newFixture(t)
	if _, err := f.c.Peers(); code(err) != rpc.CodeNotFound {
		t.Fatalf("peers without p2p: %v", err)
	}
	host, err := p2p.New(f.n, p2p.Config{Listen: "127.0.0.1:0", CheckEvery: -1})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- host.Run(ctx, func() { close(ready) }) }()
	<-ready
	defer func() { cancel(); <-done }()
	tok, _ := rpc.LoadOrCreateToken(f.dir)
	h := rpc.NewHandler(f.n, tok)
	h.SetHost(host)
	srv := httptest.NewServer(h)
	defer srv.Close()
	c := rpc.NewClient(strings.TrimPrefix(srv.URL, "http://"), tok)
	r, err := c.Peers()
	if err != nil {
		t.Fatal(err)
	}
	if r.NodeID != host.ID().String() || r.Listen != host.Addr() || len(r.Peers) != 0 {
		t.Fatalf("%+v", r)
	}

	// Peer controls, the tracker and the chase queue.
	const addr = "203.0.113.7:8646"
	p, err := c.Peer(rpc.PeerRequest{Addr: addr, Action: "block", Seconds: -1})
	if err != nil || !p.Status.Blocked || p.Key != "203.0.113.7" {
		t.Fatalf("block: %+v %v", p, err)
	}
	tr, err := c.Tracking()
	if err != nil || !tr.Tracked["203.0.113.7"].Blocked {
		t.Fatalf("tracking: %+v %v", tr, err)
	}
	if p, err = c.Peer(rpc.PeerRequest{Addr: addr, Action: "unblock"}); err != nil || p.Status.Blocked {
		t.Fatalf("unblock: %+v %v", p, err)
	}
	if p, err = c.Peer(rpc.PeerRequest{Addr: addr, Action: "timeout", Seconds: 60}); err != nil || !p.Status.TimedOut {
		t.Fatalf("timeout: %+v %v", p, err)
	}
	if _, err = c.Peer(rpc.PeerRequest{Addr: addr, Action: "template", Template: "detailed"}); err != nil {
		t.Fatal(err)
	}
	rep, err := c.TrackingOf(addr)
	if err != nil || rep.Template != "detailed" {
		t.Fatalf("report: %+v %v", rep, err)
	}
	if _, err = c.Peer(rpc.PeerRequest{Addr: addr, Action: "template", Template: "nope"}); code(err) != rpc.CodeBadRequest {
		t.Fatalf("unknown template: %v", err)
	}
	if _, err = c.Peer(rpc.PeerRequest{Addr: addr, Action: "clear", What: "some"}); code(err) != rpc.CodeBadRequest {
		t.Fatalf("bad clear: %v", err)
	}
	if _, err = c.Peer(rpc.PeerRequest{Addr: addr, Action: "frobnicate"}); code(err) != rpc.CodeBadRequest {
		t.Fatalf("unknown action: %v", err)
	}
	if p, err = c.Peer(rpc.PeerRequest{Addr: addr, Action: "delete"}); err != nil || p.Held || p.Status.TimedOut {
		t.Fatalf("delete: %+v %v", p, err)
	}
	ch, err := c.Chases()
	if err != nil || ch.Groups == nil || len(ch.Groups) != 0 {
		t.Fatalf("chases: %+v %v", ch, err)
	}
}
