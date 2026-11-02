package rpc_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// ORANGE-025: an entry body is read once into one buffer and decoded
// there. Through json's Decoder a 4 MiB entry cost 36 MiB of
// allocations on 46bcb9e; now at most 20 MiB (the hex, the bytes, the
// client's copy).
// The body keeps decodeBody's rules: unknown fields and a second value
// are refused.
func TestEntryBodyReadOnce(t *testing.T) {
	dir := t.TempDir()
	s := writePolicy(t, dir, "off")
	n, err := node.Open(dir, node.Options{})
	must(t, err)
	tok, _ := rpc.LoadOrCreateToken(dir)
	srv := httptest.NewServer(rpc.NewHandler(n, tok))
	t.Cleanup(func() { srv.Close(); n.Close() })
	post := func(body string) (int, string) {
		t.Helper()
		req, _ := http.NewRequest("POST", srv.URL+"/v1/entry", strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := http.DefaultClient.Do(req)
		must(t, err)
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(b)
	}
	ref := `{"series":"` + s.ID().String() + `","pool":1,`
	const size = 4 << 20
	big := ref + `"entry":"` + strings.Repeat("00", size) + `"}`
	runtime.GC()
	var m0, m1 runtime.MemStats
	runtime.ReadMemStats(&m0)
	code, body := post(big)
	runtime.ReadMemStats(&m1)
	if code != http.StatusBadRequest || strings.Contains(body, "not hex") {
		t.Fatalf("a junk entry: %d %s", code, body)
	}
	if got := m1.TotalAlloc - m0.TotalAlloc; got > 5*size && !raceEnabled {
		t.Fatalf("a %d MiB entry allocated %d MiB, want at most %d", size>>20, got>>20, 5*size>>20)
	}
	for _, c := range []struct{ body, want string }{
		{ref + `"entry":"00","extra":1}`, "unknown"},
		{ref + `"entry":"00"} {}`, "body"},
		{ref + `"entry":"zz"}`, "entry: not hex"},
		{`{"entry":"00"`, "body"},
	} {
		code, body := post(c.body)
		if code != http.StatusBadRequest || !strings.Contains(body, c.want) {
			t.Fatalf("%s: %d %s, want %q", c.body, code, body, c.want)
		}
	}
}
