package rpc

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/plain"
)

// HelloPath is the one call that needs no token: the server proves that it
// knows the node's tokens before a client sends one (RED-066, D162).
//
// rpc.addr outlives a node that stops without cleaning up (a crash, a kill,
// a power loss), and any process on the machine may then listen on that
// port. Dial therefore sends GET /v1/hello?nonce=N with a fresh random N,
// and sends its token only if the reply holds HMAC-SHA256(token, the
// address in rpc.addr and N). A stranger on the port knows neither token,
// and a node at another address answers for that address, so neither can
// answer; the client then reports the node as not running.
const HelloPath = "/v1/hello"

// HelloReply proves the server holds the node's tokens: Full is the MAC
// with the full token, View the one with the view token ("" for none).
type HelloReply struct {
	Full string `json:"full"`
	View string `json:"view,omitempty"`
}

// helloMAC is the proof for token, the address the client dialled and its
// nonce.
func helloMAC(token []byte, addr, nonce string) string {
	m := hmac.New(sha256.New, token)
	// The label stays "poon-rpc-hello" whatever the name (see pkg/brand).
	m.Write([]byte("poon-rpc-hello\x00" + addr + "\x00" + nonce))
	return hex.EncodeToString(m.Sum(nil))
}

// SetAddr sets the address the server wrote to rpc.addr, which its hello
// proof binds. Call it before serving.
func (h *Handler) SetAddr(addr string) { h.addr = addr }

func (h *Handler) hello(w http.ResponseWriter, r *http.Request) {
	nonce := r.URL.Query().Get("nonce")
	if b, err := hex.DecodeString(nonce); err != nil || len(b) != 32 {
		writeError(w, badRequest("nonce must be 32 bytes of hex"))
		return
	}
	rep := HelloReply{Full: helloMAC(h.token, h.addr, nonce)}
	if len(h.view) > 0 {
		rep.View = helloMAC(h.view, h.addr, nonce)
	}
	writeJSON(w, rep)
}

// ErrNoProof is why a server that answered hello wrongly, or not at all,
// is not trusted.
var ErrNoProof = errors.New("it did not prove it knows this node's token")

// prove asks the server at c.base for its hello and checks it against
// c.token and addr, the address read from rpc.addr. It sends no token.
func (c *Client) prove(addr string) error {
	if c.token == "" {
		// Anyone can make the proof for an empty token, so an empty token
		// file (a view token cut short, say) proves nothing (RED-W-021).
		return ErrEmptyToken
	}
	var n [32]byte
	if _, err := rand.Read(n[:]); err != nil {
		return err
	}
	nonce := hex.EncodeToString(n[:])
	hc := &http.Client{
		Timeout: 10 * time.Second,
		// A redirect elsewhere is not the node either.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := hc.Get(c.base + HelloPath + "?nonce=" + nonce)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w (HTTP %d)", ErrNoProof, resp.StatusCode)
	}
	var rep HelloReply
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4096)).Decode(&rep); err != nil {
		return ErrNoProof
	}
	want := helloMAC([]byte(c.token), addr, nonce)
	if hmac.Equal([]byte(rep.Full), []byte(want)) || (rep.View != "" && hmac.Equal([]byte(rep.View), []byte(want))) {
		return nil
	}
	return ErrNoProof
}

// ErrEmptyToken is why a client with an empty token does not dial: no
// server can prove it knows that token, since anyone can.
var ErrEmptyToken = errors.New("the token is empty, so no server can prove it knows it; write the token again (poon node makes it at start)")

// NotProvedError is what Dial returns when the server at the address in
// rpc.addr did not prove it knows the node's token: nothing answers there
// (Err is the dial error), or something that is not this node does
// (Err wraps ErrNoProof). It wraps ErrNotRunning, so a command falls back
// as it does when rpc.addr is missing. The token was not sent, and nothing
// the server sent is printed.
type NotProvedError struct {
	Addr string
	Err  error
}

func (e *NotProvedError) Error() string {
	return fmt.Sprintf("%v: the server at %s (from %s): %s; the file may be left from a node that stopped without cleaning up, so the token was not sent (a node of an older poon cannot prove it either: restart it)",
		ErrNotRunning, plain.Safe(e.Addr), AddrFile, plain.Safe(e.Err.Error()))
}

func (e *NotProvedError) Unwrap() []error { return []error{ErrNotRunning, e.Err} }

// dialProved returns a client for addr with token once the server there
// has proved it knows token, or a *NotProvedError.
func dialProved(addr, token string) (*Client, error) {
	c := NewClient(addr, token)
	if err := c.prove(addr); err != nil {
		return nil, &NotProvedError{Addr: addr, Err: err}
	}
	return c, nil
}
