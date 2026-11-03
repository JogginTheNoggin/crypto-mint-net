package rpc

import (
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
	plan, err := peerport.Import(req.File, req.Options, n, !req.DryRun, time.Now())
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
