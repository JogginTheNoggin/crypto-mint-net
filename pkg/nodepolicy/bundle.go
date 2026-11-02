package nodepolicy

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
//	  "scope": "series" | "policy" | "node",
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
	// Settings is reserved for the node's own settings, which a later
	// format adds. This version writes none and reads none.
	Settings json.RawMessage `json:"settings,omitempty"`
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
// machine-local settings unless full, and always of unused ones.
func ExportPolicy(p Policy, full bool) Policy {
	if full {
		p = clone(p)
		p.P2P.FastCatchUp, p.P2P.DepthLimit = false, 0
		return p
	}
	return Strip(p)
}

// ExportSeries returns the part of p about one series: its information,
// how p follows it, and its issuer when p accepts that issuer by name.
func ExportSeries(p Policy, id hash.Hash) (Policy, error) {
	out := Policy{Format: Format}
	for _, si := range p.Series {
		s, err := series.New(si.Params)
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
		bd := Bundle{Kind: BundleKind, Format: BundleFormat, Scope: ScopePolicy, Policies: []BundlePolicy{{Policy: p}}}
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
	case ScopeSeries, ScopePolicy, ScopeNode:
	default:
		return fmt.Errorf("%w: scope %q is not series, policy or node", ErrBundle, b.Scope)
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
			if err := checkName(bp.Name); err != nil {
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
	// for restoring a backup on the same machine.
	Full bool `json:"full,omitempty"`
	// SeriesOnly takes only the series, the follow list and the accepted
	// issuers: how a series bundle is imported.
	SeriesOnly bool `json:"series_only,omitempty"`
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
	// Same counts the file's settings the node already has.
	Same int `json:"same"`
	// Refused are series the result lists but would not accept, and why
	// (Policy.Accepted): the import still applies, but they stay unused
	// until their issuer is accepted.
	Refused []string `json:"refused"`
}

// Empty reports whether the import changes nothing.
func (p Plan) Empty() bool {
	return !p.New && len(p.Added) == 0 && len(p.Removed) == 0 && len(p.Changed) == 0
}

// Merge brings the node policy file, from a bundle, into local and says
// what it did. By default it adds what local lacks: series, followed
// issuers, series, pools and mints (merged as AddFollow does), accepted
// issuers, peers, registries, grind jobs and every single setting local
// leaves unset. Where both set a value and they differ, local's is kept
// and listed as a conflict, unless PreferFile. Replace takes the file's
// node policy whole. Machine-local settings are left alone unless Full.
// The result is valid (Validate) or Merge fails and local is unchanged.
func Merge(local, file Policy, opt MergeOptions) (Policy, Plan, error) {
	plan := Plan{Added: []Change{}, Removed: []Change{}, Changed: []Change{}, Conflicts: []Change{}, Skipped: []string{}, Refused: []string{}}
	local, file = clone(local), clone(file)
	file.P2P.FastCatchUp, file.P2P.DepthLimit = false, 0
	if opt.SeriesOnly {
		file = Policy{Format: Format, Series: file.Series, Follow: file.Follow, Issuers: Issuers{Accept: file.Issuers.Accept}}
	}
	if !opt.Full {
		plan.Skipped = machineSet(file)
		file = Strip(file)
	}
	var res Policy
	var err error
	if opt.Replace {
		res = file
		if !opt.Full {
			res = withMachine(res, local)
		}
		res.Format = Format
	} else {
		res, err = mergeInto(local, file, opt, &plan)
		if err != nil {
			return Policy{}, Plan{}, err
		}
	}
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
	_, refused := res.Accepted()
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

// mergeInto is Merge without Replace.
func mergeInto(local, file Policy, opt MergeOptions, plan *Plan) (Policy, error) {
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
	if !opt.SeriesOnly {
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
	for i, si := range res.Series {
		if s, err := series.New(si.Params); err == nil {
			have[s.ID()] = i
		}
	}
	for _, si := range file.Series {
		s, err := series.New(si.Params)
		if err != nil {
			return Policy{}, fmt.Errorf("%w: series in the file: %v", ErrInvalid, err)
		}
		i, ok := have[s.ID()]
		switch {
		case !ok:
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
	for _, f := range file.Follow {
		if err := res.AddFollow(f); err != nil {
			return Policy{}, fmt.Errorf("follow in the file: %w", err)
		}
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
			out["follow"+sep+"[series "+f.Series.Short()+"]"] = jsonOf(map[string]any{"pools": f.Pools, "mints": f.Mints})
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
