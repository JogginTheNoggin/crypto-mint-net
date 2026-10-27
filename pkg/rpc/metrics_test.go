package rpc_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// scrape gets /v1/metrics from base with token and returns the status
// and body.
func scrape(t *testing.T, base, token string) (int, string, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", base+"/v1/metrics", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, resp.Header.Get("Content-Type"), string(b)
}

func TestMetrics(t *testing.T) {
	f := newFixture(t)
	if st, _, _ := scrape(t, f.srv.URL, ""); st != http.StatusUnauthorized {
		t.Fatalf("no token: status %d, want 401", st)
	}
	tok, err := rpc.LoadOrCreateToken(f.dir)
	if err != nil {
		t.Fatal(err)
	}
	st, typ, body := scrape(t, f.srv.URL, tok)
	if st != 200 || !strings.HasPrefix(typ, "text/plain; version=0.0.4") {
		t.Fatalf("status %d, type %q", st, typ)
	}
	want := `poo_pool_height{series="` + f.s.ID().String() + `",pool="1"} 0`
	for _, s := range []string{"# TYPE poo_pool_height gauge", want, "poo_pool_pending{", "poo_pool_weight{"} {
		if !strings.Contains(body, s) {
			t.Errorf("metrics lack %q:\n%s", s, body)
		}
	}
	if strings.Contains(body, "poo_peers_connected") {
		t.Error("peer metrics without a p2p host")
	}
}

func TestMetricsWithHost(t *testing.T) {
	f := newFixture(t)
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
	_, _, body := scrape(t, srv.URL, tok)
	for _, s := range []string{
		`poo_peers_connected{direction="inbound"} 0`, `poo_peers_connected{direction="outbound"} 0`,
		"# TYPE poo_peer_sent_total counter", "poo_peer_book_records ",
	} {
		if !strings.Contains(body, s) {
			t.Errorf("metrics lack %q:\n%s", s, body)
		}
	}
}
