package rpc

import (
	"net/http"

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
