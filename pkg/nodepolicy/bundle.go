package nodepolicy

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

// A bundle is one shareable file of node policies (decision D149): a
// series to follow, one node policy, or every node policy a node keeps,
// for sharing, for running the same policies on several machines and for
// backups. Importing one merges it into what the node already has and
// shows the plan first (Merge). Nothing in a bundle changes what is valid:
// it holds only node policy, and goes through the same checks as any other
// change to node.json.
//
//	{
//	  "kind": "poo-node-policies",
//	  "format": 1,
//	  "scope": "series" | "policy" | "node" | "settings",
//	  "exported": "2026-11-02T21:00:00Z",
//	  "full": false,
//	  "policies": [{"name": "default", "current": true, "policy": {node.json}}],
//	  "settings": reserved for the node's settings, a later format
//	}

// BundleKind marks a bundle file.
const BundleKind = "poo-node-policies"

// BundleFormat is the version of the bundle layout this code reads and
// writes.
const BundleFormat = 1

// The scopes of a bundle.
const (
	// ScopeSeries holds one series, how it is followed and its issuer:
	// importing it touches nothing else.
	ScopeSeries = "series"
	// ScopePolicy holds one node policy.
	ScopePolicy = "policy"
	// ScopeNode holds every saved node policy of a node, each by name.
	ScopeNode = "node"
	// ScopeSettings holds one node policy's settings and nothing it
	// follows: how the node runs, to carry to another machine, back up
	// or share (D155). poo settings export writes it, poo settings
	// import reads it (ExportSettings, MergeOptions.SettingsOnly).
	ScopeSettings = "settings"
)

// MaxBundleSize bounds the bundle file a node reads.
const MaxBundleSize = 16 << 20

// MaxBundlePolicies bounds the node policies one bundle holds.
const MaxBundlePolicies = 256

// Bundle is one shareable file of node policies.
type Bundle struct {
	Kind     string `json:"kind"`
	Format   int    `json:"format"`
	Scope    string `json:"scope"`
	Exported string `json:"exported,omitempty"`
	// Full says machine-local settings were kept (MachineLocal), for a
	// backup or a clone of this machine.
	Full     bool           `json:"full,omitempty"`
	Policies []BundlePolicy `json:"policies"`
	// Settings is reserved for settings kept outside node.json, which a
	// later format may add. This version writes none and reads none: the
	// settings in node.json go in a settings bundle (ScopeSettings).
	Settings json.RawMessage `json:"settings,omitempty"`
	// Digest is the SHA-256 of the file as read (hex), set by
	// ReadBundle, so an import can be tied to the file that was
	// previewed (GUI-023, D151). It is not written.
	Digest string `json:"-"`
}

// BundlePolicy is one node policy in a bundle.
type BundlePolicy struct {
	// Name is the node policy's name (Manager); empty in a series
	// bundle.
	Name string `json:"name,omitempty"`
	// Current marks the node policy the exporting node used.
	Current bool   `json:"current,omitempty"`
	Policy  Policy `json:"policy"`
}

// NewBundle starts a bundle of the given scope.
func NewBundle(scope string, full bool, now time.Time) Bundle {
	return Bundle{Kind: BundleKind, Format: BundleFormat, Scope: scope, Full: full,
		Exported: now.UTC().Format(time.RFC3339)}
}

// MachineLocal lists the settings that describe one machine rather than
// what a node follows: where its RPC and peer port listen, the address it
// advertises, whether it syncs its disk writes, and the device and threads
// it grinds with. An export leaves them out unless it is full, and an
// import takes them only when asked (MergeOptions.Full), so a shared file
// never opens someone's control interface or picks their GPU.
var MachineLocal = []string{"rpc", "no_sync", "p2p.listen", "p2p.advertise", "grind.device", "grind.threads", "grind.jobs.device", "grind.jobs.threads"}

// Strip returns p without its machine-local settings (MachineLocal), and
// without the settings no longer used (p2p.fast_catch_up and
// p2p.depth_limit, D116).
func Strip(p Policy) Policy {
	p = clone(p)
	p.RPC = RPC{}
	p.NoSync = false
	p.P2P.Listen, p.P2P.Advertise = "", ""
	p.P2P.FastCatchUp, p.P2P.DepthLimit = false, 0
	if p.Grind != nil {
		p.Grind.Device, p.Grind.Threads = "", 0
		for i := range p.Grind.Jobs {
			p.Grind.Jobs[i].Device, p.Grind.Jobs[i].Threads = "", 0
		}
		if p.Grind.IsZero() {
			p.Grind = nil
		}
	}
	return p
}

// withMachine returns p with the machine-local settings of m.
func withMachine(p, m Policy) Policy {
	p = clone(p)
	p.RPC = m.RPC
	p.NoSync = m.NoSync
	p.P2P.Listen, p.P2P.Advertise = m.P2P.Listen, m.P2P.Advertise
	if m.Grind != nil && (m.Grind.Device != "" || m.Grind.Threads != 0) {
		if p.Grind == nil {
			p.Grind = &Grind{}
		}
		p.Grind.Device, p.Grind.Threads = m.Grind.Device, m.Grind.Threads
	}
	return p
}

// ExportPolicy returns p as it goes into a bundle: stripped of
// machine-local settings and of what is personal (Personal) unless full,
// and always of unused ones.
func ExportPolicy(p Policy, full bool) Policy {
	if full {
		p = clone(p)
		p.P2P.FastCatchUp, p.P2P.DepthLimit = false, 0
		return p
	}
	return withoutPersonal(Strip(p))
}

// Personal names what an export leaves out unless it is full, because it
// says something about the owner or opens a door rather than how the node
// runs (RED-052, D156): a registry that is a local file (its path names
// the user and the machine's folders), a registry URL with a user name,
// password, query or fragment (a password or key), and a grind job on one
// payment or entry of the owner's (tx:TID or an entry hash, and its
// until_from). Peers and tracking.peers are exported, as a shared setup
// is made of them, and ExportNotes says how many the file holds.
var Personal = []string{"p2p.registries (local files, and URLs with a user, password, query or fragment)", "grind.jobs (on one payment or entry)"}

// personalRegistry reports whether an export leaves registry r out
// unless full (Personal).
func personalRegistry(r string) bool {
	if !isURL(r) {
		return true
	}
	u, err := url.Parse(r)
	return err != nil || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != ""
}

// personalJob reports whether an export leaves grind job j out unless
// full: one on a payment or an entry, which names the owner's own.
func personalJob(j GrindJob) bool {
	return j.Target != "tip" && j.Target != "pending"
}

// withoutPersonal returns p without what Personal names.
func withoutPersonal(p Policy) Policy {
	p = clone(p)
	var regs []string
	for _, r := range p.P2P.Registries {
		if !personalRegistry(r) {
			regs = append(regs, r)
		}
	}
	p.P2P.Registries = regs
	if p.Grind != nil {
		var jobs []GrindJob
		for _, j := range p.Grind.Jobs {
			if !personalJob(j) {
				jobs = append(jobs, j)
			}
		}
		p.Grind.Jobs = jobs
		if p.Grind.IsZero() {
			p.Grind = nil
		}
	}
	return p
}

// ExportNotes says what an export of p holds and left out, for the
// person about to share it (RED-052, D156): the registries and grind jobs
// Personal left out, and the addresses it holds (p2p.peers,
// tracking.peers, registries), which whoever reads the file sees.
func ExportNotes(p Policy, full bool) []string {
	out := []string{}
	if !full {
		regs, jobs := 0, 0
		for _, r := range p.P2P.Registries {
			if personalRegistry(r) {
				regs++
			}
		}
		if p.Grind != nil {
			for _, j := range p.Grind.Jobs {
				if personalJob(j) {
					jobs++
				}
			}
		}
		if regs > 0 {
			out = append(out, fmt.Sprintf("left out %d %s that %s a local file or carry a user, password, query or fragment (-full keeps them)", regs, plural(regs, "registry", "registries"), plural(regs, "is", "are")))
		}
		if jobs > 0 {
			out = append(out, fmt.Sprintf("left out %d grind %s on one payment or entry (-full keeps them)", jobs, plural(jobs, "job", "jobs")))
		}
	}
	e := ExportPolicy(p, full)
	var holds []string
	if n := len(e.P2P.Peers); n > 0 {
		holds = append(holds, fmt.Sprintf("%d %s (p2p.peers)", n, plural(n, "peer address", "peer addresses")))
	}
	if n := len(e.Tracking.Peers); n > 0 {
		holds = append(holds, fmt.Sprintf("%d tracked %s (tracking.peers)", n, plural(n, "peer", "peers")))
	}
	if n := len(e.P2P.Registries); n > 0 {
		holds = append(holds, fmt.Sprintf("%d %s (p2p.registries)", n, plural(n, "registry", "registries")))
	}
	if len(holds) > 0 {
		out = append(out, "the file holds "+strings.Join(holds, ", ")+": whoever reads it sees them")
	}
	return out
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// ExportSeries returns the part of p about one series: its information,
// how p follows it, and its issuer when p accepts that issuer by name.
func ExportSeries(p Policy, id hash.Hash) (Policy, error) {
	out := Policy{Format: Format}
	for _, si := range p.Series {
		s, _, err := si.Build()
		if err != nil || s.ID() != id {
			continue
		}
		out.Series = []SeriesInfo{si}
		if si.IssuerWitness != "" && si.Params.Issuer != nil {
			for _, a := range p.Issuers.Accept {
				if a == *si.Params.Issuer {
					out.Issuers.Accept = []hash.Hash{a}
				}
			}
		}
	}
	if out.Series == nil {
		return Policy{}, fmt.Errorf("%w: series %s is not listed in the node policy", ErrInvalid, id.Short())
	}
	for _, f := range p.Follow {
		if f.Series != nil && *f.Series == id {
			out.Follow = append(out.Follow, f)
		}
	}
	return clone(out), nil
}

// ErrBundle marks a file that is not a bundle this node can read.
var ErrBundle = errors.New("nodepolicy: not a node policy bundle")

// ReadBundle reads a bundle. A plain node.json is read too, as a bundle of
// one node policy with no name. Every node policy in it is checked as a
// node policy file is (Validate); fields this version does not know are
// refused rather than dropped, so nothing in the file is silently lost.
func ReadBundle(r io.Reader) (Bundle, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxBundleSize+1))
	if err != nil {
		return Bundle{}, err
	}
	if len(b) > MaxBundleSize {
		return Bundle{}, fmt.Errorf("%w: over %d MiB", ErrBundle, MaxBundleSize>>20)
	}
	var head struct {
		Kind   *string `json:"kind"`
		Format *int    `json:"format"`
	}
	if err := json.Unmarshal(b, &head); err != nil {
		return Bundle{}, fmt.Errorf("%w: %v", ErrBundle, err)
	}
	if head.Kind == nil {
		// A node.json on its own.
		var p Policy
		if err := strictJSON(b, &p); err != nil {
			return Bundle{}, fmt.Errorf("%w: neither a bundle nor a node policy: %v", ErrBundle, err)
		}
		bd := Bundle{Kind: BundleKind, Format: BundleFormat, Scope: ScopePolicy, Policies: []BundlePolicy{{Policy: p}}, Digest: digest(b)}
		return bd, bd.check()
	}
	if *head.Kind != BundleKind {
		return Bundle{}, fmt.Errorf("%w: kind %q, want %q", ErrBundle, *head.Kind, BundleKind)
	}
	if head.Format == nil || *head.Format != BundleFormat {
		f := 0
		if head.Format != nil {
			f = *head.Format
		}
		return Bundle{}, fmt.Errorf("%w: format %d; this node reads format %d: a newer poo made it", ErrBundle, f, BundleFormat)
	}
	var bd Bundle
	if err := strictJSON(b, &bd); err != nil {
		return Bundle{}, fmt.Errorf("%w: %v", ErrBundle, err)
	}
	bd.Digest = digest(b)
	return bd, bd.check()
}

func strictJSON(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("more than one JSON value")
	}
	return nil
}

// HasSettings reports whether the bundle carries settings, which this
// version does not import.
func (b Bundle) HasSettings() bool {
	s := strings.TrimSpace(string(b.Settings))
	return s != "" && s != "null"
}

// check checks a bundle read from a file.
func (b *Bundle) check() error {
	switch b.Scope {
	case ScopeSeries, ScopePolicy, ScopeNode, ScopeSettings:
	default:
		return fmt.Errorf("%w: scope %q is not series, policy, node or settings", ErrBundle, b.Scope)
	}
	if len(b.Policies) == 0 {
		return fmt.Errorf("%w: it holds no node policy", ErrBundle)
	}
	if len(b.Policies) > MaxBundlePolicies {
		return fmt.Errorf("%w: over %d node policies", ErrBundle, MaxBundlePolicies)
	}
	if b.Scope != ScopeNode && len(b.Policies) != 1 {
		return fmt.Errorf("%w: a %s bundle holds one node policy, not %d", ErrBundle, b.Scope, len(b.Policies))
	}
	names := map[string]bool{}
	current := 0
	for i := range b.Policies {
		bp := &b.Policies[i]
		if b.Scope == ScopeNode || bp.Name != "" {
			if err := checkNewName(bp.Name); err != nil {
				return fmt.Errorf("%w: node policy %d: %v", ErrBundle, i, err)
			}
		}
		if names[bp.Name] {
			return fmt.Errorf("%w: node policy %q is in it twice", ErrBundle, bp.Name)
		}
		names[bp.Name] = true
		if bp.Current {
			current++
		}
		bp.Policy.Lowered = bp.Policy.lower()
		if err := bp.Policy.Validate(); err != nil {
			return fmt.Errorf("%w: node policy %q: %v", ErrBundle, bp.Name, err)
		}
		for j, si := range bp.Policy.Series {
			if _, err := series.New(si.Params); err != nil {
				return fmt.Errorf("%w: node policy %q: series %d: %v", ErrBundle, bp.Name, j, err)
			}
		}
		if b.Scope == ScopeSeries && len(bp.Policy.Series) != 1 {
			return fmt.Errorf("%w: a series bundle holds one series", ErrBundle)
		}
		if b.Scope == ScopeSettings && !isSettingsOnly(bp.Policy) {
			return fmt.Errorf("%w: a settings bundle holds no series, follows or accepted issuers", ErrBundle)
		}
	}
	if current > 1 {
		return fmt.Errorf("%w: more than one node policy is marked current", ErrBundle)
	}
	return nil
}

// Write writes the bundle as indented JSON.
func (b Bundle) Write(w io.Writer) error {
	out, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(out, '\n'))
	return err
}

// MergeOptions choose how an imported node policy meets the one a node
// has.
type MergeOptions struct {
	// PreferFile takes the file's value where both set one and they
	// differ; by default the node keeps its own and the plan lists it.
	PreferFile bool `json:"prefer_file,omitempty"`
	// Replace puts the file's node policy in place of the node's, keeping
	// only the node's machine-local settings (unless Full).
	Replace bool `json:"replace,omitempty"`
	// Full takes the file's machine-local settings too (MachineLocal):
	// for restoring a backup on the same machine. It takes the node
	// settings too, as Settings does.
	Full bool `json:"full,omitempty"`
	// Settings takes the file's node settings (Followed lists what is not
	// one): peers, registries, grind jobs, tracking, the p2p switches and
	// limits. By default an import takes only what to follow, so a file
	// from someone else never turns off a defence of this node, points it
	// at an address or starts work on it unasked (RED-043, D150).
	Settings bool `json:"settings,omitempty"`
	// SeriesOnly takes only the series, the follow list and the accepted
	// issuers: how a series bundle is imported.
	SeriesOnly bool `json:"series_only,omitempty"`
	// SettingsOnly takes only the node settings, never what to follow:
	// how poo settings import works (D155). It implies Settings. With
	// Replace the file's settings take the place of the node's, and what
	// the node follows stays its own.
	SettingsOnly bool `json:"settings_only,omitempty"`
	// MapPaths rewrites the file's local paths (registry files) before
	// anything else, so a file from another machine points at this one's
	// (D155). The first entry whose From starts a path is used.
	MapPaths []PathMap `json:"map_paths,omitempty"`
}

// Change is one line of a plan. Field names the setting as in node.json,
// with a list item in brackets: series[1a2b3c4d], follow[series 1a2b3c4d],
// p2p.peers[host:port]. Local and File are the values, as JSON.
type Change struct {
	Field string `json:"field"`
	Local string `json:"local,omitempty"`
	File  string `json:"file,omitempty"`
	// Took, on a conflict, is "local" or "file".
	Took string `json:"took,omitempty"`
}

// Plan says what an import does to one node policy.
type Plan struct {
	// New is set when the node policy does not exist yet and the import
	// creates it.
	New bool `json:"new,omitempty"`
	// Added, Removed and Changed compare the node policy before and
	// after.
	Added   []Change `json:"added"`
	Removed []Change `json:"removed"`
	Changed []Change `json:"changed"`
	// Conflicts are settings both set to different values, and which was
	// taken.
	Conflicts []Change `json:"conflicts"`
	// Skipped are machine-local settings the file holds and the import
	// left alone (MergeOptions.Full takes them).
	Skipped []string `json:"skipped"`
	// SkippedSettings are node settings the file holds and the import
	// left alone (MergeOptions.Settings takes them), and registries that
	// are local files (only Full takes them).
	SkippedSettings []string `json:"skipped_settings"`
	// Defences are the lines of this plan to read first: with Settings,
	// every node setting it changes but the cosmetic ones (RED-048), and
	// a whole-series follow of LargeFollow pools or more (RED-046).
	Defences []string `json:"defences"`
	// Follows says, for each series whose follow the import adds to or
	// changes, how many pools it has and how many the result follows.
	Follows []FollowCount `json:"follows"`
	// Same counts the file's settings the node already has.
	Same int `json:"same"`
	// Refused are series the result lists but would not accept, and why
	// (Policy.Accepted): the import still applies, but they stay unused
	// until their issuer is accepted.
	Refused []string `json:"refused"`
	// Declined are series the file lists that the import left out, by
	// short id and why: the node policy's series_ceiling declines them
	// (D170). Their follow rules are left out too.
	Declined []string `json:"declined,omitempty"`
	// NowUsed are series local lists but does not use (Refused) that the
	// result uses, by short id: an issuer setting turned them on (RED-056,
	// D156). They are named in Defences too.
	NowUsed []string `json:"now_used"`
	// DroppedReleases are releases of a variable series the file held
	// that do not check, and why: the import leaves them out (or, for a
	// series local already lists, the node does not use them), and the
	// series is followed with the parts proven without them (RED-073).
	DroppedReleases []string `json:"dropped_releases,omitempty"`
	// Fit lists what the import brings in that may not fit this machine:
	// a device or thread count it lacks, a file it does not have, an
	// address that is not its own (Fit, D155). The caller fills it, as
	// only it knows the machine. It refuses nothing.
	Fit []string `json:"fit"`
}

// Empty reports whether the import changes nothing. A conflict where the
// file's value was taken is a change: it is listed with the conflicts, not
// in Changed, and was not counted, so a plan made of such conflicts alone
// said "took the file's" and saved nothing (RED-054, D156).
func (p Plan) Empty() bool {
	if p.New || len(p.Added) > 0 || len(p.Removed) > 0 || len(p.Changed) > 0 {
		return false
	}
	for _, c := range p.Conflicts {
		if c.Took == "file" {
			return false
		}
	}
	return true
}

// Merge brings the node policy file, from a bundle, into local and says
// what it did. By default it adds only what to follow (Followed) that
// local lacks: series, followed issuers, series, pools and mints (merged
// as AddFollow does) and accepted issuers; the file's node settings are
// listed in Plan.SkippedSettings and left alone (RED-043, D150). With
// Settings it also adds peers, registries (http and https only, unless
// Full), grind jobs and every single setting local leaves unset, and names
// the lines that touch a defence first (Plan.Defences). Where both set a
// value and they differ, local's is kept and listed as a conflict, unless
// PreferFile. Replace takes the file's node policy whole, or without
// Settings only what it follows. Machine-local settings are left alone
// unless Full. The result is valid (Validate) or Merge fails and local is
// unchanged.
func Merge(local, file Policy, opt MergeOptions) (Policy, Plan, error) {
	plan := Plan{Added: []Change{}, Removed: []Change{}, Changed: []Change{}, Conflicts: []Change{}, Skipped: []string{}, SkippedSettings: []string{}, Defences: []string{}, Refused: []string{}, NowUsed: []string{}, Fit: []string{}}
	if opt.SeriesOnly && opt.SettingsOnly {
		return Policy{}, Plan{}, fmt.Errorf("%w: an import takes the series only or the settings only, not both", ErrInvalid)
	}
	local, file = clone(local), clone(file)
	file.P2P.FastCatchUp, file.P2P.DepthLimit = false, 0
	file, mapped := mapPaths(file, opt.MapPaths)
	if opt.SeriesOnly {
		file = followed(file)
	}
	if opt.SettingsOnly {
		file = settingsPart(file)
	}
	settings := opt.Settings || opt.Full || opt.SettingsOnly
	if !opt.Full {
		plan.Skipped = machineSet(file)
		file = Strip(file)
		var keep []string
		for _, r := range file.P2P.Registries {
			if isURL(r) || mapped[r] {
				// A registry file -map-path pointed at this machine is
				// taken without the rest of -full (RED-055, D156).
				keep = append(keep, r)
			} else if settings {
				plan.SkippedSettings = append(plan.SkippedSettings, "p2p.registries: "+r+" (a local file: -map-path OLD=NEW or -full takes it)")
			}
		}
		file.P2P.Registries = keep
	}
	if !settings {
		plan.SkippedSettings = append(plan.SkippedSettings, settingsSet(file, local)...)
		file = followed(file)
	}
	var res Policy
	var err error
	switch {
	case opt.Replace && !settings:
		// What to follow is the file's; how the node runs stays its own.
		res = clone(local)
		res.Series, res.Follow, res.Issuers.Accept = file.Series, file.Follow, file.Issuers.Accept
		res.Format = Format
	case opt.Replace:
		res = file
		if !opt.Full {
			res = withMachine(res, local)
		}
		if opt.SettingsOnly {
			// How the node runs is the file's; what it follows stays its
			// own.
			res.Series, res.Follow, res.Issuers.Accept = local.Series, local.Follow, local.Issuers.Accept
		}
		res.Format = Format
	default:
		res, err = mergeInto(local, file, opt, settings, &plan)
		if err != nil {
			return Policy{}, Plan{}, err
		}
	}
	res.Series, plan.DroppedReleases = withoutBadReleases(res.Series)
	if err := res.Validate(); err != nil {
		return Policy{}, Plan{}, err
	}
	before, after, fromFile := flatAll(local), flatAll(res), flatAll(file)
	for _, k := range sortedKeys(after) {
		if v, ok := before[k]; !ok {
			plan.Added = append(plan.Added, Change{Field: show(k), File: after[k]})
		} else if v != after[k] && !conflicted(plan.Conflicts, show(k)) {
			plan.Changed = append(plan.Changed, Change{Field: show(k), Local: v, File: after[k]})
		}
	}
	for _, k := range sortedKeys(before) {
		if _, ok := after[k]; !ok {
			plan.Removed = append(plan.Removed, Change{Field: show(k), Local: before[k]})
		}
	}
	for k, v := range fromFile {
		if before[k] == v {
			plan.Same++
		}
	}
	if settings {
		for _, cs := range [][]Change{plan.Added, plan.Changed, plan.Removed} {
			for _, c := range cs {
				if isDefence(c.Field) {
					plan.Defences = append(plan.Defences, c.Field)
				}
			}
		}
		for _, c := range plan.Conflicts {
			if c.Took == "file" && isDefence(c.Field) {
				plan.Defences = append(plan.Defences, c.Field)
			}
		}
	}
	// How much of each series whose follow changed the result follows,
	// and a large whole-series follow named first (RED-046).
	touched := map[string]bool{}
	for _, cs := range [][]Change{plan.Added, plan.Changed} {
		for _, c := range cs {
			if id, ok := strings.CutPrefix(c.Field, "follow.[series "); ok {
				touched[strings.TrimSuffix(id, "]")] = true
			}
		}
	}
	plan.Follows = []FollowCount{}
	if len(touched) > 0 {
		for _, si := range res.Series {
			s, _, err := si.Build()
			if err != nil || !touched[s.ID().Short()] {
				continue
			}
			fc := FollowCount{Series: s.ID().Short(), Pools: s.PoolCount()}
			pools := map[uint64]bool{}
			for _, f := range res.Follow {
				if f.Series == nil || *f.Series != s.ID() {
					continue
				}
				if len(f.Pools) == 0 && len(f.Mints) == 0 {
					fc.Whole = true
					fc.Except += uint64(len(f.Except))
				}
				for _, k := range f.Pools {
					pools[k] = true
				}
				for _, m := range f.Mints {
					pools[s.PoolOf(m)] = true
				}
			}
			fc.Followed = uint64(len(pools))
			if fc.Whole {
				fc.Followed = fc.Pools - min(fc.Except, fc.Pools)
			}
			plan.Follows = append(plan.Follows, fc)
			if fc.Whole && fc.Followed >= LargeFollow {
				plan.Defences = append(plan.Defences, fmt.Sprintf("follow.[series %s] (%d of %d pools)", fc.Series, fc.Followed, fc.Pools))
			}
		}
	}
	accepted, refused := res.Accepted()
	// Series local lists but refused, and the result uses: a setting such
	// as issuers.any can turn them on without touching what is followed
	// (RED-056, D156). They are named first.
	if _, was := local.Accepted(); len(was) > 0 {
		wasRefused := map[hash.Hash]bool{}
		for _, r := range was {
			if s, err := series.New(local.Series[r.Index].Params); err == nil {
				wasRefused[s.ID()] = true
			}
		}
		for _, a := range accepted {
			if id := a.Series.ID(); wasRefused[id] {
				plan.NowUsed = append(plan.NowUsed, id.Short())
			}
		}
		if len(plan.NowUsed) > 0 {
			plan.Defences = append(plan.Defences, "now used: series "+strings.Join(plan.NowUsed, ", "))
		}
	}
	for _, r := range refused {
		id := "?"
		if s, err := series.New(res.Series[r.Index].Params); err == nil {
			id = s.ID().Short()
		}
		plan.Refused = append(plan.Refused, fmt.Sprintf("series %s: %v", id, r.Reason))
	}
	return res, plan, nil
}

func conflicted(cs []Change, field string) bool {
	for _, c := range cs {
		if c.Field == field {
			return true
		}
	}
	return false
}

// Followed lists the parts of a node policy that say what it follows: its
// series, its follow list and the issuers it accepts by name. A default
// import takes only these; everything else is a node setting, which an
// import takes only when asked (MergeOptions.Settings, RED-043, D150). A
// setting added to node.json later is a node setting unless it is put
// here, so a new switch is never taken from someone else's file unasked.
var Followed = []string{"series", "follow", "issuers.accept"}

// Cosmetic lists the node settings that change only how the node reports,
// never what it does. Every other node setting an import changes is named
// in Plan.Defences, so a setting added to node.json later is named until
// someone shows it is cosmetic and puts it here (RED-048, D152).
var Cosmetic = []string{"log.format"}

// LargeFollow is the number of pools from which a whole-series follow an
// import adds is named in Plan.Defences: the node keeps a little for
// every pool it follows (RED-046, D152). It refuses nothing.
const LargeFollow = 1 << 16

// FollowCount says how much of a series the result of an import
// follows.
type FollowCount struct {
	Series   string `json:"series"`
	Pools    uint64 `json:"pools"`
	Followed uint64 `json:"followed"`
	Whole    bool   `json:"whole,omitempty"`
	// Except counts the pools a whole follow leaves out.
	Except uint64 `json:"except,omitempty"`
}

// followed returns the parts of p that Followed names.
func followed(p Policy) Policy {
	return Policy{Format: Format, Series: p.Series, Follow: p.Follow, Issuers: Issuers{Accept: p.Issuers.Accept}}
}

// settingsSet lists the node settings p sets that followed would drop
// and local does not already have.
func settingsSet(p, local Policy) []string {
	out := []string{}
	keep, have := flatAll(followed(p)), flatAll(local)
	flat := flatAll(p)
	for _, k := range sortedKeys(flat) {
		if v, ok := have[k]; ok && v == flat[k] {
			continue
		}
		if v, ok := keep[k]; !ok || v != flat[k] {
			out = append(out, show(k))
		}
	}
	return out
}

// isDefence reports whether a plan's field is to be read first: every
// node setting but the cosmetic ones (Cosmetic). It used to be a list of
// the settings thought of as defences, which let the others pass unseen
// (RED-048, D152).
func isDefence(field string) bool {
	return !under(field, Followed) && !under(field, Cosmetic)
}

// under reports whether field is one of list, or an item or part of one.
func under(field string, list []string) bool {
	for _, d := range list {
		if field == d || strings.HasPrefix(field, d+".") || strings.HasPrefix(field, d+"[") {
			return true
		}
	}
	return false
}

func isURL(loc string) bool {
	return strings.HasPrefix(loc, "http://") || strings.HasPrefix(loc, "https://")
}

// machineSet lists the machine-local settings p sets.
func machineSet(p Policy) []string {
	out := []string{}
	flat := flatAll(p)
	for _, k := range sortedKeys(flat) {
		if isMachine(k) {
			out = append(out, show(k))
		}
	}
	if p.Grind != nil {
		for _, j := range p.Grind.Jobs {
			if j.Device != "" || j.Threads != 0 {
				out = append(out, "grind.jobs.device/threads")
				break
			}
		}
	}
	return out
}

func isMachine(k string) bool {
	parts := strings.Split(k, sep)
	for _, m := range MachineLocal {
		mp := strings.Split(m, ".")
		if len(parts) >= len(mp) && strings.Join(parts[:len(mp)], ".") == m {
			return true
		}
	}
	return false
}

// withoutBadReleases returns list with every kept release that does not
// check left out, and why: an import never saves a release the node would
// not use (RED-073). The series stay, followed with the parts proven
// without it. list itself is not changed.
func withoutBadReleases(list []SeriesInfo) ([]SeriesInfo, []string) {
	var out []SeriesInfo
	var dropped []string
	for i, si := range list {
		s, relErr, err := si.Build()
		if err != nil || relErr == nil {
			continue
		}
		if out == nil {
			out = append([]SeriesInfo(nil), list...)
		}
		out[i].Release = nil
		dropped = append(dropped, fmt.Sprintf("series %s: %v", s.ID().Short(), relErr))
	}
	if out == nil {
		return list, nil
	}
	return out, dropped
}

// mergeInto is Merge without Replace.
func mergeInto(local, file Policy, opt MergeOptions, settings bool, plan *Plan) (Policy, error) {
	// Single settings, compared one by one.
	lm, err := toMap(local)
	if err != nil {
		return Policy{}, err
	}
	fm, err := toMap(file)
	if err != nil {
		return Policy{}, err
	}
	lf, ff := map[string]any{}, map[string]any{}
	flatten(lm, nil, lf)
	if settings && !opt.SeriesOnly {
		flatten(fm, nil, ff)
	}
	for _, k := range sortedKeys(ff) {
		fv := ff[k]
		lv, ok := lf[k]
		switch {
		case !ok:
			setPath(lm, strings.Split(k, sep), fv)
		case jsonOf(lv) == jsonOf(fv):
		case opt.PreferFile:
			plan.Conflicts = append(plan.Conflicts, Change{Field: show(k), Local: jsonOf(lv), File: jsonOf(fv), Took: "file"})
			setPath(lm, strings.Split(k, sep), fv)
		default:
			plan.Conflicts = append(plan.Conflicts, Change{Field: show(k), Local: jsonOf(lv), File: jsonOf(fv), Took: "local"})
		}
	}
	var res Policy
	b, err := json.Marshal(lm)
	if err != nil {
		return Policy{}, err
	}
	if err := json.Unmarshal(b, &res); err != nil {
		return Policy{}, err
	}
	res.Format = Format

	// Lists, merged item by item.
	have := map[hash.Hash]int{}
	built := map[hash.Hash]*series.Series{}
	for i, si := range res.Series {
		if s, _, err := si.Build(); err == nil {
			if _, ok := have[s.ID()]; !ok {
				have[s.ID()], built[s.ID()] = i, s
			}
		}
	}
	declined := map[hash.Hash]bool{}
	for _, si := range file.Series {
		s, _, err := si.Build()
		if err != nil {
			return Policy{}, fmt.Errorf("%w: series in the file: %v", ErrInvalid, err)
		}
		i, ok := have[s.ID()]
		if !ok {
			if err := res.DeclinesSeries(si.Params); err != nil {
				declined[s.ID()] = true
				plan.Declined = append(plan.Declined, s.ID().Short()+": "+err.Error())
				continue
			}
		}
		switch {
		case !ok:
			built[s.ID()] = s
			have[s.ID()] = len(res.Series)
			res.Series = append(res.Series, si)
		case si.IssuerWitness == "" || si.IssuerWitness == res.Series[i].IssuerWitness:
		case res.Series[i].IssuerWitness == "":
			res.Series[i].IssuerWitness = si.IssuerWitness
		default:
			field := "series[" + s.ID().Short() + "].issuer_witness"
			took := "local"
			if opt.PreferFile {
				took = "file"
				res.Series[i].IssuerWitness = si.IssuerWitness
			}
			plan.Conflicts = append(plan.Conflicts, Change{Field: field, Local: quote(res.Series[i].IssuerWitness), File: quote(si.IssuerWitness), Took: took})
		}
	}
	look := func(id hash.Hash) (*series.Series, error) {
		if s, ok := built[id]; ok {
			return s, nil
		}
		return nil, fmt.Errorf("%w: series %s is not listed in the node policy", ErrInvalid, id.Short())
	}
	follows := file.Follow
	if len(declined) > 0 {
		follows = nil
		for _, f := range file.Follow {
			if f.Series == nil || !declined[*f.Series] {
				follows = append(follows, f)
			}
		}
	}
	if err := res.addFollows(follows, look); err != nil {
		return Policy{}, fmt.Errorf("follow in the file: %w", err)
	}
	res.Issuers.Accept = unionHashes(res.Issuers.Accept, file.Issuers.Accept)
	res.P2P.Peers = unionStrings(res.P2P.Peers, file.P2P.Peers)
	res.P2P.Registries = unionStrings(res.P2P.Registries, file.P2P.Registries)
	if file.Grind != nil && len(file.Grind.Jobs) > 0 {
		if res.Grind == nil {
			res.Grind = &Grind{}
		}
	next:
		for _, j := range file.Grind.Jobs {
			for _, k := range res.Grind.Jobs {
				if SameJob(j, k) {
					continue next
				}
			}
			res.Grind.Jobs = append(res.Grind.Jobs, j)
		}
	}
	return res, nil
}

// sep joins the parts of a setting's path inside this file; show turns it
// into the dotted form.
const sep = "\x00"

func show(k string) string { return strings.ReplaceAll(k, sep, ".") }

// listed are the lists merged item by item, not compared as one value.
var listed = map[string]bool{
	"series": true, "follow": true, "issuers" + sep + "accept": true,
	"p2p" + sep + "peers": true, "p2p" + sep + "registries": true, "grind" + sep + "jobs": true,
}

// whole are the objects compared as one value: a custom tracking template
// and the weights are each one choice, not a set of separate settings.
func whole(path []string) bool {
	return (len(path) == 3 && path[0] == "tracking" && path[1] == "custom") ||
		(len(path) == 2 && path[0] == "tracking" && path[1] == "weights")
}

// flatten collects the single settings of m by path, leaving out format
// and the listed lists.
func flatten(m map[string]any, path []string, out map[string]any) {
	for k, v := range m {
		p := append(append([]string(nil), path...), k)
		key := strings.Join(p, sep)
		if (len(path) == 0 && k == "format") || listed[key] {
			continue
		}
		if sub, ok := v.(map[string]any); ok && !whole(p) {
			flatten(sub, p, out)
			continue
		}
		out[key] = v
	}
}

// flatAll is every setting of p by path, each listed item its own entry,
// as JSON: what a plan compares.
func flatAll(p Policy) map[string]string {
	out := map[string]string{}
	m, err := toMap(p)
	if err != nil {
		return out
	}
	single := map[string]any{}
	flatten(m, nil, single)
	for k, v := range single {
		out[k] = jsonOf(v)
	}
	for _, si := range p.Series {
		if s, err := series.New(si.Params); err == nil {
			out["series"+sep+"["+s.ID().Short()+"]"] = jsonOf(si)
		}
	}
	for _, f := range p.Follow {
		switch {
		case f.Issuer != nil:
			out["follow"+sep+"[issuer "+f.Issuer.Short()+"]"] = "true"
		case f.Series != nil:
			v := map[string]any{"pools": f.Pools, "mints": f.Mints}
			if len(f.Except) > 0 {
				v["except"] = f.Except
			}
			out["follow"+sep+"[series "+f.Series.Short()+"]"] = jsonOf(v)
		}
	}
	for _, a := range p.Issuers.Accept {
		out["issuers"+sep+"accept"+sep+"["+a.Short()+"]"] = "true"
	}
	for _, s := range p.P2P.Peers {
		out["p2p"+sep+"peers"+sep+"["+s+"]"] = "true"
	}
	for _, s := range p.P2P.Registries {
		out["p2p"+sep+"registries"+sep+"["+s+"]"] = "true"
	}
	if p.Grind != nil {
		for _, j := range p.Grind.Jobs {
			out["grind"+sep+"jobs"+sep+"["+jsonOf(j)+"]"] = "true"
		}
	}
	return out
}

// setPath sets a single setting in m, making the objects on the way.
func setPath(m map[string]any, path []string, v any) {
	for _, k := range path[:len(path)-1] {
		sub, ok := m[k].(map[string]any)
		if !ok {
			sub = map[string]any{}
			m[k] = sub
		}
		m = sub
	}
	m[path[len(path)-1]] = v
}

func toMap(p Policy) (map[string]any, error) {
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var m map[string]any
	return m, dec.Decode(&m)
}

func jsonOf(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(b)
}

func quote(s string) string { return jsonOf(s) }

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func unionHashes(a, b []hash.Hash) []hash.Hash {
	seen := map[hash.Hash]bool{}
	var out []hash.Hash
	for _, h := range append(append([]hash.Hash(nil), a...), b...) {
		if !seen[h] {
			seen[h] = true
			out = append(out, h)
		}
	}
	return out
}

func unionStrings(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range append(append([]string(nil), a...), b...) {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// clone returns a deep copy of p, so a merge never changes its inputs.
func clone(p Policy) Policy {
	lowered := p.Lowered
	b, err := json.Marshal(p)
	if err != nil {
		return p
	}
	var q Policy
	if err := json.Unmarshal(b, &q); err != nil {
		return p
	}
	q.Lowered = lowered
	return q
}

func digest(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
