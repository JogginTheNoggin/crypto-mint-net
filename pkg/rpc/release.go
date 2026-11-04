package rpc

import (
	"errors"
	"net/http"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

// ReleaseRequest gives the node a release of a part of a variable series
// (D166). The node keeps it in node.json when it is valid and newer than
// the one kept, and reloads to follow the new part's pools.
type ReleaseRequest struct {
	Release series.Release `json:"release"`
	// NoReload saves node.json without applying it now.
	NoReload bool `json:"no_reload,omitempty"`
}

// ReleaseReply says what the node did with a release.
type ReleaseReply struct {
	nodepolicy.ReleaseAdded
	Saved     bool `json:"saved,omitempty"`
	Reloading bool `json:"reloading,omitempty"`
}

func (h *Handler) addRelease(w http.ResponseWriter, r *http.Request) {
	var req ReleaseRequest
	if e := decodeBody(r, &req); e != nil {
		writeError(w, e)
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
	res, err := p.AddRelease(req.Release)
	if err != nil {
		h.policyMu.Unlock()
		status, code := http.StatusUnprocessableEntity, CodeInvalid
		if errors.Is(err, nodepolicy.ErrSeriesNotListed) {
			status, code = http.StatusNotFound, CodeNotFound
		}
		writeError(w, &Error{Message: err.Error(), Code: code, Status: status})
		return
	}
	out := ReleaseReply{ReleaseAdded: res}
	if res.Added {
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

// AddRelease gives the running node a release.
func (c *Client) AddRelease(req ReleaseRequest) (ReleaseReply, error) {
	var r ReleaseReply
	return r, c.do("POST", "/v1/series/release", req, &r)
}
