package nodepolicy

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
)

// GrindEdit is one change to a grind section, applied where the section
// is kept: by the node under the lock that orders its changes, or to
// node.json by a command run without a node. Changing a section this way
// instead of reading it and writing it back whole means two changes made
// at once both take effect, rather than one silently undoing the other
// (GUI-014, decision D125).
//
// The steps apply in order: Remove, then Add, then Set.
type GrindEdit struct {
	// Remove takes out jobs. Each names a job by its number, from 1 as
	// listed, and by the whole job, which must still be the one at that
	// number, so a stale number never removes another job; two jobs that
	// are the same in every field are one choice.
	Remove []GrindRemove `json:"remove,omitempty"`
	// RemoveAll takes out every job, before Add.
	RemoveAll bool `json:"remove_all,omitempty"`
	// Add appends jobs.
	Add []GrindJob `json:"add,omitempty"`
	// Set changes the section's own settings; a nil field is left as it
	// is.
	Set *GrindSet `json:"set,omitempty"`
	// Version, when set, is the Version of the section the edit was made
	// against: the edit is refused with ErrGrindChanged if the section
	// has changed since.
	Version string `json:"version,omitempty"`
}

// GrindRemove names one job to remove.
type GrindRemove struct {
	N   int      `json:"n"`
	Job GrindJob `json:"job"`
}

// GrindSet changes a grind section's own settings.
type GrindSet struct {
	Device     *string `json:"device,omitempty"`
	Threads    *int    `json:"threads,omitempty"`
	LayerFloor *int    `json:"layer_floor,omitempty"`
	LayerRate  *int    `json:"layer_rate,omitempty"`
}

// ErrGrindChanged is an edit or a replacement made against a version of
// the grind section that is no longer the current one.
var ErrGrindChanged = errors.New("the grind schedule changed meanwhile: list it again")

// IsZero reports whether e changes nothing.
func (e GrindEdit) IsZero() bool {
	return len(e.Remove) == 0 && !e.RemoveAll && len(e.Add) == 0 && e.Set == nil
}

// Apply returns g with e applied. g is not changed. The result is not
// validated: the caller validates the policy it goes into.
func (e GrindEdit) Apply(g Grind) (Grind, error) {
	if e.Version != "" && e.Version != g.Version() {
		return g, ErrGrindChanged
	}
	next := g
	next.Jobs = slices.Clone(g.Jobs)
	if e.RemoveAll {
		next.Jobs = nil
	} else if len(e.Remove) > 0 {
		drop := map[int]bool{}
		for _, r := range e.Remove {
			if r.N < 1 || r.N > len(g.Jobs) {
				return g, fmt.Errorf("no scheduled job %d: they are numbered 1 to %d: %w", r.N, len(g.Jobs), ErrGrindChanged)
			}
			if !SameJob(g.Jobs[r.N-1], r.Job) {
				return g, fmt.Errorf("scheduled job %d is not the job named: %w", r.N, ErrGrindChanged)
			}
			drop[r.N-1] = true
		}
		next.Jobs = nil
		for i, j := range g.Jobs {
			if !drop[i] {
				next.Jobs = append(next.Jobs, j)
			}
		}
	}
	next.Jobs = append(next.Jobs, e.Add...)
	if s := e.Set; s != nil {
		if s.Device != nil {
			next.Device = *s.Device
			if next.Device == "auto" {
				next.Device = ""
			}
		}
		if s.Threads != nil {
			next.Threads = *s.Threads
		}
		if s.LayerFloor != nil {
			next.LayerFloor = *s.LayerFloor
		}
		if s.LayerRate != nil {
			next.LayerRate = *s.LayerRate
		}
	}
	return next, nil
}

// SameJob reports whether a and b are the same job in every field.
func SameJob(a, b GrindJob) bool {
	ja, _ := json.Marshal(a)
	jb, _ := json.Marshal(b)
	return string(ja) == string(jb)
}

// Version names the content of g: it changes whenever any field does, so
// a client that read g can make a change only against what it read.
func (g Grind) Version() string {
	b, _ := json.Marshal(g)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}
