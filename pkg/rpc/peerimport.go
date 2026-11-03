package rpc

import (
	"errors"
	"net/http"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peerport"
)

// MaxPeerImportBody bounds a peer file import sent with the full token:
// any file peerport.Read takes, even with every byte escaped (six bytes
// each), and the options, as MaxImportBody does for a node policy.
const MaxPeerImportBody = 6*peerport.MaxSize + MaxBody

// PeersImportRequest imports a peer file into the running node's book and
// tracker (peerport.Import, D157). With DryRun it only returns the plan.
type PeersImportRequest struct {
	File    peerport.File    `json:"file"`
	Options peerport.Options `json:"options"`
	DryRun  bool             `json:"dry_run,omitempty"`
	// IfDigest, with an apply, applies only if the plan the node makes now
	// has this import digest (peerport.Digest of the file's digest, the
	// options and the plan), so what is applied is what was previewed
	// (RED-062, D158).
	IfDigest string `json:"if_digest,omitempty"`
}

// PeersImportReply is the plan, and whether it was applied.
type PeersImportReply struct {
	Plan    peerport.Plan `json:"plan"`
	Applied bool          `json:"applied,omitempty"`
}

func (h *Handler) importPeers(w http.ResponseWriter, r *http.Request) {
	if !h.needHost(w) {
		return
	}
	var req PeersImportRequest
	if e := decodeBody(r, &req); e != nil {
		writeError(w, e)
		return
	}
	n := peerport.Node{Book: h.host.Book(), Tracker: h.host.Tracker(), AllowLocal: h.host.AllowsLocalPeers(), Listed: h.host.Listed()}
	var plan peerport.Plan
	var err error
	if req.IfDigest != "" && !req.DryRun {
		plan, err = peerport.ImportIf(req.File, req.Options, n, req.IfDigest, time.Now())
	} else {
		plan, err = peerport.Import(req.File, req.Options, n, !req.DryRun, time.Now())
	}
	if errors.Is(err, peerport.ErrNotPreviewed) {
		writeError(w, &Error{Message: err.Error(), Code: CodeConflict, Status: http.StatusConflict})
		return
	}
	if err != nil {
		writeError(w, &Error{Message: err.Error(), Code: CodeInvalid, Status: http.StatusUnprocessableEntity})
		return
	}
	writeJSON(w, PeersImportReply{Plan: plan, Applied: !req.DryRun && !plan.Empty()})
}

// ImportPeers imports a peer file into the running node.
func (c *Client) ImportPeers(req PeersImportRequest) (PeersImportReply, error) {
	var out PeersImportReply
	return out, c.do("POST", "/v1/peers/import", req, &out)
}

// ForgetImportedRequest forgets the addresses known only from imported
// peer files (peers.Book.ForgetImported, RED-058). With DryRun it only
// counts them.
type ForgetImportedRequest struct {
	DryRun bool `json:"dry_run,omitempty"`
}

// ForgetImportedReply says how many records are known only from imported
// files, and how many were forgotten.
type ForgetImportedReply struct {
	Imported  int `json:"imported"`
	Forgotten int `json:"forgotten"`
}

func (h *Handler) forgetImported(w http.ResponseWriter, r *http.Request) {
	if !h.needHost(w) {
		return
	}
	var req ForgetImportedRequest
	if e := decodeBody(r, &req); e != nil {
		writeError(w, e)
		return
	}
	book := h.host.Book()
	rep := ForgetImportedReply{Imported: book.Unlisted()}
	if !req.DryRun {
		rep.Forgotten = book.ForgetImported()
		if err := book.Save(); err != nil {
			writeError(w, &Error{Message: err.Error(), Code: CodeInternal, Status: http.StatusInternalServerError})
			return
		}
	}
	writeJSON(w, rep)
}

// ForgetImported forgets, or with DryRun counts, the running node's
// imported addresses.
func (c *Client) ForgetImported(req ForgetImportedRequest) (ForgetImportedReply, error) {
	var out ForgetImportedReply
	return out, c.do("POST", "/v1/peers/forget-imported", req, &out)
}
