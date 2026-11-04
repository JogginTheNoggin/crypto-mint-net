package rpc

import (
	"net/http"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
)

// LimitsRequest sets bounded settings of node.json by name (D161): 0
// puts one back to its default. They are checked whole against their
// bounds before anything is saved.
type LimitsRequest struct {
	Set map[string]int `json:"set"`
	// NoReload saves node.json without applying it now.
	NoReload bool `json:"no_reload,omitempty"`
}

// LimitsReply is every bounded setting after the change, whether node.json
// changed, and whether the node is reloading to apply it.
type LimitsReply struct {
	Limits    []nodepolicy.Limit `json:"limits"`
	Saved     bool               `json:"saved"`
	Reloading bool               `json:"reloading,omitempty"`
	// InPlace says the running node applied the change without a reload
	// (D168): every changed limit is one nodepolicy.InPlace names.
	InPlace bool `json:"in_place,omitempty"`
}

func (h *Handler) setLimits(w http.ResponseWriter, r *http.Request) {
	var req LimitsRequest
	if e := decodeBody(r, &req); e != nil {
		writeError(w, e)
		return
	}
	if len(req.Set) == 0 {
		writeError(w, badRequest("set names no limit"))
		return
	}
	if !req.NoReload && h.reload == nil {
		writeError(w, &Error{Message: "this node cannot reload; set no_reload and restart it", Code: CodeNotFound, Status: http.StatusNotFound})
		return
	}
	h.policyMu.Lock()
	path := h.policyPath()
	p, err := nodepolicy.Load(path)
	if err != nil {
		h.policyMu.Unlock()
		writeError(w, classify(err))
		return
	}
	changed, err := p.SetLimits(req.Set)
	if err != nil {
		h.policyMu.Unlock()
		writeError(w, &Error{Message: err.Error(), Code: CodeInvalid, Status: http.StatusUnprocessableEntity})
		return
	}
	out := LimitsReply{Limits: p.Limits()}
	if changed {
		if err := nodepolicy.Save(path, p); err != nil {
			h.policyMu.Unlock()
			writeError(w, classify(err))
			return
		}
		out.Saved, out.Reloading = true, !req.NoReload
		if out.Reloading && h.applyInPlace(p) {
			out.Reloading, out.InPlace = false, true
		}
	}
	h.policyMu.Unlock()
	writeJSON(w, out)
	if out.Reloading {
		h.reload()
	}
}

// SetLimits sets bounded settings of the node's node.json and applies them.
func (c *Client) SetLimits(req LimitsRequest) (LimitsReply, error) {
	var r LimitsReply
	return r, c.do("POST", "/v1/policy/limits", req, &r)
}

// applyInPlace applies changed limits to the running node's parts when
// every one of them can be (nodepolicy.InPlace), and reports whether it
// did. The changes are taken against the policy the node runs with, not
// the node.json read before the change, so limits edited by hand since are
// applied too, and anything else edited by hand leaves it to the reload
// (nodepolicy.SameButInPlace). Each part ends as a reload onto the same node.json would leave it:
// the book takes its new limits and fits to them as on start, the tracker
// its retention, and the engine a new, empty seen list (ORANGE-035's
// design, accepted by Black in Round 25; D168). Anything else is left to
// the reload, and so is everything when this handler has no p2p host.
func (h *Handler) applyInPlace(p nodepolicy.Policy) bool {
	if h.host == nil {
		return false
	}
	run := h.n.Running()
	if !nodepolicy.SameButInPlace(run, p) {
		return false
	}
	for _, name := range nodepolicy.ChangedLimits(run, p) {
		if !nodepolicy.InPlace[name] {
			return false
		}
	}
	if run.BookLimits() != p.BookLimits() {
		h.host.Book().SetLimits(p.BookLimits(), time.Now())
	}
	if run.Retention() != p.Retention() {
		h.host.Tracker().SetRetention(p.Retention())
	}
	if n := nodepolicy.BoundSeenSize.Or(p.SeenSize); nodepolicy.BoundSeenSize.Or(run.SeenSize) != n {
		h.n.Engine().SetSeenSize(n)
	}
	h.n.SetRunning(p)
	return true
}
