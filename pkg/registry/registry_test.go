package registry_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/registry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

var series = wire.SeriesSection(hash.Zero, hash.Hash{1})

func TestRoundTripAndFetch(t *testing.T) {
	doc, err := registry.Encode([]registry.Entry{{Addr: "203.0.113.5:8646", Follows: []wire.Section{series}}})
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/reg.json" {
			http.NotFound(w, r)
			return
		}
		w.Write(doc)
	}))
	defer srv.Close()
	es, err := registry.Fetch(context.Background(), srv.Client(), srv.URL+"/reg.json")
	if err != nil || len(es) != 1 || es[0].Addr != "203.0.113.5:8646" || es[0].Follows[0] != series {
		t.Fatalf("%+v %v", es, err)
	}
	if _, err := registry.Fetch(context.Background(), srv.Client(), srv.URL+"/missing"); err == nil {
		t.Fatal("404 accepted")
	}
	path := filepath.Join(t.TempDir(), "reg.json")
	os.WriteFile(path, doc, 0o600)
	for _, loc := range []string{path, "file://" + path} {
		if es, err := registry.Fetch(context.Background(), nil, loc); err != nil || len(es) != 1 {
			t.Fatalf("%s: %v", loc, err)
		}
	}
}

func TestParse(t *testing.T) {
	good := `{"format":1,"nodes":[
		{"addr":"1.2.3.4:1","follows":["` + series.Text() + `","bad"]},
		{"addr":"no-port","follows":["` + series.Text() + `"]},
		{"addr":"1.2.3.5:1","follows":["bad"]}]}`
	es, err := registry.Parse([]byte(good))
	if err != nil || len(es) != 1 || len(es[0].Follows) != 1 {
		t.Fatalf("%+v %v", es, err)
	}
	for _, bad := range []string{`{"format":2,"nodes":[]}`, `[]`, `nonsense`} {
		if _, err := registry.Parse([]byte(bad)); !errors.Is(err, registry.ErrFormat) {
			t.Errorf("%s: %v", bad, err)
		}
	}
	big := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(strings.Repeat(" ", registry.MaxSize+10)))
	}))
	defer big.Close()
	if _, err := registry.Fetch(context.Background(), big.Client(), big.URL); err == nil {
		t.Fatal("oversized registry accepted")
	}
}
