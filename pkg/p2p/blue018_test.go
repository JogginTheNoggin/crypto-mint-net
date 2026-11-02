package p2p_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/registry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// RED-043, D150: unless the policy allows local peers, a registry
// location never makes the node connect into its own network.
func TestRegistryRefusesLocalAddresses(t *testing.T) {
	doc, err := registry.Encode([]registry.Entry{{Addr: "203.0.113.5:8646", Follows: []wire.Section{wire.SeriesSection(hash.Zero, exSeries.ID())}}})
	must(t, err)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1); w.Write(doc) }))
	defer srv.Close()

	d := startNode(t, "d", func(c *p2p.Config) {
		c.AllowLocalPeers = false
		c.Registries = []string{srv.URL, "http://169.254.169.254/latest/meta-data/"}
	})
	if n := d.h.LoadRegistries(context.Background()); n != 0 || hits.Load() != 0 {
		t.Fatalf("a registry on loopback gave %d candidates, %d requests", n, hits.Load())
	}
	if log := d.log.String(); strings.Count(log, "not public") < 2 {
		t.Fatalf("log does not say why:\n%s", log)
	}

	// Allowed, the same registry is read.
	e := startNode(t, "e", func(c *p2p.Config) { c.Registries = []string{srv.URL} })
	if n := e.h.LoadRegistries(context.Background()); n != 1 || hits.Load() != 1 {
		t.Fatalf("allowed: %d candidates, %d requests", n, hits.Load())
	}
}
