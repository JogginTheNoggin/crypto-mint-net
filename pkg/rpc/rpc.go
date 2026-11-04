// Package rpc is the node's local control interface: JSON over HTTP, on a
// loopback address by default, for the node's owner and their tools (a
// wallet, a script, the poo command). It is not the peer-to-peer protocol;
// that is package p2p, and this interface only reports on it.
//
// Every request must carry the node's token, "Authorization: Bearer
// <token>". The token is 32 random bytes in hex, created on first start in
// the data directory as rpc.token, readable only by the node's user. A
// second token, rpc.view-token, made the same way, opens the GET endpoints
// only: give it to a monitor or a read-only tool, never rpc.token. The
// address the server actually listens on is written to rpc.addr, so a
// client on the same machine needs only the data directory (see Dial).
//
// Items are sent as the hex of their canonical byte form
// (docs/protocol/encoding.md). Hashes are hex. Weights are decimal strings,
// since they can exceed 64 bits.
//
// Endpoints (all under /v1):
//
//	GET  /status                          every followed pool
//	GET  /series                          the known series
//	POST /tx       {tx}                   submit a transaction
//	POST /entry    {series, pool, entry}  submit an entry
//	POST /layer    {series, pool, layer}  submit a layer
//	POST /build    {series, pool, max_tries, device, threads, for}
//	POST /work     {series, pool, entry, start, tries, device, threads, for}
//	GET  /payment?series=&serial=&tid=    where a transaction stands, the
//	                                      transaction, and known rivals
//	GET  /holdings?series=&serial=        unspent portions of a mint
//	GET  /peers                           connected peers (when p2p runs)
//	GET  /tracking[?addr=]                tracked peers, or one peer's record
//	GET  /chases                          the chase queue
//	POST /peer     {addr, action, ...}    time out, block, unblock, clear,
//	                                      set a template for or delete a peer
//	GET  /pools?FILTER                    followed pools the filter covers
//	GET  /ledger?series=&pool=&below=&limit=&FILTER
//	                                      a page of the selected ledger
//	GET  /pending?FILTER                  pending transactions
//	GET  /unspent?FILTER                  unspent portions
//	GET  /tx?series=&id=[&serial=]        one transaction and its status
//	GET  /address?series=&id=&limit=&FILTER
//	                                      an address's balance and history
//	GET  /watch?FILTER                    a live event stream (NDJSON)
//	GET  /follow                          what the node follows
//	POST /follow   {remove, issuer, series, pools, mints, no_reload}
//	                                      change it and apply it
//	POST /reload                          apply the node policy again
//	GET  /metrics                         status and peer counts in
//	                                      Prometheus text format
//
// FILTER is the parameters of pkg/filter: series, pool, serial, address,
// policy, tid and kind, each comma-separated.
//
// Errors are {"error": message, "code": code} with a matching HTTP status.
package rpc

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/grind"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/mempool"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/watch"
)

// File names in the data directory.
const (
	TokenFile = "rpc.token"
	AddrFile  = "rpc.addr"
	// ViewTokenFile holds the view token: it opens every GET endpoint and
	// nothing else, for a monitor or a read-only tool (GREEN-007).
	ViewTokenFile = "rpc.view-token"
)

// MaxBody bounds a request body, except an entry's submitted with the
// full token (RED-038, D144).
const MaxBody = 8 << 20

// MaxImportBody bounds a node policy import sent with the full token: any
// node policy a bundle the node reads can hold (nodepolicy.MaxBundleSize),
// even if encoding it again escapes every byte (six bytes each), and the
// options, so poo policy import never reads a file the node then refuses
// (RED-045, D150).
const MaxImportBody = 6*nodepolicy.MaxBundleSize + MaxBody

// Error codes.
const (
	CodeBadRequest     = "bad_request"
	CodeUnauthorized   = "unauthorized"
	CodeForbidden      = "forbidden"
	CodeNotFound       = "not_found"
	CodeConflict       = "conflict"
	CodeInvalid        = "invalid"
	CodeFull           = "full"
	CodeNothingPending = "nothing_pending"
	// CodeWaiting: a transaction on a mint whose part this node has not
	// seen released; not invalid, it waits for a release (D166).
	CodeWaiting  = "waiting"
	CodeInternal = "internal"
)

// Error is an error reply.
type Error struct {
	Message string `json:"error"`
	Code    string `json:"code"`
	Status  int    `json:"-"`
}

func (e *Error) Error() string { return e.Message }

// LoadOrCreateToken returns the token in dir, creating it if missing.
func LoadOrCreateToken(dir string) (string, error) {
	return loadOrCreateToken(filepath.Join(dir, TokenFile))
}

// LoadOrCreateViewToken returns the view token in dir, creating it if
// missing.
func LoadOrCreateViewToken(dir string) (string, error) {
	return loadOrCreateToken(filepath.Join(dir, ViewTokenFile))
}

func loadOrCreateToken(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err == nil {
		tok := strings.TrimSpace(string(b))
		if len(tok) == 64 {
			return tok, nil
		}
		return "", fmt.Errorf("rpc: %s is not a token; delete it to make a new one", path)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	tok := hex.EncodeToString(raw)
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return "", err
	}
	if _, err := f.WriteString(tok + "\n"); err != nil {
		f.Close()
		return "", err
	}
	return tok, f.Close()
}

// Handler serves the interface for one node.
type Handler struct {
	n      *node.Node
	host   *p2p.Host
	hub    *watch.Hub
	reload func()
	grind  *grind.Manager
	token  []byte
	view   []byte // the view token, or nil for none
	addr   string // the address in rpc.addr, which hello binds
	mux    *http.ServeMux
	// policyMu serialises the edits of node.json (follow, unfollow, the
	// grind schedule), each a read, a change and a write (RED-013). It is
	// the node's own (node.PolicyMu), which the p2p host takes too when it
	// keeps a release a peer passed on (D168).
	policyMu *sync.Mutex
}

// NewHandler returns the HTTP handler for n, accepting only token.
func NewHandler(n *node.Node, token string) *Handler {
	h := &Handler{n: n, token: []byte(token), mux: http.NewServeMux(), policyMu: &sync.Mutex{}}
	if n != nil {
		h.policyMu = n.PolicyMu()
	}
	h.mux.HandleFunc("GET /v1/status", h.status)
	h.mux.HandleFunc("GET /v1/series", h.series)
	h.mux.HandleFunc("POST /v1/tx", h.submitTx)
	h.mux.HandleFunc("POST /v1/entry", h.submitEntry)
	h.mux.HandleFunc("POST /v1/layer", h.submitLayer)
	h.mux.HandleFunc("POST /v1/build", h.build)
	h.mux.HandleFunc("POST /v1/work", h.work)
	h.mux.HandleFunc("GET /v1/payment", h.payment)
	h.mux.HandleFunc("GET /v1/holdings", h.holdings)
	h.mux.HandleFunc("GET /v1/peers", h.peers)
	h.mux.HandleFunc("GET /v1/tracking", h.tracking)
	h.mux.HandleFunc("GET /v1/chases", h.chases)
	h.mux.HandleFunc("POST /v1/peer", h.peer)
	h.mux.HandleFunc("POST /v1/peers/import", h.importPeers)
	h.mux.HandleFunc("POST /v1/policy/limits", h.setLimits)
	h.mux.HandleFunc("POST /v1/series/release", h.addRelease)
	h.mux.HandleFunc("POST /v1/peers/forget-imported", h.forgetImported)
	h.mux.HandleFunc("GET /v1/metrics", h.metrics)
	h.viewRoutes()
	h.grindRoutes()
	return h
}

// SetViewToken makes the interface also accept tok, for GET requests
// only. Call it before serving.
func (h *Handler) SetViewToken(tok string) { h.view = []byte(tok) }

// SetHost lets the interface report on the node's p2p host. Call it before
// serving.
func (h *Handler) SetHost(host *p2p.Host) { h.host = host }

// PeersReply is the reply of /v1/peers.
type PeersReply struct {
	NodeID string         `json:"node_id"`
	Listen string         `json:"listen,omitempty"`
	Peers  []p2p.PeerInfo `json:"peers"`
	// Book is every node in the peer book, with its state per policy. It
	// is left out of a counts-only reply (GET /v1/peers?counts=1).
	Book []peers.Record `json:"book"`
	// Counts is the book's size, set in a counts-only reply.
	Counts *BookCounts `json:"counts,omitempty"`
}

// BookCounts is a peer book's size without its records, for a view that
// refreshes often: a full book of 16,384 addresses is about 12.8 MB of
// JSON (GUI ORANGE-017).
type BookCounts struct {
	// Addresses is the number of records in the book.
	Addresses int `json:"addresses"`
	// Connected is the number of open connections.
	Connected int `json:"connected"`
	// States counts, for each book state, the addresses that hold it for
	// at least one mint policy; an address may count under several.
	States map[string]int `json:"states"`
}

// CountBook returns the counts of a book's records and connections.
func CountBook(book []peers.Record, conns int) BookCounts {
	c := BookCounts{Addresses: len(book), Connected: conns, States: map[string]int{}}
	for _, st := range peers.States {
		c.States[st.String()] = 0
	}
	for _, r := range book {
		seen := map[peers.State]bool{}
		for _, p := range r.Policies {
			if !seen[p.State] {
				seen[p.State] = true
				c.States[p.State.String()]++
			}
		}
	}
	return c
}

func (h *Handler) peers(w http.ResponseWriter, r *http.Request) {
	if !h.needHost(w) {
		return
	}
	conns := h.host.Peers()
	reply := PeersReply{NodeID: h.host.ID().String(), Listen: h.host.Addr(), Peers: conns, Book: []peers.Record{}}
	if r.URL.Query().Get("counts") == "1" {
		// Counted in place: a counts-only reply copies no record
		// (ORANGE-034).
		n, states := h.host.Book().Counts()
		c := BookCounts{Addresses: n, Connected: len(conns), States: map[string]int{}}
		for _, st := range peers.States {
			c.States[st.String()] = states[st]
		}
		reply.Counts = &c
	} else if book := h.host.Book().Records(); book != nil {
		reply.Book = book
	}
	writeJSON(w, reply)
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet && r.URL.Path == HelloPath {
		h.hello(w, r)
		return
	}
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	full := ok && subtle.ConstantTimeCompare([]byte(got), h.token) == 1
	view := ok && len(h.view) > 0 && subtle.ConstantTimeCompare([]byte(got), h.view) == 1
	switch {
	case full:
	case view && (r.Method == http.MethodGet || r.Method == http.MethodHead):
	case view:
		writeError(w, &Error{Message: "the view token only reads; this needs " + TokenFile, Code: CodeForbidden, Status: http.StatusForbidden})
		return
	default:
		writeError(w, &Error{Message: "missing or wrong token", Code: CodeUnauthorized, Status: http.StatusUnauthorized})
		return
	}
	switch {
	case full && r.Method == http.MethodPost && r.URL.Path == "/v1/entry":
		// An entry has no size bound: the node takes one of any size from
		// a peer, in parts (D117), so the full token may hand it one of
		// any size too, as poo submit and the GUI do (RED-038, D144).
	case full && r.Method == http.MethodPost && r.URL.Path == "/v1/policy/import":
		r.Body = http.MaxBytesReader(w, r.Body, MaxImportBody)
	case full && r.Method == http.MethodPost && r.URL.Path == "/v1/peers/import":
		r.Body = http.MaxBytesReader(w, r.Body, MaxPeerImportBody)
	default:
		r.Body = http.MaxBytesReader(w, r.Body, MaxBody)
	}
	h.mux.ServeHTTP(w, r)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, e *Error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(e.Status)
	json.NewEncoder(w).Encode(e)
}

func badRequest(format string, args ...any) *Error {
	return &Error{Message: fmt.Sprintf(format, args...), Code: CodeBadRequest, Status: http.StatusBadRequest}
}

// classify maps an engine, ledger or mempool error to a reply.
func classify(err error) *Error {
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	code, status := CodeInternal, http.StatusInternalServerError
	switch {
	case errors.Is(err, engine.ErrUnknownSeries), errors.Is(err, engine.ErrNotFollowed),
		errors.Is(err, ledger.ErrUnknownEntry), errors.Is(err, ledger.ErrUnknownParent), errors.Is(err, ledger.ErrNotHeld), errors.Is(err, ledger.ErrNoClaim):
		code, status = CodeNotFound, http.StatusNotFound
	case errors.Is(err, engine.ErrSeen), errors.Is(err, mempool.ErrKnown), errors.Is(err, mempool.ErrConflict),
		errors.Is(err, mempool.ErrInSelected), errors.Is(err, mempool.ErrDependsOnPending),
		errors.Is(err, engine.ErrAlreadyBuilt), errors.Is(err, ledger.ErrKnownEntry), errors.Is(err, ledger.ErrKnownLayer):
		code, status = CodeConflict, http.StatusConflict
	case errors.Is(err, mempool.ErrInvalid), errors.Is(err, ledger.ErrInvalidEntry), errors.Is(err, ledger.ErrWeakLayer),
		isTxRule(err):
		code, status = CodeInvalid, http.StatusUnprocessableEntity
	case errors.Is(err, mempool.ErrFull):
		code, status = CodeFull, http.StatusServiceUnavailable
	case errors.Is(err, tx.ErrWaiting):
		// Not invalid: the node has not seen the release of the mint's
		// part yet (D166).
		code, status = CodeWaiting, http.StatusConflict
	case errors.Is(err, engine.ErrNothingPending), errors.Is(err, engine.ErrNoStrongerLayer):
		code, status = CodeNothingPending, http.StatusConflict
	}
	return &Error{Message: err.Error(), Code: code, Status: status}
}

func isTxRule(err error) bool {
	for _, e := range []error{
		tx.ErrWrongSeries, tx.ErrSerialRange, tx.ErrNoOutputs, tx.ErrZeroAmount, tx.ErrRepeatedAddress,
		tx.ErrRepeatedInput, tx.ErrOverflow, tx.ErrClaimSupply, tx.ErrWitnessCount, tx.ErrWitness,
		tx.ErrWrongPart,
	} {
		if errors.Is(err, e) {
			return true
		}
	}
	return false
}

func decodeBody(r *http.Request, v any) *Error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return badRequest("body: %v", err)
	}
	if dec.More() {
		return badRequest("body: more than one JSON value")
	}
	return nil
}

func decodeHex(field, s string) ([]byte, *Error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return nil, badRequest("%s: not hex", field)
	}
	return b, nil
}

// PoolRef names a pool in a request.
type PoolRef struct {
	Series hash.Hash `json:"series"`
	Pool   uint64    `json:"pool"`
}

func (p PoolRef) key() engine.PoolKey { return engine.PoolKey{Series: p.Series, Index: p.Pool} }

// PoolStatus is one pool in a status reply.
type PoolStatus struct {
	Series   hash.Hash `json:"series"`
	Pool     uint64    `json:"pool"`
	Tip      hash.Hash `json:"tip"`
	Height   int       `json:"height"`
	Weight   string    `json:"weight"`
	Entries  int       `json:"entries"`
	Branches int       `json:"branches"`
	Pending  int       `json:"pending"`
	Waiting  int       `json:"waiting"`
	Stored   int       `json:"stored_records"`
}

// StatusReply is the reply to GET /v1/status.
type StatusReply struct {
	Pools  []PoolStatus     `json:"pools"`
	Replay node.ReplayStats `json:"replay"`
	// PolicyChanged says node.json now differs from the policy the node
	// runs, outside the grind section it applies at once: a change made
	// with -no-reload or by hand waits for poo reload (GREEN-037). Limits
	// applied in place count as applied (D168).
	PolicyChanged bool `json:"policy_changed,omitempty"`
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	stored := map[engine.PoolKey]int{}
	for _, s := range h.n.Stores() {
		stored[s.Key] = s.Records
	}
	rep := StatusReply{Pools: []PoolStatus{}, Replay: h.n.Replayed()}
	if p, err := nodepolicy.Load(h.policyPath()); err == nil {
		rep.PolicyChanged = !p.SameApplied(h.n.Running())
	}
	for _, k := range h.n.Engine().ListedPools() {
		st, err := h.n.Engine().Status(k)
		if err != nil {
			writeError(w, classify(err))
			return
		}
		rep.Pools = append(rep.Pools, PoolStatus{
			Series: k.Series, Pool: k.Index, Tip: st.Tip, Height: st.Height, Weight: st.Weight.String(),
			Entries: st.Entries, Branches: len(st.Branches), Pending: st.Pending, Waiting: st.Waiting, Stored: stored[k],
		})
	}
	writeJSON(w, rep)
}

// SeriesInfo is one series in a series reply.
type SeriesInfo struct {
	ID     hash.Hash     `json:"id"`
	Params series.Params `json:"params"`
	Pools  uint64        `json:"pools"`
	// Variable is set for a variable series: what this node knows released
	// (D166). A fixed series' reply is as before.
	Variable *VariableInfo `json:"variable,omitempty"`
}

// VariableInfo is what a node knows of a variable series' parts, worked out
// by poo so a front end holds no rule of its own (GUI-GREEN-050).
type VariableInfo struct {
	// Released is the newest part released at this node.
	Released uint64 `json:"released"`
	// NextPart is the next part the issuer can release, left out when the
	// largest number of parts is reached.
	NextPart uint64 `json:"next_part,omitempty"`
	// Mints is the serials that exist here: 0 to Mints − 1.
	Mints uint64 `json:"mints"`
	// Units is Mints × S in decimal, the supply so far, which may pass 64
	// bits.
	Units string `json:"units"`
	// Release is the newest release the node keeps, left out while only
	// part 1 is released. A client adds it to its own copy of the series
	// (Series.AddRelease checks it), so it knows the same parts.
	Release *series.Release `json:"release,omitempty"`
}

// VariableOf returns what is known of a variable series, nil for a fixed one.
func VariableOf(s *series.Series) *VariableInfo {
	if !s.IsVariable() {
		return nil
	}
	v := &VariableInfo{Released: s.Released(), Mints: s.Mints()}
	if m := s.MaxParts(); m == 0 || v.Released < m {
		if _, err := s.MintsThrough(v.Released + 1); err == nil {
			v.NextPart = v.Released + 1
		}
	}
	if r, ok := s.NewestRelease(); ok {
		v.Release = &r
	}
	u := new(big.Int).SetUint64(v.Mints)
	v.Units = u.Mul(u, new(big.Int).SetUint64(s.Supply())).String()
	return v
}

func (h *Handler) series(w http.ResponseWriter, r *http.Request) {
	out := []SeriesInfo{}
	for _, id := range h.n.Engine().FollowedSeries() {
		s, _ := h.n.Engine().Series(id)
		out = append(out, SeriesInfo{ID: s.ID(), Params: s.Params(), Pools: s.PoolCount(), Variable: VariableOf(s)})
	}
	writeJSON(w, out)
}

// TxRequest submits a transaction.
type TxRequest struct {
	Tx string `json:"tx"`
}

// TxReply reports a kept transaction.
type TxReply struct {
	Series  hash.Hash `json:"series"`
	Pool    uint64    `json:"pool"`
	TID     hash.Hash `json:"tid"`
	Forward bool      `json:"forward"`
}

func (h *Handler) submitTx(w http.ResponseWriter, r *http.Request) {
	var req TxRequest
	if e := decodeBody(r, &req); e != nil {
		writeError(w, e)
		return
	}
	b, e := decodeHex("tx", req.Tx)
	if e != nil {
		writeError(w, e)
		return
	}
	t, err := tx.Decode(b)
	if err != nil {
		writeError(w, badRequest("%v", err))
		return
	}
	res, err := h.n.HandleTx(t, engine.Pushed)
	if err != nil {
		writeError(w, classify(err))
		return
	}
	writeJSON(w, TxReply{Series: res.Key.Series, Pool: res.Key.Index, TID: res.TID, Forward: res.Forward})
}

// EntryRequest submits an entry.
type EntryRequest struct {
	PoolRef
	Entry string `json:"entry"`
}

// EntryReply reports a kept entry.
type EntryReply struct {
	Hash     hash.Hash `json:"hash"`
	Entry    string    `json:"entry,omitempty"`
	Switched bool      `json:"switched"`
	Pruned   int       `json:"pruned"`
	Dropped  int       `json:"dropped"`
	// Grind, for a build: how it ground.
	Grind *GrindStats `json:"grind,omitempty"`
	// Parent, for a build: the entry it was built on. Refused are the
	// transactions it left out (D161).
	Parent  hash.Hash   `json:"parent,omitzero"`
	Refused []RefusedTx `json:"refused,omitempty"`
}

func entryReply(res engine.EntryResult) EntryReply {
	return EntryReply{Hash: res.Hash, Switched: res.Switch != nil, Pruned: len(res.Pruned), Dropped: len(res.Dropped)}
}

// entryBody is EntryRequest as the handler reads it: the same JSON, with
// the hex decoded straight from the body into bytes, so a large entry is
// not also held as a string of twice its size (Black's Round 14 note,
// D145).
type entryBody struct {
	PoolRef
	Entry hexBytes `json:"entry"`
}

// hexBytes is a JSON string of hex, decoded as it is read.
type hexBytes []byte

func (b *hexBytes) UnmarshalJSON(data []byte) error {
	if len(data) < 2 || data[0] != '"' || data[len(data)-1] != '"' || bytes.IndexByte(data, '\\') >= 0 {
		// Not a plain string: decode it the usual way, so an escaped
		// string still reads and anything else gives the usual error.
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return err
		}
		data = []byte(`"` + s + `"`)
	}
	src := data[1 : len(data)-1]
	out := make([]byte, hex.DecodedLen(len(src)))
	if _, err := hex.Decode(out, src); err != nil {
		return errNotHex
	}
	*b = out
	return nil
}

var errNotHex = errors.New("not hex")

// entryHint is the most of an entry body set aside before any of it
// arrives, whatever size the request states. Past it the buffer doubles
// as bytes arrive, never past the stated size, so a false or hostile
// Content-Length sets aside at most 64 MiB, or twice what was really
// sent, while an honest large entry costs a copy or two on the way
// (ORANGE-025; Blue's and Black's notes on its hint, D147).
const entryHint = 64 << 20

// readEntryBody reads an entry request whole into one buffer and decodes
// it in place: json's Decoder would copy the body again into its own
// buffer, growing it step by step, so a large entry cost eight times its
// size in memory instead of three (ORANGE-025). It is decodeBody's JSON:
// the same options as encoding/json, unknown fields refused, and nothing
// after the value.
func readEntryBody(r *http.Request, v any) *Error {
	body, err := readSized(r.Body, r.ContentLength, entryHint)
	if err != nil {
		return badRequest("body: %v", err)
	}
	if err := jsonv2.Unmarshal(body, v, json.DefaultOptionsV1(), jsonv2.RejectUnknownMembers(true)); err != nil {
		return badRequest("body: %v", err)
	}
	return nil
}

// readSized reads all of rd. It starts with room for min(stated, hint)
// bytes (hint when stated is unknown) and doubles the room as it fills,
// up to stated when that is known and not yet reached, so what it sets
// aside is never more than twice what has arrived, nor more than the
// stated size once the room has grown to it. Past the hint it starts
// with the stated size halved until it fits, so the doublings end at
// exactly the stated size: starting at the hint itself, the last
// doubling of a body just over a power of two copied the whole body
// again for its last few bytes (ORANGE-026).
func readSized(rd io.Reader, stated, hint int64) ([]byte, error) {
	room := max(hint, 512) + 1 // +1: the read that sees EOF
	switch {
	case stated > hint:
		want := stated + 1
		for want > room {
			want = (want + 1) / 2
		}
		room = want
	case stated >= 0:
		room = max(stated, 512) + 1
	}
	buf := make([]byte, 0, room)
	for {
		if len(buf) == cap(buf) {
			grow := int64(cap(buf)) * 2
			if stated > int64(cap(buf)) {
				grow = min(grow, stated+1)
			}
			buf = append(make([]byte, 0, grow), buf...)
		}
		n, err := rd.Read(buf[len(buf):cap(buf)])
		buf = buf[:len(buf)+n]
		if err == io.EOF {
			return buf, nil
		}
		if err != nil {
			return nil, err
		}
	}
}

func (h *Handler) submitEntry(w http.ResponseWriter, r *http.Request) {
	var req entryBody
	if e := readEntryBody(r, &req); e != nil {
		if strings.Contains(e.Message, errNotHex.Error()) {
			e = badRequest("entry: not hex")
		}
		writeError(w, e)
		return
	}
	en, err := entry.Decode(req.Entry)
	if err != nil {
		writeError(w, badRequest("%v", err))
		return
	}
	res, err := h.n.HandleEntry(req.key(), en, engine.Pushed)
	if err != nil {
		writeError(w, classify(err))
		return
	}
	writeJSON(w, entryReply(res))
}

// LayerRequest submits a layer.
type LayerRequest struct {
	PoolRef
	Layer string `json:"layer"`
}

// LayerReply reports a kept layer.
type LayerReply struct {
	Hash     hash.Hash `json:"hash"`
	Layer    string    `json:"layer,omitempty"`
	Strength int       `json:"strength"`
	Switched bool      `json:"switched"`
	// Grind, for work: how it ground.
	Grind *GrindStats `json:"grind,omitempty"`
	// StoppedOnTime, for work: the request gave more than MaxWorkTries
	// tries and no for, and stopped at MaxGrindFor before trying them
	// all (RED-081).
	StoppedOnTime bool `json:"stopped_on_time,omitempty"`
}

func (h *Handler) submitLayer(w http.ResponseWriter, r *http.Request) {
	var req LayerRequest
	if e := decodeBody(r, &req); e != nil {
		writeError(w, e)
		return
	}
	b, e := decodeHex("layer", req.Layer)
	if e != nil {
		writeError(w, e)
		return
	}
	l, err := layer.Decode(b)
	if err != nil {
		writeError(w, badRequest("%v", err))
		return
	}
	res, err := h.n.HandleLayer(req.key(), l, engine.Pushed)
	if err != nil {
		writeError(w, classify(err))
		return
	}
	writeJSON(w, LayerReply{Hash: res.Hash, Strength: res.Hash.Strength(), Switched: res.Switch != nil})
}

// MaxBuildTries bounds the nonces one build request may try for an entry,
// and is what a request that names none gets.
const MaxBuildTries = 1 << 24

// BuildRequest asks the node to build an entry from its pending
// transactions.
type BuildRequest struct {
	PoolRef
	MaxTries uint64 `json:"max_tries,omitempty"`
	GrindOptions
	// Parent, when set, is where to build instead of the selected tip
	// (D161): an entry hash, "root" for the pool's first entry, or
	// "before-claim:N" for the parent of the selected entry that holds
	// mint N's claim, to build a rival claim on.
	Parent string `json:"parent,omitempty"`
	// Txs, when set, are the transactions to build from instead of the
	// pending ones, each hex-encoded (at most MaxBuildTxs).
	Txs []string `json:"txs,omitempty"`
}

// MaxBuildTxs bounds the transactions one build request may name.
const MaxBuildTxs = 1024

// RefusedTx is a transaction a build left out, and why.
type RefusedTx struct {
	TID   hash.Hash `json:"tid"`
	Error string    `json:"error"`
}

func (h *Handler) build(w http.ResponseWriter, r *http.Request) {
	var req BuildRequest
	if e := decodeBody(r, &req); e != nil {
		writeError(w, e)
		return
	}
	// Bounded like work, and given up when the client goes away
	// (RED-006, decision D99).
	switch {
	case req.MaxTries > MaxBuildTries:
		writeError(w, badRequest("max_tries must be at most %d", MaxBuildTries))
		return
	case req.MaxTries == 0 && req.For == "":
		req.MaxTries = MaxBuildTries
	}
	p, d, e := h.plan(req.key(), req.GrindOptions)
	if e != nil {
		writeError(w, e)
		return
	}
	var tried atomic.Uint64
	ctx, cancel, g := grindCtx(r, p, d, &tried)
	defer cancel()
	t0 := time.Now()
	on, e := h.buildOn(req)
	if e != nil {
		writeError(w, e)
		return
	}
	en, res, refused, err := h.n.BuildOn(ctx, req.key(), req.MaxTries, g, on)
	if err != nil {
		if len(refused) > 0 && errors.Is(err, engine.ErrNothingPending) {
			err = fmt.Errorf("%w: every transaction was left out; %s: %v", err, refused[0].TID.Short(), refused[0].Err)
		}
		writeError(w, classify(err))
		return
	}
	rep := entryReply(res)
	rep.Parent = en.Parent
	for _, r := range refused {
		rep.Refused = append(rep.Refused, RefusedTx{TID: r.TID, Error: r.Err.Error()})
	}
	rep.Entry = hex.EncodeToString(en.Encode())
	rep.Grind = stats(p, tried.Load(), t0)
	writeJSON(w, rep)
}

// MaxWorkTries bounds the nonces one work request may try, so a request
// cannot hold the node for long. A series whose minimum layer strength
// needs more allows more: see WorkTriesFor.
const MaxWorkTries = 1 << 24

// workTimeBound is the time bound on a work request of more than
// MaxWorkTries tries with no for: MaxGrindFor, a variable only for tests.
var workTimeBound = MaxGrindFor

// maxWorkShift is the largest power of two WorkTriesFor allows (2^40).
const maxWorkShift = 40

// WorkTriesFor is the most nonces one work request may try on series s:
// MaxWorkTries, or 2^(floor+4) for a series whose minimum layer strength
// (the floor) is high, so a full-length request finds a layer at the
// floor with near certainty (1 − e^−16), up to 2^40 (D168).
func WorkTriesFor(s *series.Series) uint64 {
	n := uint64(MaxWorkTries)
	if s == nil {
		return n
	}
	if sh := min(s.MinLayerStrength()+4, maxWorkShift); sh > 24 {
		n = 1 << uint(sh)
	}
	return n
}

// DefaultWorkTries is the nonces poo work tries when neither -tries nor
// -for is given: 65,536, or 2^(floor+2) for a series whose floor needs
// more (about a 98% chance of a layer at the floor), within WorkTriesFor.
func DefaultWorkTries(s *series.Series) uint64 {
	n := uint64(1 << 16)
	if s == nil {
		return n
	}
	if sh := s.MinLayerStrength() + 2; sh > 16 {
		n = 1 << uint(min(sh, maxWorkShift))
	}
	return min(n, WorkTriesFor(s))
}

// WorkRequest asks the node to add a layer to an entry.
type WorkRequest struct {
	PoolRef
	Entry hash.Hash `json:"entry"`
	Start uint64    `json:"start,omitempty"`
	// Tries is how many nonces to try, 1 to MaxWorkTries; 0 with for
	// tries as many as fit in that time.
	Tries uint64 `json:"tries,omitempty"`
	GrindOptions
}

func (h *Handler) work(w http.ResponseWriter, r *http.Request) {
	var req WorkRequest
	if e := decodeBody(r, &req); e != nil {
		writeError(w, e)
		return
	}
	s, _ := h.n.Engine().Series(req.Series)
	if most := WorkTriesFor(s); req.Tries > most || (req.Tries == 0 && req.For == "") {
		writeError(w, badRequest("tries must be 1 to %d, or 0 with for", most))
		return
	}
	p, d, e := h.plan(req.key(), req.GrindOptions)
	if e != nil {
		writeError(w, e)
		return
	}
	// More tries than MaxWorkTries with no for are bounded in time too,
	// as for is: a series with a high floor allows up to 2^40 tries,
	// which is hours (RED-081, D169).
	timed := d == 0 && req.Tries > MaxWorkTries
	if timed {
		d = workTimeBound
	}
	var tried atomic.Uint64
	ctx, cancel, g := grindCtx(r, p, d, &tried)
	defer cancel()
	t0 := time.Now()
	l, res, err := h.n.WorkWith(ctx, req.key(), req.Entry, req.Start, req.Tries, g)
	stopped := timed && errors.Is(ctx.Err(), context.DeadlineExceeded)
	if err != nil {
		e := *classify(err)
		if stopped {
			e.Message += fmt.Sprintf(" (stopped after %s, the bound on a request of more than %d tries with no for, at %d tries; use a grind job for longer)", workTimeBound, MaxWorkTries, tried.Load())
		}
		writeError(w, &e)
		return
	}
	writeJSON(w, LayerReply{Hash: res.Hash, Layer: hex.EncodeToString(l.Encode()), Strength: res.Hash.Strength(), Switched: res.Switch != nil, Grind: stats(p, tried.Load(), t0), StoppedOnTime: stopped})
}

func queryHash(r *http.Request, name string) (hash.Hash, *Error) {
	h, err := hash.Parse(r.URL.Query().Get(name))
	if err != nil {
		return hash.Hash{}, badRequest("%s: %v", name, err)
	}
	return h, nil
}

func queryUint(r *http.Request, name string) (uint64, *Error) {
	v, err := strconv.ParseUint(r.URL.Query().Get(name), 10, 64)
	if err != nil {
		return 0, badRequest("%s: not a number", name)
	}
	return v, nil
}

// mintRef reads series and serial from the query and returns the series.
func (h *Handler) mintRef(r *http.Request) (*series.Series, uint64, *Error) {
	id, e := queryHash(r, "series")
	if e != nil {
		return nil, 0, e
	}
	serial, e := queryUint(r, "serial")
	if e != nil {
		return nil, 0, e
	}
	s, ok := h.n.Engine().Series(id)
	if !ok {
		return nil, 0, classify(engine.ErrUnknownSeries)
	}
	if err := tx.CheckMint(s, s.Mint(serial)); errors.Is(err, tx.ErrWaiting) {
		return nil, 0, classify(err)
	} else if err != nil {
		return nil, 0, badRequest("serial out of range")
	}
	return s, serial, nil
}

// PaymentReply says where a transaction stands.
type PaymentReply struct {
	Pending  bool       `json:"pending"`
	Selected bool       `json:"selected"`
	Entry    *hash.Hash `json:"entry,omitempty"`
	// From is the weight a challenger must beat to reverse the payment.
	From string `json:"from"`
	// Tx is the transaction's byte form in hex, when the node holds it, so
	// a receiver can check what it pays.
	Tx string `json:"tx,omitempty"`
	// Rivals are known inactive branches spending one of the same
	// portions.
	Rivals []RivalInfo `json:"rivals,omitempty"`
}

// RivalInfo is one known rival of a payment.
type RivalInfo struct {
	Tip      hash.Hash `json:"tip"`
	Fork     hash.Hash `json:"fork"`
	Entry    hash.Hash `json:"entry"`
	Conflict hash.Hash `json:"conflict"`
	// After is the rival's weight after its fork: the payment is ahead
	// while From is greater.
	After string `json:"after"`
}

func (h *Handler) payment(w http.ResponseWriter, r *http.Request) {
	s, serial, e := h.mintRef(r)
	if e != nil {
		writeError(w, e)
		return
	}
	tid, e := queryHash(r, "tid")
	if e != nil {
		writeError(w, e)
		return
	}
	p, err := h.n.Engine().Payment(engine.PoolKey{Series: s.ID(), Index: s.PoolOf(serial)}, tid)
	if err != nil {
		writeError(w, classify(err))
		return
	}
	rep := PaymentReply{Pending: p.Pending, Selected: p.Selected, From: p.From.String()}
	if p.Selected {
		rep.Entry = &p.Entry
	}
	if p.Tx != nil {
		rep.Tx = hex.EncodeToString(p.Tx.Encode())
	}
	for _, rv := range p.Rivals {
		rep.Rivals = append(rep.Rivals, RivalInfo{Tip: rv.Tip, Fork: rv.Fork, Entry: rv.Entry, Conflict: rv.Conflict, After: rv.After.String()})
	}
	writeJSON(w, rep)
}

// Holding is one unspent portion.
type Holding struct {
	TID     hash.Hash `json:"tid"`
	Index   uint32    `json:"index"`
	Address hash.Hash `json:"address"`
	Amount  uint64    `json:"amount"`
}

func (h *Handler) holdings(w http.ResponseWriter, r *http.Request) {
	s, serial, e := h.mintRef(r)
	if e != nil {
		writeError(w, e)
		return
	}
	hs, err := h.n.Engine().Holdings(s.ID(), serial)
	if err != nil {
		writeError(w, classify(err))
		return
	}
	out := make([]Holding, len(hs))
	for i, x := range hs {
		out[i] = Holding{TID: x.Input.TID, Index: x.Input.Index, Address: x.Portion.Address, Amount: x.Portion.Amount}
	}
	writeJSON(w, out)
}

// buildOn reads a build request's parent and transactions (D161).
func (h *Handler) buildOn(req BuildRequest) (engine.BuildOn, *Error) {
	var on engine.BuildOn
	eng := h.n.Engine()
	switch par := req.Parent; {
	case par == "":
	case par == "root":
		id, err := eng.PoolID(req.key())
		if err != nil {
			return on, classify(err)
		}
		on.Parent = &id
	case strings.HasPrefix(par, "before-claim:"):
		serial, err := strconv.ParseUint(strings.TrimPrefix(par, "before-claim:"), 10, 64)
		if err != nil {
			return on, badRequest("parent %q: before-claim:N takes a mint serial", par)
		}
		ph, err := eng.ClaimParent(req.key(), serial)
		if err != nil {
			return on, classify(err)
		}
		on.Parent = &ph
	default:
		ph, err := hash.Parse(par)
		if err != nil {
			return on, badRequest("parent %q: an entry hash, root or before-claim:N", par)
		}
		on.Parent = &ph
	}
	if len(req.Txs) > MaxBuildTxs {
		return on, badRequest("at most %d transactions", MaxBuildTxs)
	}
	for i, x := range req.Txs {
		b, err := hex.DecodeString(x)
		if err != nil {
			return on, badRequest("transaction %d: not hex", i)
		}
		t, err := tx.Decode(b)
		if err != nil {
			return on, badRequest("transaction %d: %v", i, err)
		}
		on.Txs = append(on.Txs, t)
	}
	return on, nil
}
