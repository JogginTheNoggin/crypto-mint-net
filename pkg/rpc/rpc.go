// Package rpc is the node's local control interface: JSON over HTTP, on a
// loopback address by default, for the node's owner and their tools (a
// wallet, a script, the poo command). It is not the peer-to-peer protocol;
// that is package p2p, and this interface only reports on it.
//
// Every request must carry the node's token, "Authorization: Bearer
// <token>". The token is 32 random bytes in hex, created on first start in
// the data directory as rpc.token, readable only by the node's user. The
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
//	POST /build    {series, pool, max_tries}
//	POST /work     {series, pool, entry, start, tries}
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
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/entry"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/mempool"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
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
)

// MaxBody bounds a request body.
const MaxBody = 8 << 20

// Error codes.
const (
	CodeBadRequest     = "bad_request"
	CodeUnauthorized   = "unauthorized"
	CodeNotFound       = "not_found"
	CodeConflict       = "conflict"
	CodeInvalid        = "invalid"
	CodeFull           = "full"
	CodeNothingPending = "nothing_pending"
	CodeInternal       = "internal"
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
	path := filepath.Join(dir, TokenFile)
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
	token  []byte
	mux    *http.ServeMux
}

// NewHandler returns the HTTP handler for n, accepting only token.
func NewHandler(n *node.Node, token string) *Handler {
	h := &Handler{n: n, token: []byte(token), mux: http.NewServeMux()}
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
	h.mux.HandleFunc("GET /v1/metrics", h.metrics)
	h.viewRoutes()
	return h
}

// SetHost lets the interface report on the node's p2p host. Call it before
// serving.
func (h *Handler) SetHost(host *p2p.Host) { h.host = host }

// PeersReply is the reply of /v1/peers.
type PeersReply struct {
	NodeID string         `json:"node_id"`
	Listen string         `json:"listen,omitempty"`
	Peers  []p2p.PeerInfo `json:"peers"`
	// Book is every node in the peer book, with its state per policy.
	Book []peers.Record `json:"book"`
}

func (h *Handler) peers(w http.ResponseWriter, r *http.Request) {
	if !h.needHost(w) {
		return
	}
	book := h.host.Book().Records()
	if book == nil {
		book = []peers.Record{}
	}
	writeJSON(w, PeersReply{NodeID: h.host.ID().String(), Listen: h.host.Addr(), Peers: h.host.Peers(), Book: book})
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	got, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || subtle.ConstantTimeCompare([]byte(got), h.token) != 1 {
		writeError(w, &Error{Message: "missing or wrong token", Code: CodeUnauthorized, Status: http.StatusUnauthorized})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxBody)
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
		errors.Is(err, ledger.ErrUnknownEntry), errors.Is(err, ledger.ErrUnknownParent):
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
	case errors.Is(err, engine.ErrNothingPending), errors.Is(err, engine.ErrNoStrongerLayer):
		code, status = CodeNothingPending, http.StatusConflict
	}
	return &Error{Message: err.Error(), Code: code, Status: status}
}

func isTxRule(err error) bool {
	for _, e := range []error{
		tx.ErrWrongSeries, tx.ErrSerialRange, tx.ErrNoOutputs, tx.ErrZeroAmount, tx.ErrRepeatedAddress,
		tx.ErrRepeatedInput, tx.ErrOverflow, tx.ErrClaimSupply, tx.ErrWitnessCount, tx.ErrWitness,
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
}

func (h *Handler) status(w http.ResponseWriter, r *http.Request) {
	stored := map[engine.PoolKey]int{}
	for _, s := range h.n.Stores() {
		stored[s.Key] = s.Records
	}
	rep := StatusReply{Pools: []PoolStatus{}, Replay: h.n.Replayed()}
	for _, k := range h.n.Engine().FollowedPools() {
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
}

func (h *Handler) series(w http.ResponseWriter, r *http.Request) {
	out := []SeriesInfo{}
	seen := map[hash.Hash]bool{}
	for _, k := range h.n.Engine().FollowedPools() {
		if seen[k.Series] {
			continue
		}
		seen[k.Series] = true
		s, _ := h.n.Engine().Series(k.Series)
		out = append(out, SeriesInfo{ID: s.ID(), Params: s.Params(), Pools: s.PoolCount()})
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
}

func entryReply(res engine.EntryResult) EntryReply {
	return EntryReply{Hash: res.Hash, Switched: res.Switch != nil, Pruned: len(res.Pruned), Dropped: len(res.Dropped)}
}

func (h *Handler) submitEntry(w http.ResponseWriter, r *http.Request) {
	var req EntryRequest
	if e := decodeBody(r, &req); e != nil {
		writeError(w, e)
		return
	}
	b, e := decodeHex("entry", req.Entry)
	if e != nil {
		writeError(w, e)
		return
	}
	en, err := entry.Decode(b)
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

// BuildRequest asks the node to build an entry from its pending
// transactions.
type BuildRequest struct {
	PoolRef
	MaxTries uint64 `json:"max_tries,omitempty"`
}

func (h *Handler) build(w http.ResponseWriter, r *http.Request) {
	var req BuildRequest
	if e := decodeBody(r, &req); e != nil {
		writeError(w, e)
		return
	}
	en, res, err := h.n.Build(req.key(), req.MaxTries)
	if err != nil {
		writeError(w, classify(err))
		return
	}
	rep := entryReply(res)
	rep.Entry = hex.EncodeToString(en.Encode())
	writeJSON(w, rep)
}

// MaxWorkTries bounds the nonces one work request may try, so a request
// cannot hold the node for long.
const MaxWorkTries = 1 << 24

// WorkRequest asks the node to add a layer to an entry.
type WorkRequest struct {
	PoolRef
	Entry hash.Hash `json:"entry"`
	Start uint64    `json:"start,omitempty"`
	Tries uint64    `json:"tries"`
}

func (h *Handler) work(w http.ResponseWriter, r *http.Request) {
	var req WorkRequest
	if e := decodeBody(r, &req); e != nil {
		writeError(w, e)
		return
	}
	if req.Tries == 0 || req.Tries > MaxWorkTries {
		writeError(w, badRequest("tries must be 1 to %d", MaxWorkTries))
		return
	}
	l, res, err := h.n.Work(req.key(), req.Entry, req.Start, req.Tries)
	if err != nil {
		writeError(w, classify(err))
		return
	}
	writeJSON(w, LayerReply{Hash: res.Hash, Layer: hex.EncodeToString(l.Encode()), Strength: res.Hash.Strength(), Switched: res.Switch != nil})
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
	if serial >= s.Mints() {
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
