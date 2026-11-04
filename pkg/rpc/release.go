package rpc

import (
	"errors"
	"net/http"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

// ReleaseRequest gives the node a release of a part of a variable series
// (D166). The node keeps it in node.json when it is valid and newer than
// the one kept, and follows the new part's pools in place, passing the
// release on to its peers (D168), or reloads when it cannot.
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
	// InPlace says the running node took the release with no reload: the
	// part's pools are followed and the release is passed on (D168).
	InPlace bool `json:"in_place,omitempty"`
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
	refuse := func(err error) {
		status, code := http.StatusUnprocessableEntity, CodeInvalid
		if errors.Is(err, nodepolicy.ErrSeriesNotListed) {
			status, code = http.StatusNotFound, CodeNotFound
		}
		writeError(w, &Error{Message: err.Error(), Code: code, Status: status})
	}
	// Checking a far part takes its part number in hashes the first time,
	// so it is done before policyMu is taken: other policy commands go on
	// meanwhile, and the add below finds the part ids already worked out
	// (RED-075).
	if !req.NoReload {
		if _, ok := h.n.Engine().Series(req.Release.Series); ok {
			h.addReleaseInPlace(w, req.Release, refuse)
			return
		}
	}
	h.policyMu.Lock()
	path := h.policyPath()
	p, err := nodepolicy.Load(path)
	h.policyMu.Unlock()
	if err != nil {
		writeError(w, classify(err))
		return
	}
	if err := p.CheckRelease(req.Release); err != nil {
		refuse(err)
		return
	}
	h.policyMu.Lock()
	p, err = nodepolicy.Load(path)
	if err != nil {
		h.policyMu.Unlock()
		writeError(w, classify(err))
		return
	}
	res, err := p.AddRelease(req.Release)
	if err != nil {
		h.policyMu.Unlock()
		refuse(err)
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

// addReleaseInPlace gives the running node r through node.AddRelease,
// which checks it in full, keeps it in node.json and adds it to the
// running series; the host then indexes the part's pools as on start and
// passes r on. When the host cannot index them so, the node reloads onto
// the saved node.json instead.
func (h *Handler) addReleaseInPlace(w http.ResponseWriter, r series.Release, refuse func(error)) {
	res, err := h.n.AddRelease(r)
	if err != nil {
		if errors.Is(err, series.ErrRelease) || errors.Is(err, series.ErrFixed) || errors.Is(err, nodepolicy.ErrSeriesNotListed) {
			refuse(err)
			return
		}
		writeError(w, classify(err))
		return
	}
	out := ReleaseReply{ReleaseAdded: res, Saved: res.Added}
	if res.Added {
		out.InPlace = h.host == nil || h.host.AddPools(r.Series)
		out.Reloading = !out.InPlace
		if h.host != nil {
			h.host.Announce(r, "")
		}
	}
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
