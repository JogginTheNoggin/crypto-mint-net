package rpc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"runtime"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/bounty"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/layer"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
)

// The bounty market's local calls (D196): post, cancel and pay for this
// node's offers; ask peers for theirs, and work one.

// BountyReply is the market as this node sees it.
type BountyReply struct {
	Post bool `json:"post"`
	Take bool `json:"take"`
	// Mine are the offers this node posted, with their leases and claims.
	Mine []bounty.Mine `json:"mine"`
	// Held are the offers peers sent, with what this node did with them.
	Held []bounty.Held `json:"held"`
	// Payers is, by peer, what each payer accepted and paid of this
	// node's claims.
	Payers map[string]bounty.PayerRecord `json:"payers,omitempty"`
}

// BountyPostRequest posts an offer.
type BountyPostRequest struct {
	PoolRef
	Entry       hash.Hash `json:"entry"`
	Strength    uint8     `json:"strength"`
	Amount      uint64    `json:"amount"`
	Count       uint16    `json:"count"`
	OpenSeconds uint32    `json:"open_seconds,omitempty"`
	// FindPool offers on the followed pool of the series that holds
	// Entry, whatever Pool says (GREEN-061).
	FindPool bool `json:"find_pool,omitempty"`
}

// BountyPostReply is the posted offer and how many peers it went to.
type BountyPostReply struct {
	Offer bounty.Mine `json:"offer"`
	Sent  int         `json:"sent"`
}

// BountyIDRequest names one offer.
type BountyIDRequest struct {
	ID hash.Hash `json:"id"`
}

// BountyAskRequest asks peers for their open offers.
type BountyAskRequest struct {
	Series hash.Hash `json:"series,omitempty"`
}

// BountyAskReply says how many peers were asked.
type BountyAskReply struct {
	Asked int `json:"asked"`
}

// BountyWorkRequest takes a lease on a peer's offer, grinds the layer
// and claims it, paid to PayTo.
type BountyWorkRequest struct {
	ID    hash.Hash `json:"id"`
	PayTo hash.Hash `json:"pay_to"`
	// MaxSeconds bounds the grinding (default and most: MaxBountyWork);
	// the lease may end it sooner.
	MaxSeconds int `json:"max_seconds,omitempty"`
}

// MaxBountyWork bounds one work call, under the client's timeout.
const MaxBountyWork = 4 * time.Minute

// BountyWorkReply is what came of a work call.
type BountyWorkReply struct {
	Rate     uint64       `json:"rate"`
	Grant    bounty.Grant `json:"grant"`
	Found    bool         `json:"found"`
	Nonce    uint64       `json:"nonce,omitempty"`
	Strength int          `json:"strength,omitempty"`
	Tried    uint64       `json:"tried"`
	Outcome  string       `json:"outcome,omitempty"`
	Ack      *bounty.Ack  `json:"ack,omitempty"`
}

// BountyPaidRequest records the payment of an accepted claim.
type BountyPaidRequest struct {
	ID    hash.Hash   `json:"id"`
	Nonce uint64      `json:"nonce"`
	TIDs  []hash.Hash `json:"tids"`
}

// BountyPaidReply says whether the grinder was told.
type BountyPaidReply struct {
	Told bool `json:"told"`
}

func (h *Handler) routeBounty() {
	h.mux.HandleFunc("GET /v1/bounty", h.bountyList)
	h.mux.HandleFunc("POST /v1/bounty/post", h.bountyPost)
	h.mux.HandleFunc("POST /v1/bounty/cancel", h.bountyCancel)
	h.mux.HandleFunc("POST /v1/bounty/ask", h.bountyAsk)
	h.mux.HandleFunc("POST /v1/bounty/work", h.bountyWork)
	h.mux.HandleFunc("POST /v1/bounty/paid", h.bountyPaid)
}

func bountyErr(err error) *Error {
	if errors.Is(err, p2p.ErrNoBounty) {
		return &Error{Message: err.Error(), Code: CodeNotFound, Status: http.StatusNotFound}
	}
	return &Error{Message: err.Error(), Code: CodeConflict, Status: http.StatusConflict}
}

func (h *Handler) bountyBook(w http.ResponseWriter) *bounty.Book {
	if !h.needHost(w) {
		return nil
	}
	b := h.host.Bounty()
	if b == nil {
		writeError(w, bountyErr(p2p.ErrNoBounty))
	}
	return b
}

func (h *Handler) bountyList(w http.ResponseWriter, r *http.Request) {
	b := h.bountyBook(w)
	if b == nil {
		return
	}
	c := b.Config()
	writeJSON(w, BountyReply{Post: c.Post, Take: c.Take, Mine: b.Mine(), Held: b.HeldOffers(), Payers: b.Payers()})
}

func (h *Handler) bountyPost(w http.ResponseWriter, r *http.Request) {
	var req BountyPostRequest
	if e := decodeBody(r, &req); e != nil {
		writeError(w, e)
		return
	}
	if !h.needHost(w) {
		return
	}
	if req.FindPool {
		ks := h.host.BountyPools(req.Series, req.Entry)
		switch len(ks) {
		case 0:
			writeError(w, &Error{Message: "no followed pool of the series holds entry " + req.Entry.String() + " (poo build prints an entry's hash; poo pools shows each pool's tip)", Code: CodeNotFound, Status: http.StatusNotFound})
			return
		case 1:
			req.Pool = ks[0]
		default:
			writeError(w, &Error{Message: fmt.Sprintf("pools %v all hold entry %s; name one with -pool", ks, req.Entry.Short()), Code: CodeConflict, Status: http.StatusConflict})
			return
		}
	}
	o := bounty.Offer{Kind: bounty.KindLayer, Series: req.Series, Pool: req.Pool, Entry: req.Entry,
		Strength: req.Strength, Amount: req.Amount, Count: req.Count, OpenSeconds: req.OpenSeconds}
	m, sent, err := h.host.PostBounty(o)
	if err != nil {
		writeError(w, bountyErr(err))
		return
	}
	writeJSON(w, BountyPostReply{Offer: m, Sent: sent})
}

func (h *Handler) bountyCancel(w http.ResponseWriter, r *http.Request) {
	var req BountyIDRequest
	if e := decodeBody(r, &req); e != nil {
		writeError(w, e)
		return
	}
	b := h.bountyBook(w)
	if b == nil {
		return
	}
	if err := b.Cancel(req.ID); err != nil {
		writeError(w, bountyErr(err))
		return
	}
	m, _ := b.Get(req.ID)
	writeJSON(w, m)
}

func (h *Handler) bountyAsk(w http.ResponseWriter, r *http.Request) {
	var req BountyAskRequest
	if e := decodeBody(r, &req); e != nil {
		writeError(w, e)
		return
	}
	if !h.needHost(w) {
		return
	}
	n, err := h.host.AskBounties(req.Series)
	if err != nil {
		writeError(w, bountyErr(err))
		return
	}
	writeJSON(w, BountyAskReply{Asked: n})
}

func (h *Handler) bountyPaid(w http.ResponseWriter, r *http.Request) {
	var req BountyPaidRequest
	if e := decodeBody(r, &req); e != nil {
		writeError(w, e)
		return
	}
	if !h.needHost(w) {
		return
	}
	told, err := h.host.PaidBounty(req.ID, req.Nonce, req.TIDs)
	if err != nil {
		writeError(w, bountyErr(err))
		return
	}
	writeJSON(w, BountyPaidReply{Told: told})
}

// bountyRate measures how many tries a second this node makes with the
// series' hash, over about a quarter of a second.
func bountyRate(ctx context.Context, id hash.ID, entry hash.Hash, workers int) uint64 {
	const tries = 1 << 16
	start := time.Now()
	n := uint64(0)
	for time.Since(start) < 250*time.Millisecond && ctx.Err() == nil {
		if _, _, err := layer.GrindHash(id, entry, n, tries, workers); err != nil {
			return 0
		}
		n += tries
	}
	el := time.Since(start).Seconds()
	if el <= 0 {
		return 0
	}
	return uint64(float64(n) / el)
}

func (h *Handler) bountyWork(w http.ResponseWriter, r *http.Request) {
	var req BountyWorkRequest
	if e := decodeBody(r, &req); e != nil {
		writeError(w, e)
		return
	}
	b := h.bountyBook(w)
	if b == nil {
		return
	}
	held, ok := b.HeldOffer(req.ID)
	if !ok {
		writeError(w, badRequest("no offer with that id was received; poo bounty list -ask asks peers for theirs"))
		return
	}
	s, ok := h.n.Engine().Series(held.Offer.Series)
	if !ok {
		writeError(w, badRequest("this node does not follow the offer's series"))
		return
	}
	ctx := r.Context()
	workers := max(1, runtime.NumCPU()-1)
	id := s.Params().HashID
	out := BountyWorkReply{Rate: bountyRate(ctx, id, held.Offer.Entry, workers)}
	if out.Rate == 0 {
		writeError(w, badRequest("could not measure this node's rate"))
		return
	}
	g, err := h.host.TakeBounty(ctx, req.ID, out.Rate, req.PayTo)
	if err != nil {
		writeError(w, bountyErr(err))
		return
	}
	out.Grant = g
	if g.Seconds == 0 {
		writeJSON(w, out)
		return
	}
	limit := MaxBountyWork
	if req.MaxSeconds > 0 {
		limit = min(limit, time.Duration(req.MaxSeconds)*time.Second)
	}
	limit = min(limit, time.Duration(g.Seconds)*time.Second)
	gctx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	const chunk = 1 << 20
	for start := uint64(0); gctx.Err() == nil; start += chunk {
		l, st, err := layer.GrindHash(id, held.Offer.Entry, start, chunk, workers)
		if err != nil {
			writeError(w, badRequest("%v", err))
			return
		}
		out.Tried += chunk
		if st >= int(held.Offer.Strength) {
			out.Found, out.Nonce, out.Strength = true, l.Nonce, st
			break
		}
	}
	if !out.Found {
		writeJSON(w, out)
		return
	}
	a, err := h.host.ClaimBounty(ctx, req.ID, out.Nonce, req.PayTo)
	if err != nil {
		writeError(w, bountyErr(err))
		return
	}
	out.Ack, out.Outcome = &a, bounty.OutcomeName(a.Outcome)
	writeJSON(w, out)
}

// Bounty returns the market as the node sees it.
func (c *Client) Bounty() (BountyReply, error) {
	var out BountyReply
	return out, c.do(http.MethodGet, "/v1/bounty", nil, &out)
}

// BountyPost posts an offer.
func (c *Client) BountyPost(req BountyPostRequest) (BountyPostReply, error) {
	var out BountyPostReply
	return out, c.do(http.MethodPost, "/v1/bounty/post", req, &out)
}

// BountyCancel closes an offer.
func (c *Client) BountyCancel(id hash.Hash) (bounty.Mine, error) {
	var out bounty.Mine
	return out, c.do(http.MethodPost, "/v1/bounty/cancel", BountyIDRequest{ID: id}, &out)
}

// BountyAsk asks peers for their open offers.
func (c *Client) BountyAsk(series hash.Hash) (BountyAskReply, error) {
	var out BountyAskReply
	return out, c.do(http.MethodPost, "/v1/bounty/ask", BountyAskRequest{Series: series}, &out)
}

// BountyWork leases, grinds and claims an offer.
func (c *Client) BountyWork(req BountyWorkRequest) (BountyWorkReply, error) {
	var out BountyWorkReply
	return out, c.do(http.MethodPost, "/v1/bounty/work", req, &out)
}

// BountyPaid records the payment of an accepted claim.
func (c *Client) BountyPaid(req BountyPaidRequest) (BountyPaidReply, error) {
	var out BountyPaidReply
	return out, c.do(http.MethodPost, "/v1/bounty/paid", req, &out)
}
