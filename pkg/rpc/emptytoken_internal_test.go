package rpc

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// RED-W-021: anyone can make the hello proof for an empty token, so a
// client with one never dials, even when a server answers with that proof.
func TestEmptyTokenNeverProved(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := &Handler{token: nil}
		h.SetAddr(strings.TrimPrefix(r.Host, ""))
		h.hello(w, r)
	}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "http://")
	_, err := dialProved(addr, "")
	if !errors.Is(err, ErrEmptyToken) || !errors.Is(err, ErrNotRunning) {
		t.Fatalf("an empty token dialled: %v", err)
	}
}
