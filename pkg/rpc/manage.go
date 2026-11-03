package rpc

import (
	"errors"
	"net/http"
	"net/url"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/chase"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
)

// The owner's view of peer tracking and the chase queue, and their
// controls over single peers: timeouts, blocks, clearing records,
// templates and deleting.

// TrackingReply is the reply of GET /v1/tracking without an address.
type TrackingReply struct {
	// Tracked maps each tracked key to its status.
	Tracked map[string]tracking.Status `json:"tracked"`
	// Ranges are the blocked network ranges and until when.
	Ranges map[string]time.Time `json:"ranges"`
}

// ChasesReply is the reply of GET /v1/chases.
type ChasesReply struct {
	Groups []chase.Group `json:"groups"`
}

// PeerRequest is one action on a peer, by address. Actions:
//
//	timeout   pause it for Seconds (0: the policy's length)
//	block     refuse it for Seconds (0: the policy's length, -1: until
//	          unblocked), or its whole network range with Range
//	unblock   end a timeout or block of it and its range
//	clear     clear its flags, measures or all (What)
//	template  track it with Template ("" for its groups and the default)
//	delete    take it out of the peer book and forget it
//	add       put it in the peer book by hand, as a candidate for every
//	          followed mint policy, with Label; Keep also lists it in
//	          node.json (p2p.peers) so it is always dialled (D157)
//	remove    delete, and take it out of node.json's p2p.peers
//	label     set its Label ("" clears it)
//	connect   dial it now; with Imported and no address, make every
//	          unlisted address from an imported peer file a candidate
type PeerRequest struct {
	Addr     string `json:"addr"`
	Action   string `json:"action"`
	Label    string `json:"label,omitempty"`
	Keep     bool   `json:"keep,omitempty"`
	Imported bool   `json:"imported,omitempty"`
	Seconds  int    `json:"seconds,omitempty"`
	Range    bool   `json:"range,omitempty"`
	What     string `json:"what,omitempty"`
	Template string `json:"template,omitempty"`
}

// PeerReply says what an action did.
type PeerReply struct {
	// Key is the tracker's key for the address.
	Key string `json:"key"`
	// Closed counts the connections the action closed.
	Closed int `json:"closed,omitempty"`
	// Held reports, for delete, remove and label, whether the peer book
	// held the address.
	Held bool `json:"held,omitempty"`
	// Kept reports, for add with keep and for remove, whether node.json's
	// p2p.peers changed.
	Kept bool `json:"kept,omitempty"`
	// Promoted counts, for connect with imported, the addresses made
	// candidates.
	Promoted int `json:"promoted,omitempty"`
	// Status is the address's status after the action.
	Status tracking.Status `json:"status"`
}

func (h *Handler) needHost(w http.ResponseWriter) bool {
	if h.host == nil {
		writeError(w, &Error{Message: "p2p is not running", Code: CodeNotFound, Status: http.StatusNotFound})
		return false
	}
	return true
}

func (h *Handler) tracking(w http.ResponseWriter, r *http.Request) {
	if !h.needHost(w) {
		return
	}
	if addr := r.URL.Query().Get("addr"); addr != "" {
		rep, _ := h.host.TrackerReport(addr)
		writeJSON(w, rep)
		return
	}
	tracked, ranges := h.host.Tracked()
	writeJSON(w, TrackingReply{Tracked: tracked, Ranges: ranges})
}

func (h *Handler) chases(w http.ResponseWriter, r *http.Request) {
	if !h.needHost(w) {
		return
	}
	gs := h.host.ChaseQueue().Groups("", time.Now())
	if gs == nil {
		gs = []chase.Group{}
	}
	writeJSON(w, ChasesReply{Groups: gs})
}

func (h *Handler) peer(w http.ResponseWriter, r *http.Request) {
	if !h.needHost(w) {
		return
	}
	var req PeerRequest
	if e := decodeBody(r, &req); e != nil {
		writeError(w, e)
		return
	}
	if req.Addr == "" && !(req.Action == "connect" && req.Imported) {
		writeError(w, badRequest("addr is required"))
		return
	}
	d := time.Duration(req.Seconds) * time.Second
	var out PeerReply
	switch req.Action {
	case "timeout":
		if req.Seconds < 0 {
			writeError(w, badRequest("a timeout must not be negative"))
			return
		}
		out.Closed = h.host.TimeOut(req.Addr, d)
	case "block":
		if req.Seconds < -1 {
			writeError(w, badRequest("seconds: -1 blocks until unblocked"))
			return
		}
		out.Closed = h.host.Block(req.Addr, d, req.Range)
	case "unblock":
		h.host.Unblock(req.Addr)
	case "clear":
		if err := h.host.ClearPeer(req.Addr, req.What); err != nil {
			writeError(w, badRequest("%v", err))
			return
		}
	case "template":
		if err := h.host.SetPeerTemplate(req.Addr, req.Template); err != nil {
			writeError(w, badRequest("%v", err))
			return
		}
	case "delete":
		out.Held = h.host.DeletePeer(req.Addr)
	case "add":
		if err := h.host.AddPeer(req.Addr, req.Label); err != nil && !(req.Keep && errors.Is(err, p2p.ErrNotPublic)) {
			writeError(w, badRequest("%v", err))
			return
		}
		if req.Keep {
			kept, e := h.keepPeer(req.Addr, true)
			if e != nil {
				writeError(w, e)
				return
			}
			out.Kept = kept
		}
		out.Held = true
	case "remove":
		out.Held = h.host.DeletePeer(req.Addr)
		kept, e := h.keepPeer(req.Addr, false)
		if e != nil {
			writeError(w, e)
			return
		}
		out.Kept = kept
	case "label":
		held, err := h.host.LabelPeer(req.Addr, req.Label)
		if err != nil {
			writeError(w, badRequest("%v", err))
			return
		}
		out.Held = held
	case "connect":
		if req.Imported {
			if req.Addr != "" {
				writeError(w, badRequest("connect takes an address or imported, not both"))
				return
			}
			out.Promoted = h.host.ConnectImported()
			writeJSON(w, out)
			return
		}
		if err := h.host.Connect(req.Addr); err != nil {
			writeError(w, badRequest("dialling %s: %v", req.Addr, err))
			return
		}
	default:
		writeError(w, badRequest("unknown action %q", req.Action))
		return
	}
	tr := h.host.Tracker()
	out.Key = tr.Key(req.Addr)
	out.Status = tr.Status(req.Addr, time.Now())
	writeJSON(w, out)
}

// keepPeer adds addr to, or takes it out of, node.json's p2p.peers under
// the same lock as the other edits of node.json. The running node already
// has it in its book; the change is for its next start or reload.
func (h *Handler) keepPeer(addr string, keep bool) (bool, *Error) {
	h.policyMu.Lock()
	defer h.policyMu.Unlock()
	path := h.policyPath()
	p, err := nodepolicy.Load(path)
	if err != nil {
		return false, classify(err)
	}
	if !nodepolicy.KeepPeer(&p, addr, keep) {
		return false, nil
	}
	if err := nodepolicy.Save(path, p); err != nil {
		return false, classify(err)
	}
	return true, nil
}

// Tracking returns every tracked key's status and the blocked ranges.
func (c *Client) Tracking() (TrackingReply, error) {
	var r TrackingReply
	return r, c.do("GET", "/v1/tracking", nil, &r)
}

// TrackingOf returns everything the tracker keeps about addr.
func (c *Client) TrackingOf(addr string) (tracking.Report, error) {
	var r tracking.Report
	return r, c.do("GET", "/v1/tracking?"+url.Values{"addr": {addr}}.Encode(), nil, &r)
}

// Chases returns the chase queue's groups, in queue order.
func (c *Client) Chases() (ChasesReply, error) {
	var r ChasesReply
	return r, c.do("GET", "/v1/chases", nil, &r)
}

// Peer runs one action on a peer.
func (c *Client) Peer(req PeerRequest) (PeerReply, error) {
	var r PeerReply
	return r, c.do("POST", "/v1/peer", req, &r)
}
