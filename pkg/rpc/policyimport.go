package rpc

import (
	"net/http"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/grind"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
)

// thisMachine describes the node's machine for an import's fit check
// (nodepolicy.Fit, D155); a test may replace it.
var thisMachine = func() nodepolicy.Machine { return grind.ThisMachine(grind.Default) }

// PolicyImportRequest merges a node policy from a bundle into the running
// node's (nodepolicy.Merge, D149). With DryRun it only returns the plan;
// otherwise it saves the result and, unless NoReload, applies it.
type PolicyImportRequest struct {
	Policy   nodepolicy.Policy       `json:"policy"`
	Options  nodepolicy.MergeOptions `json:"options"`
	DryRun   bool                    `json:"dry_run,omitempty"`
	NoReload bool                    `json:"no_reload,omitempty"`
}

// PolicyImportReply is the plan, and whether it was saved and is being
// applied.
type PolicyImportReply struct {
	Plan      nodepolicy.Plan `json:"plan"`
	Saved     bool            `json:"saved,omitempty"`
	Reloading bool            `json:"reloading,omitempty"`
}

// importPolicy merges under policyMu, so it never races a follow change or
// a schedule edit, and saves only a result that validates.
func (h *Handler) importPolicy(w http.ResponseWriter, r *http.Request) {
	var req PolicyImportRequest
	if e := decodeBody(r, &req); e != nil {
		writeError(w, e)
		return
	}
	invalid := func(err error) {
		writeError(w, &Error{Message: err.Error(), Code: CodeInvalid, Status: http.StatusUnprocessableEntity})
	}
	if err := req.Policy.Validate(); err != nil {
		invalid(err)
		return
	}
	if !req.DryRun && !req.NoReload && h.reload == nil {
		writeError(w, &Error{Message: "this node cannot reload; set no_reload and restart it", Code: CodeNotFound, Status: http.StatusNotFound})
		return
	}
	path := h.policyPath()
	h.policyMu.Lock()
	local, err := nodepolicy.Load(path)
	if err != nil {
		h.policyMu.Unlock()
		writeError(w, classify(err))
		return
	}
	// The interface never rewrites its own section: where and to whom it
	// listens changes only in node.json with the node stopped, so a token
	// holder cannot open it to the network (D149).
	rpcKept := req.Options.Full && req.Policy.RPC != (nodepolicy.RPC{}) && req.Policy.RPC != local.RPC
	if req.Options.Full {
		req.Policy.RPC = local.RPC
	}
	res, plan, err := nodepolicy.Merge(local, req.Policy, req.Options)
	if err != nil {
		h.policyMu.Unlock()
		invalid(err)
		return
	}
	plan.Fit = nodepolicy.Fit(local, res, thisMachine())
	if rpcKept {
		plan.Skipped = append(plan.Skipped, "rpc (changes only with the node stopped)")
	}
	out := PolicyImportReply{Plan: plan}
	if !req.DryRun && !plan.Empty() {
		if err := nodepolicy.Save(path, res); err != nil {
			h.policyMu.Unlock()
			writeError(w, classify(err))
			return
		}
		out.Saved = true
		out.Reloading = !req.NoReload
	}
	h.policyMu.Unlock()
	writeJSON(w, out)
	if out.Reloading {
		h.reload()
	}
}
