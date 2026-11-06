package rpc_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// RED-040 (D148): Client.SubmitEntryHex sends its body's length, not a
// chunked body, so the node sizes its buffer once (ORANGE-025, D147). It
// goes through the client itself, as poon submit does.
func TestSubmitEntryHexSendsItsLength(t *testing.T) {
	var length int64
	var chunked bool
	var got int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		length = r.ContentLength
		chunked = len(r.TransferEncoding) > 0
		b, _ := io.ReadAll(r.Body)
		got = len(b)
		w.Write([]byte(`{"hash":"` + hash.Zero.String() + `"}`))
	}))
	t.Cleanup(srv.Close)
	c := rpc.NewClient(strings.TrimPrefix(srv.URL, "http://"), "tok")
	hexText := []byte(strings.Repeat("ab", 300_000))
	if _, err := c.SubmitEntryHex(rpc.PoolRef{Pool: 1}, hexText); err != nil {
		t.Fatal(err)
	}
	if chunked || length != int64(got) || got < len(hexText) {
		t.Fatalf("Content-Length %d, chunked %v, body %d bytes", length, chunked, got)
	}
}
