package rpc

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/engine"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/filter"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/watch"
)

// PingInterval is how often the watch stream writes a ping line while no
// event arrives, so a client can tell a quiet node from a lost one.
var PingInterval = 15 * time.Second

// SetHub lets the interface stream events from hub. Without one,
// /v1/watch replies not_found.
func (h *Handler) SetHub(hub *watch.Hub) { h.hub = hub }

// SetReload gives the interface a way to make the node apply its node
// policy again (see pkg/daemon). Without one, /v1/reload and the reload of
// /v1/follow reply not_found.
func (h *Handler) SetReload(f func()) { h.reload = f }

func (h *Handler) viewRoutes() {
	h.mux.HandleFunc("GET /v1/pools", h.pools)
	h.mux.HandleFunc("GET /v1/ledger", h.ledgerPage)
	h.mux.HandleFunc("GET /v1/ledger/export", h.ledgerExport)
	h.mux.HandleFunc("POST /v1/ledger/import", h.ledgerImport)
	h.mux.HandleFunc("GET /v1/pending", h.pending)
	h.mux.HandleFunc("GET /v1/tx", h.txView)
	h.mux.HandleFunc("GET /v1/address", h.address)
	h.mux.HandleFunc("GET /v1/unspent", h.unspent)
	h.mux.HandleFunc("GET /v1/watch", h.watch)
	h.mux.HandleFunc("GET /v1/follow", h.follows)
	h.mux.HandleFunc("POST /v1/follow", h.changeFollow)
	h.mux.HandleFunc("POST /v1/reload", h.reloadNow)
	h.mux.HandleFunc("POST /v1/policy/import", h.importPolicy)
}

func readFilter(r *http.Request) (filter.Filter, *Error) {
	f, err := filter.FromValues(r.URL.Query())
	if err != nil {
		return f, badRequest("%v", err)
	}
	return f, nil
}

// seriesParam reads the series query parameter, or takes the only
// followed series when it is left out.
func (h *Handler) seriesParam(r *http.Request) (hash.Hash, *Error) {
	if r.URL.Query().Get("series") != "" {
		return queryHash(r, "series")
	}
	ids := h.n.Engine().FollowedSeries()
	if len(ids) != 1 {
		return hash.Hash{}, badRequest("series: needed when the node does not follow exactly one series")
	}
	return ids[0], nil
}

func optInt(r *http.Request, name string) (int, *Error) {
	v := r.URL.Query().Get(name)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return 0, badRequest("%s: not a number", name)
	}
	return n, nil
}

func (h *Handler) pools(w http.ResponseWriter, r *http.Request) {
	f, e := readFilter(r)
	if e != nil {
		writeError(w, e)
		return
	}
	writeJSON(w, h.poolList(f))
}

// The ledger view takes series (optional with one followed series), pool,
// below (a height; 0 or absent for the tip), limit, and the filter.
func (h *Handler) ledgerPage(w http.ResponseWriter, r *http.Request) {
	id, e := h.seriesParam(r)
	if e != nil {
		writeError(w, e)
		return
	}
	pool, e := queryUint(r, "pool")
	if e != nil {
		writeError(w, e)
		return
	}
	below, e := optInt(r, "below")
	if e != nil {
		writeError(w, e)
		return
	}
	limit, e := optInt(r, "limit")
	if e != nil {
		writeError(w, e)
		return
	}
	f, e := readFilter(r)
	if e != nil {
		writeError(w, e)
		return
	}
	out, err := watch.LedgerPage(h.n.Engine(), engine.PoolKey{Series: id, Index: pool}, f, below, limit)
	if err != nil {
		writeError(w, classify(err))
		return
	}
	writeJSON(w, out)
}

func (h *Handler) pending(w http.ResponseWriter, r *http.Request) {
	f, e := readFilter(r)
	if e != nil {
		writeError(w, e)
		return
	}
	writeJSON(w, watch.Pending(h.n.Engine(), f))
}

func (h *Handler) unspent(w http.ResponseWriter, r *http.Request) {
	f, e := readFilter(r)
	if e != nil {
		writeError(w, e)
		return
	}
	writeJSON(w, watch.Holdings(h.n.Engine(), f))
}

// The transaction view takes series (optional with one followed series),
// id (the tid) and, to look in one pool only, serial.
func (h *Handler) txView(w http.ResponseWriter, r *http.Request) {
	id, e := h.seriesParam(r)
	if e != nil {
		writeError(w, e)
		return
	}
	tid, e := queryHash(r, "id")
	if e != nil {
		writeError(w, e)
		return
	}
	var serial *uint64
	if r.URL.Query().Get("serial") != "" {
		n, e := queryUint(r, "serial")
		if e != nil {
			writeError(w, e)
			return
		}
		serial = &n
	}
	st, err := watch.TxOfSeries(h.n.Engine(), id, tid, serial)
	if errors.Is(err, watch.ErrNotFound) {
		writeError(w, &Error{Message: err.Error(), Code: CodeNotFound, Status: http.StatusNotFound})
		return
	}
	if err != nil {
		writeError(w, classify(err))
		return
	}
	writeJSON(w, st)
}

// The address view takes series (optional with one followed series), id
// (the address), limit, and the filter's pool and serial fields.
func (h *Handler) address(w http.ResponseWriter, r *http.Request) {
	id, e := h.seriesParam(r)
	if e != nil {
		writeError(w, e)
		return
	}
	a, e := queryHash(r, "id")
	if e != nil {
		writeError(w, e)
		return
	}
	limit, e := optInt(r, "limit")
	if e != nil {
		writeError(w, e)
		return
	}
	below, e := optInt(r, "below")
	if e != nil {
		writeError(w, e)
		return
	}
	f, e := readFilter(r)
	if e != nil {
		writeError(w, e)
		return
	}
	// Bounded per request and given up when the client goes away
	// (RED-008, decision D103).
	out, err := watch.AddressPage(r.Context(), h.n.Engine(), id, a, f, below, limit)
	if err != nil {
		writeError(w, classify(err))
		return
	}
	writeJSON(w, out)
}

// watch streams events as newline-delimited JSON until the client goes
// away or the node stops. A {"kind":"missed","missed":N} line says N
// events were dropped because the client read too slowly; a
// {"kind":"ping"} line is written when nothing else has been for
// PingInterval.
func (h *Handler) watch(w http.ResponseWriter, r *http.Request) {
	if h.hub == nil {
		writeError(w, &Error{Message: "this node does not stream events", Code: CodeNotFound, Status: http.StatusNotFound})
		return
	}
	f, e := readFilter(r)
	if e != nil {
		writeError(w, e)
		return
	}
	sub := h.hub.Subscribe(f, 0)
	defer sub.Close()
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	enc := json.NewEncoder(w)
	send := func(v any) bool {
		if enc.Encode(v) != nil {
			return false
		}
		return rc.Flush() == nil
	}
	if !send(watch.Event{Kind: "hello", Time: time.Now().UTC()}) {
		return
	}
	ping := time.NewTicker(PingInterval)
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ping.C:
			if !send(watch.Event{Kind: "ping", Time: time.Now().UTC()}) {
				return
			}
		case ev, ok := <-sub.C:
			if !ok {
				send(watch.Event{Kind: "end", Time: time.Now().UTC()})
				return
			}
			if m := sub.Missed(); m > 0 && !send(watch.Event{Kind: "missed", Time: time.Now().UTC(), Missed: m}) {
				return
			}
			if !send(ev) {
				return
			}
		}
	}
}

// FollowReply lists what the node policy follows and the pools the
// running node follows.
type FollowReply struct {
	Follow []nodepolicy.Follow `json:"follow"`
	Pools  []watch.Pool        `json:"pools"`
	// Reloading: the node is applying a changed policy and its interface
	// is about to restart.
	Reloading bool `json:"reloading,omitempty"`
}

// FollowRequest adds to or removes from the node policy's follow list
// (nodepolicy.AddFollow, RemoveFollow) or, with Replace, puts Pools and
// Mints in place of the series' rule (nodepolicy.ReplaceFollow) in one
// change, and, unless NoReload, applies it.
type FollowRequest struct {
	Remove   bool       `json:"remove,omitempty"`
	Replace  bool       `json:"replace,omitempty"`
	Issuer   *hash.Hash `json:"issuer,omitempty"`
	Series   *hash.Hash `json:"series,omitempty"`
	Pools    []uint64   `json:"pools,omitempty"`
	Mints    []uint64   `json:"mints,omitempty"`
	NoReload bool       `json:"no_reload,omitempty"`
}

func (h *Handler) policyPath() string { return filepath.Join(h.n.Dir(), nodepolicy.FileName) }

func (h *Handler) follows(w http.ResponseWriter, r *http.Request) {
	p, err := nodepolicy.Load(h.policyPath())
	if err != nil {
		writeError(w, classify(err))
		return
	}
	writeJSON(w, h.followReply(p, false))
}

// poolList is watch.Pools with each pool's store record count.
func (h *Handler) poolList(f filter.Filter) []watch.Pool {
	out := watch.Pools(h.n.Engine(), f)
	if out == nil {
		return []watch.Pool{}
	}
	stored := map[engine.PoolKey]int{}
	for _, s := range h.n.Stores() {
		stored[s.Key] = s.Records
	}
	for i := range out {
		out[i].StoredRecords = stored[engine.PoolKey{Series: out[i].Series, Index: out[i].Pool}]
	}
	return out
}

func (h *Handler) followReply(p nodepolicy.Policy, reloading bool) FollowReply {
	out := FollowReply{Follow: p.Follow, Pools: h.poolList(filter.Filter{}), Reloading: reloading}
	if out.Follow == nil {
		out.Follow = []nodepolicy.Follow{}
	}
	if out.Pools == nil {
		out.Pools = []watch.Pool{}
	}
	return out
}

func (h *Handler) changeFollow(w http.ResponseWriter, r *http.Request) {
	var req FollowRequest
	if e := decodeBody(r, &req); e != nil {
		writeError(w, e)
		return
	}
	if !req.NoReload && h.reload == nil {
		writeError(w, &Error{Message: "this node cannot reload; set no_reload and restart it", Code: CodeNotFound, Status: http.StatusNotFound})
		return
	}
	path := h.policyPath()
	h.policyMu.Lock()
	p, err := nodepolicy.Load(path)
	if err != nil {
		h.policyMu.Unlock()
		writeError(w, classify(err))
		return
	}
	f := nodepolicy.Follow{Issuer: req.Issuer, Series: req.Series, Pools: req.Pools, Mints: req.Mints}
	switch {
	case req.Remove && req.Replace:
		err = fmt.Errorf("%w: remove or replace, not both", nodepolicy.ErrInvalid)
	case req.Replace:
		err = p.ReplaceFollow(f)
	case req.Remove:
		err = p.RemoveFollow(f)
	default:
		err = p.AddFollow(f)
	}
	if err == nil {
		err = p.Validate()
	}
	if err == nil {
		err = nodepolicy.Save(path, p)
		if err != nil {
			h.policyMu.Unlock()
			writeError(w, classify(err))
			return
		}
	}
	h.policyMu.Unlock()
	if err != nil {
		writeError(w, &Error{Message: err.Error(), Code: CodeInvalid, Status: http.StatusUnprocessableEntity})
		return
	}
	writeJSON(w, h.followReply(p, !req.NoReload))
	if !req.NoReload {
		h.reload()
	}
}

func (h *Handler) reloadNow(w http.ResponseWriter, r *http.Request) {
	if h.reload == nil {
		writeError(w, &Error{Message: "this node cannot reload; restart it", Code: CodeNotFound, Status: http.StatusNotFound})
		return
	}
	if _, err := nodepolicy.Load(h.policyPath()); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, classify(err))
			return
		}
		// Reloading would stop the node for good (GREEN-011).
		writeError(w, &Error{Message: "not reloading, the node keeps its current policy: " + err.Error(), Code: CodeInvalid, Status: http.StatusUnprocessableEntity})
		return
	}
	writeJSON(w, map[string]bool{"reloading": true})
	h.reload()
}
