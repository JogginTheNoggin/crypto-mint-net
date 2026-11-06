package peerport

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// Options choose what an import takes besides the addresses.
type Options struct {
	// Blocks takes the file's address blocks. By default they are listed
	// in the plan and left alone.
	Blocks bool `json:"blocks,omitempty"`
	// Ranges takes the file's network range blocks too. It needs Blocks.
	Ranges bool `json:"ranges,omitempty"`
}

// Tracker is what an import needs of the node's tracker
// (*tracking.Tracker).
type Tracker interface {
	Key(addr string) string
	Status(addr string, now time.Time) tracking.Status
	Ranges() map[string]time.Time
	BlockFor(addr string, d time.Duration, now time.Time)
	BlockRange(addr string, d time.Duration, now time.Time) string
	Save() error
}

// Node is the node an import lands on. It is not part of the digest: the
// plan it gives is.
type Node struct {
	Book    *peers.Book
	Tracker Tracker
	// AllowLocal is the node policy's p2p.allow_local_peers.
	AllowLocal bool
	// Listed are the addresses the node policy lists (p2p.peers) and the
	// node's own: never blocked by a file, and taken even when local.
	Listed []string
}

// Line is one line of a plan.
type Line struct {
	Addr   string `json:"addr"`
	Detail string `json:"detail,omitempty"`
}

// Plan says what an import does.
type Plan struct {
	// Added are addresses new to the book, each unlisted.
	Added []Line `json:"added"`
	// More are known addresses that gain mint policies, as unlisted.
	More []Line `json:"more"`
	// Labels are labels set where the book has none; LabelConflicts are
	// labels the book already has otherwise, kept.
	Labels         []Line `json:"labels"`
	LabelConflicts []Line `json:"label_conflicts"`
	// Blocks are the blocks the import adds or lengthens (with
	// Options.Blocks).
	Blocks []Line `json:"blocks"`
	// Skipped are blocks left alone, and why.
	Skipped []Line `json:"skipped"`
	// Refused are addresses the book does not take, and why.
	Refused []Line `json:"refused"`
	// Same counts the file's addresses and blocks the node already has.
	Same int `json:"same"`
	// Notes say what the plan does beyond its lines: that imported
	// addresses will be dialled, how many places of the book they take,
	// how much of the address space the range blocks cover, and that a
	// listed host name is checked by name only (RED-058, RED-059, RED-060,
	// RED-062, D158).
	Notes []string `json:"notes"`
}

// Empty reports whether the import changes nothing.
func (p Plan) Empty() bool {
	return len(p.Added) == 0 && len(p.More) == 0 && len(p.Labels) == 0 && len(p.Blocks) == 0
}

// op is one change an import makes.
type op struct {
	addr     string
	sections []wire.Section
	label    string
	block    string // an address or a range address
	isRange  bool
	d        time.Duration
}

// Import plans the file's import into n and, with apply, makes it: every
// new address and mint policy unlisted (peers.Book.AddUnlisted), labels
// where the book has none, and, with Options.Blocks, the blocks that are
// longer than what the node holds. It never removes, unblocks or shortens
// anything, and it saves the book and the tracker when it changed them.
func Import(f File, opt Options, n Node, apply bool, now time.Time) (Plan, error) {
	return importWith(f, opt, n, apply, nil, now)
}

// ErrNotPreviewed is returned by ImportIf when the plan made now is not
// the one whose digest was given.
var ErrNotPreviewed = errors.New("peerport: this is not the one previewed")

// ImportIf is Import with apply, but it applies only if the plan it makes
// has the import digest want (Digest of the file's digest, the options and
// the plan). The plan checked is the plan applied: a running node's book
// can change between a preview and an apply (RED-062, D158).
func ImportIf(f File, opt Options, n Node, want string, now time.Time) (Plan, error) {
	return importWith(f, opt, n, true, func(p Plan) error {
		if dg := Digest(f.Digest, opt, p); !strings.EqualFold(dg, want) {
			return fmt.Errorf("%w: the file, -blocks, -ranges or what it would change differ (digest %s, not %s), so nothing was changed: preview it again", ErrNotPreviewed, dg, want)
		}
		return nil
	}, now)
}

func importWith(f File, opt Options, n Node, apply bool, check func(Plan) error, now time.Time) (Plan, error) {
	if opt.Ranges && !opt.Blocks {
		return Plan{}, fmt.Errorf("peerport: range blocks come with blocks: take both")
	}
	if err := f.Check(); err != nil {
		return Plan{}, err
	}
	plan := Plan{Added: []Line{}, More: []Line{}, Labels: []Line{}, LabelConflicts: []Line{}, Blocks: []Line{}, Skipped: []Line{}, Refused: []Line{}, Notes: []string{}}
	var ops []op
	listed := map[string]bool{}
	listedKey := map[string]bool{}
	listedRange := map[string]bool{}
	for _, a := range n.Listed {
		listed[a] = true
		listedKey[n.Tracker.Key(a)] = true
		listedRange[peers.Range(a)] = true
	}

	// The book as it stands, and how full it and each range would get.
	have := map[string]peers.Record{}
	perRange := map[string]int{}
	for _, r := range n.Book.Records() {
		have[r.Addr] = r
		perRange[peers.Range(r.Addr)]++
	}
	lim := n.Book.Limits()
	total := len(have)
	unlisted := n.Book.Unlisted()
	seen := map[string]bool{}
	for _, p := range f.Peers {
		if seen[p.Addr] {
			continue
		}
		seen[p.Addr] = true
		var secs []wire.Section
		for _, pp := range p.Policies {
			s, _ := wire.ParseSection(pp.Section) // Check read every one
			secs = append(secs, s)
		}
		st := n.Tracker.Status(p.Addr, now)
		switch {
		case st.Blocked:
			plan.Refused = append(plan.Refused, Line{p.Addr, "blocked here"})
			continue
		case !n.AllowLocal && !listed[p.Addr] && !p2p.Learnable(p.Addr):
			plan.Refused = append(plan.Refused, Line{p.Addr, "not a public address (p2p.allow_local_peers takes it)"})
			continue
		}
		r, known := have[p.Addr]
		if !known {
			rng := peers.Range(p.Addr)
			switch {
			case unlisted >= lim.MaxUnlisted:
				// A file takes at most a share of the book, so it never
				// fills it against what the node learns (RED-058, D158).
				plan.Refused = append(plan.Refused, Line{p.Addr, fmt.Sprintf("the book keeps at most %d imported addresses (of its %d places)", lim.MaxUnlisted, lim.MaxRecords)})
				continue
			case total >= lim.MaxRecords:
				plan.Refused = append(plan.Refused, Line{p.Addr, fmt.Sprintf("the peer book is full (%d)", lim.MaxRecords)})
				continue
			case perRange[rng] >= lim.PerRange:
				plan.Refused = append(plan.Refused, Line{p.Addr, fmt.Sprintf("%d addresses from network range %s already", lim.PerRange, rng)})
				continue
			}
			total++
			unlisted++
			perRange[rng]++
			plan.Added = append(plan.Added, Line{p.Addr, fmt.Sprintf("unlisted, %d mint %s", len(secs), plural(len(secs), "policy", "policies"))})
			ops = append(ops, op{addr: p.Addr, sections: secs, label: p.Label})
			if p.Label != "" {
				plan.Labels = append(plan.Labels, Line{p.Addr, printable(p.Label)})
			}
			continue
		}
		var add []wire.Section
		for _, s := range secs {
			if !hasSection(r, s) {
				add = append(add, s)
			}
		}
		o := op{addr: p.Addr, sections: add}
		switch {
		case p.Label == "" || p.Label == r.Label:
		case r.Label == "":
			o.label = p.Label
			plan.Labels = append(plan.Labels, Line{p.Addr, printable(p.Label)})
		default:
			plan.LabelConflicts = append(plan.LabelConflicts, Line{p.Addr, fmt.Sprintf("kept %q (file: %q)", r.Label, p.Label)})
		}
		if len(add) > 0 {
			plan.More = append(plan.More, Line{p.Addr, fmt.Sprintf("%d more mint %s, unlisted", len(add), plural(len(add), "policy", "policies"))})
		}
		if len(add) > 0 || o.label != "" {
			ops = append(ops, o)
		} else {
			plan.Same++
		}
	}

	// Blocks: only when asked, only adding or lengthening.
	if len(f.Blocks) > 0 && !opt.Blocks {
		plan.Skipped = append(plan.Skipped, Line{"", fmt.Sprintf("%d %s (-blocks takes them)", len(f.Blocks), plural(len(f.Blocks), "block", "blocks"))})
	}
	ranges := n.Tracker.Ranges()
	for _, b := range f.Blocks {
		if !opt.Blocks {
			break
		}
		d := b.Until.Sub(now)
		if d <= 0 {
			plan.Skipped = append(plan.Skipped, Line{b.Addr + b.Range, "already ended"})
			continue
		}
		until := "until " + b.Until.UTC().Format(time.RFC3339)
		if d > forever {
			d, until = -1, "until unblocked"
		}
		if b.Range != "" {
			addr, _ := rangeAddr(b.Range)
			rng := peers.Range(addr)
			switch {
			case !opt.Ranges:
				plan.Skipped = append(plan.Skipped, Line{"range " + b.Range, "a network range (-ranges takes it)"})
			case listedRange[rng]:
				plan.Skipped = append(plan.Skipped, Line{"range " + b.Range, "holds an address this node policy lists"})
			case !ranges[rng].Before(b.Until):
				plan.Same++
			default:
				plan.Blocks = append(plan.Blocks, Line{"range " + b.Range, until})
				ops = append(ops, op{block: addr, isRange: true, d: d})
			}
			continue
		}
		key := n.Tracker.Key(b.Addr)
		st := n.Tracker.Status(b.Addr, now)
		switch {
		case listedKey[key]:
			plan.Skipped = append(plan.Skipped, Line{b.Addr, "this node policy lists it (p2p.peers)"})
		case st.Blocked && !st.BlockedUntil.Before(b.Until):
			plan.Same++
		default:
			plan.Blocks = append(plan.Blocks, Line{b.Addr, until})
			ops = append(ops, op{block: b.Addr, d: d})
		}
	}
	plan.Notes = notes(plan, opt, n, unlisted, lim)
	if !apply || plan.Empty() {
		return plan, nil
	}
	if check != nil {
		if err := check(plan); err != nil {
			return plan, err
		}
	}
	booked, blocked := false, false
	for _, o := range ops {
		switch {
		case o.block != "" && o.isRange:
			n.Tracker.BlockRange(o.block, o.d, now)
			blocked = true
		case o.block != "":
			n.Tracker.BlockFor(o.block, o.d, now)
			blocked = true
		default:
			if len(o.sections) > 0 {
				if err := n.Book.AddUnlisted(o.addr, o.sections, now); err != nil {
					return plan, fmt.Errorf("peerport: %s: %w", o.addr, err)
				}
			}
			if o.label != "" {
				if _, err := n.Book.SetLabel(o.addr, o.label); err != nil {
					return plan, err
				}
			}
			booked = true
		}
	}
	if booked {
		if err := n.Book.Save(); err != nil {
			return plan, err
		}
	}
	if blocked {
		if err := n.Tracker.Save(); err != nil {
			return plan, err
		}
	}
	return plan, nil
}

func hasSection(r peers.Record, s wire.Section) bool {
	for _, p := range r.Policies {
		if p.Section == s {
			return true
		}
	}
	return false
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// Digest is the SHA-256 of what an import applies: the file's digest, the
// options and the plan. A preview prints it, and an apply with the digest
// recomputes it from a fresh plan and refuses a difference, as policy
// import does (D151, D154).
func Digest(file string, opt Options, p Plan) string {
	b, _ := json.Marshal(struct {
		File    string  `json:"file"`
		Options Options `json:"options"`
		Plan    Plan    `json:"plan"`
	}{file, opt, p})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// notes says what plan does beyond its lines (Plan.Notes).
func notes(plan Plan, opt Options, n Node, unlisted int, lim peers.Limits) []string {
	out := []string{}
	if len(plan.Added) > 0 || len(plan.More) > 0 {
		out = append(out, fmt.Sprintf("once applied, the node tries these addresses when it is short of peers, after its own candidates and before registries (p2p.no_dial_imported stops that); imported addresses take %d of the %d places the book keeps for them (of %d), and poon peer remove -imported takes them all out", unlisted, lim.MaxUnlisted, lim.MaxRecords))
	}
	ranges, v4 := 0, map[string]bool{}
	for _, l := range plan.Blocks {
		if r, ok := strings.CutPrefix(l.Addr, "range "); ok {
			ranges++
			if strings.HasPrefix(r, "v4:") {
				v4[r] = true
			}
		}
	}
	if blocks := len(plan.Blocks); blocks > 0 {
		line := fmt.Sprintf("%d %s", blocks, plural(blocks, "block", "blocks"))
		if ranges > 0 {
			line += fmt.Sprintf(", %d of them network %s", ranges, plural(ranges, "range", "ranges"))
		}
		if len(v4) > 0 {
			line += fmt.Sprintf("; the IPv4 ranges cover %.2f%% of IPv4", float64(len(v4))*100/65536)
		}
		out = append(out, line)
	}
	if opt.Blocks {
		for _, a := range n.Listed {
			if strings.HasPrefix(peers.Range(a), "host:") {
				out = append(out, "a listed host name is protected by its name only: a block on the address it resolves to is not seen here")
				break
			}
		}
	}
	return out
}
