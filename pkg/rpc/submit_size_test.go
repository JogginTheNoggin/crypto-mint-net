package rpc_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// RED-038 (D144): an entry has no size bound, so the full token may
// submit one larger than MaxBody, and the node decodes it (this one is
// junk, so it is refused as an entry, not as too large). The view token
// is refused before the body is read, however large; other requests stay
// bounded by MaxBody.
func TestSubmitEntryBodySize(t *testing.T) {
	dir := t.TempDir()
	s := writePolicy(t, dir, "off")
	n, err := node.Open(dir, node.Options{})
	must(t, err)
	tok, _ := rpc.LoadOrCreateToken(dir)
	view, _ := rpc.LoadOrCreateViewToken(dir)
	h := rpc.NewHandler(n, tok)
	h.SetViewToken(view)
	srv := httptest.NewServer(h)
	t.Cleanup(func() { srv.Close(); n.Close() })
	post := func(path, token, body string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest("POST", srv.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		must(t, err)
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	big := `{"series":"` + s.ID().String() + `","pool":1,"entry":"` + strings.Repeat("00", rpc.MaxBody) + `"}`
	if code, body := post("/v1/entry", tok, big); code != http.StatusBadRequest || strings.Contains(body, "too large") || !strings.Contains(body, "entry") {
		t.Fatalf("a %d-byte entry body: %d %s", len(big), code, body)
	}
	if code, body := post("/v1/entry", view, big); code != http.StatusForbidden {
		t.Fatalf("the view token's entry: %d %s", code, body)
	}
	if code, body := post("/v1/layer", tok, big); code != http.StatusBadRequest || !strings.Contains(body, "too large") {
		t.Fatalf("a layer body over MaxBody: %d %s", code, body)
	}
	if code, _ := post("/v1/entry", "", big); code != http.StatusUnauthorized {
		t.Fatalf("no token: %d", code)
	}
}
