package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/fsutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/grind"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/plain"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// policyExportCmd writes node policies to one shareable file, a bundle
// (nodepolicy.Bundle, D149): one node policy, one series of it, or with
// -all every saved node policy.
func policyExportCmd(args []string, out io.Writer) error {
	fs := newFlagSet("policy export", out)
	w := addWhere(fs)
	all := fs.Bool("all", false, "every saved node policy under -root, each by name: for a backup or another machine")
	seriesID := fs.String("series", "", "only this series (hex id): its information, how it is followed and its issuer")
	full := fs.Bool("full", false, "keep machine-local settings (rpc, no_sync, p2p listen and advertise, grind device and threads) and personal ones (registry files, registry URLs with a user, password or query, grind jobs on one payment or entry): for a backup or a clone of this machine")
	file := fs.String("out", "", "write the bundle to this file (default: print it)")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("poo policy export takes flags only")
	}
	m := w.manager()
	cur, _ := m.Current()
	var b nodepolicy.Bundle
	var notes []string
	now := time.Now()
	switch {
	case *all:
		if *seriesID != "" || *w.name != "" || *w.dir != "" {
			return errors.New("-all exports every node policy: leave out -series, -policy and -dir")
		}
		b = nodepolicy.NewBundle(nodepolicy.ScopeNode, *full, now)
		list, err := m.List()
		if err != nil {
			return err
		}
		for _, e := range list {
			if e.Archived {
				continue
			}
			p, err := m.Load(e.Name)
			if err != nil {
				return err
			}
			b.Policies = append(b.Policies, nodepolicy.BundlePolicy{Name: e.Name, Current: e.Name == cur, Policy: nodepolicy.ExportPolicy(p, *full)})
			notes = append(notes, exportNotes(e.Name, p, *full, true)...)
		}
		if len(b.Policies) == 0 {
			return fmt.Errorf("no saved node policy under %s", *w.root)
		}
	default:
		dir := w.resolve()
		p, err := nodepolicy.Load(filepath.Join(dir, nodepolicy.FileName))
		if err != nil {
			return err
		}
		name := ""
		if *w.dir == "" {
			name = w.policyName()
		}
		if *seriesID != "" {
			if *full {
				return errors.New("a series bundle holds no machine-local settings: leave out -full")
			}
			id, err := hash.Parse(*seriesID)
			if err != nil {
				return fmt.Errorf("-series: %w", err)
			}
			sp, err := nodepolicy.ExportSeries(p, id)
			if err != nil {
				return err
			}
			b = nodepolicy.NewBundle(nodepolicy.ScopeSeries, false, now)
			b.Policies = []nodepolicy.BundlePolicy{{Policy: sp}}
			break
		}
		b = nodepolicy.NewBundle(nodepolicy.ScopePolicy, *full, now)
		b.Policies = []nodepolicy.BundlePolicy{{Name: name, Current: name != "" && name == cur, Policy: nodepolicy.ExportPolicy(p, *full)}}
		notes = exportNotes(name, p, *full, false)
	}
	return writeBundle(out, b, *file, fmt.Sprintf("%d node %s (%s)", len(b.Policies), plural(len(b.Policies), "policy", "policies"), b.Scope), notes)
}

// writeBundle prints b, or writes it to file (readable by its owner only)
// and says what it wrote. notes, what the file holds and left out
// (nodepolicy.ExportNotes), follow on out after a file is written, and go
// to the invocation's Err when the bundle is printed, so the printed JSON
// stays whole (RED-052).
func writeBundle(out io.Writer, b nodepolicy.Bundle, file, what string, notes []string) error {
	if file == "" {
		if err := b.Write(out); err != nil {
			return err
		}
		for _, n := range notes {
			fmt.Fprintf(errOf(out), "note: %s\n", n)
		}
		return nil
	}
	var buf bytes.Buffer
	if err := b.Write(&buf); err != nil {
		return err
	}
	if err := fsutil.WriteFile(file, buf.Bytes(), 0o600); err != nil {
		return err
	}
	fmt.Fprintf(out, "exported %s to %s\n", what, file)
	for _, n := range notes {
		fmt.Fprintf(out, "  note: %s\n", n)
	}
	return nil
}

// exportNotes returns nodepolicy.ExportNotes for p, each line named after
// its node policy when a bundle holds more than one.
func exportNotes(name string, p nodepolicy.Policy, full, many bool) []string {
	ns := nodepolicy.ExportNotes(p, full)
	if many {
		for i := range ns {
			ns[i] = name + ": " + ns[i]
		}
	}
	return ns
}

// thisMachine describes this machine for an import's fit check
// (nodepolicy.Fit, D155); a test may replace it.
var thisMachine = func() nodepolicy.Machine { return grind.ThisMachine(grind.Default) }

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// importResult is what poo policy import did, or would do, to one node
// policy.
type importResult struct {
	Name    string `json:"name,omitempty"`
	Dir     string `json:"dir"`
	Running bool   `json:"running"`
	// Mode is create, merge or replace.
	Mode      string          `json:"mode"`
	Plan      nodepolicy.Plan `json:"plan"`
	Saved     bool            `json:"saved"`
	Reloading bool            `json:"reloading,omitempty"`
	Current   bool            `json:"made_current,omitempty"`
	// Digest is the SHA-256 of the file read, the options and every
	// plan, the same on every row: pass it back with -if-digest to apply
	// only that file with those options and that plan (GUI-023, RED-051).
	Digest string `json:"digest"`
}

// policyImportCmd reads a bundle (or a node.json) and merges it into the
// node policies here: it shows the plan, and changes them only with
// -apply. A running node takes the change at once.
func policyImportCmd(args []string, out io.Writer) error {
	return importBundleCmd(args, out, false)
}

// importBundleCmd is poo policy import, and with settingsOnly poo
// settings import (D155): the same reading, plan, digest and apply, the
// settings import taking only the node settings.
func importBundleCmd(args []string, out io.Writer, settingsOnly bool) error {
	cmd := "policy import"
	if settingsOnly {
		cmd = "settings import"
	}
	fs := newFlagSet(cmd, out)
	w := addWhere(fs)
	apply := fs.Bool("apply", false, "make the change (default: only show the plan)")
	prefer := fs.Bool("prefer-file", false, "where both set a value and they differ, take the file's (default: keep this node's)")
	settings := new(bool)
	var replace, full *bool
	if settingsOnly {
		replace = fs.Bool("replace", false, "put the file's settings in place of this node's instead of merging, keeping what it follows and this machine's machine-local settings")
		full = fs.Bool("full", false, "take the machine-local settings (rpc, no_sync, p2p listen and advertise, grind device and threads) and local registry files too: moving to a new machine or restoring a backup")
	} else {
		replace = fs.Bool("replace", false, "put the file's node policy in place of this one's instead of merging, keeping this machine's machine-local settings")
		settings = fs.Bool("settings", false, "take the file's node settings too (peers, registries, grind jobs, tracking, the p2p switches and limits); by default only what to follow is taken")
		full = fs.Bool("full", false, "take every setting: machine-local ones (rpc, no_sync, p2p listen and advertise, grind device and threads) and local registry files too: restoring a backup on this machine")
	}
	var maps stringList
	fs.Var(&maps, "map-path", "OLD=NEW: read a registry file path that starts with OLD as starting with NEW, for a file made on another machine (repeatable)")
	noReload := fs.Bool("no-reload", false, "change node.json only; a running node applies it on its next reload or start")
	asJSON := fs.Bool("json", false, "print the plan as JSON")
	ifDigest := fs.String("if-digest", "", "apply only if the import digest (hex, as the preview prints it: the file, the options and the plan) is this one: so what is applied is what was previewed")
	// The file may come before the flags: poo policy import FILE -apply.
	var file string
	if len(args) > 0 && (args[0] == "-" || !strings.HasPrefix(args[0], "-")) {
		file, args = args[0], args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if file == "" && fs.NArg() == 1 {
		file = fs.Arg(0)
	} else if fs.NArg() != 0 || file == "" {
		if settingsOnly {
			return errors.New("usage: poo settings import FILE|- [-apply] [-prefer-file] [-replace] [-full] [-map-path OLD=NEW]... [-if-digest D] [-policy NAME | -dir DIR]")
		}
		return errors.New("usage: poo policy import FILE|- [-apply] [-prefer-file] [-replace] [-settings] [-full] [-map-path OLD=NEW]... [-if-digest D] [-policy NAME | -dir DIR]")
	}
	var pathMaps []nodepolicy.PathMap
	for _, m := range maps {
		pm, err := nodepolicy.ParsePathMap(m)
		if err != nil {
			return fmt.Errorf("-map-path: %w", err)
		}
		pathMaps = append(pathMaps, pm)
	}
	if *apply && viewOnlyOf(out) {
		return errViewOnly(cmd + " -apply")
	}
	// A digest given empty is not a digest left out: a script passing an
	// unset variable must not apply the file unchecked (RED-047, D152).
	digestGiven := false
	fs.Visit(func(f *flag.Flag) { digestGiven = digestGiven || f.Name == "if-digest" })
	if digestGiven {
		if d, err := hex.DecodeString(*ifDigest); err != nil || len(d) != sha256.Size {
			return fmt.Errorf("-if-digest needs the digest the preview printed (64 hex digits), not %q; nothing was changed", *ifDigest)
		}
		if !*apply {
			return errors.New("-if-digest checks the file an -apply changes: add -apply, or leave it out to preview")
		}
	}
	var b nodepolicy.Bundle
	var err error
	if file == "-" {
		// Standard input, as poo submit -f - reads it: how the GUI hands
		// over a file picked in the browser.
		b, err = nodepolicy.ReadBundle(inputOf(out))
		if cerr := contextOf(out).Err(); cerr != nil {
			return fmt.Errorf("standard input was cut off, so nothing was read: %w", cerr)
		}
	} else {
		var f *os.File
		if f, err = os.Open(file); err != nil {
			return err
		}
		b, err = nodepolicy.ReadBundle(f)
		f.Close()
	}
	if err != nil {
		return err
	}
	switch {
	case settingsOnly && b.Scope == nodepolicy.ScopeSeries:
		return errors.New("this file holds a series, not settings: poo policy import adds it")
	case settingsOnly && b.Scope == nodepolicy.ScopeNode:
		return errors.New("this file holds several node policies: export the settings of one with poo settings export, or import it whole with poo policy import")
	case !settingsOnly && b.Scope == nodepolicy.ScopeSettings:
		return errors.New("this file holds settings only: poo settings import takes them")
	}
	opt := nodepolicy.MergeOptions{PreferFile: *prefer, Replace: *replace, Settings: *settings, Full: *full,
		SeriesOnly: b.Scope == nodepolicy.ScopeSeries, SettingsOnly: settingsOnly, MapPaths: pathMaps}
	if opt.SeriesOnly && *replace {
		return errors.New("a series bundle adds one series: -replace is for a node policy")
	}
	m := w.manager()
	type target struct {
		name, dir string
		p         nodepolicy.Policy
		current   bool
	}
	var targets []target
	if b.Scope == nodepolicy.ScopeNode {
		if *w.name != "" || *w.dir != "" {
			return errors.New("this file holds several node policies, each imported under its own name: leave out -policy and -dir")
		}
		list, err := m.List()
		if err != nil {
			return err
		}
		archived := map[string]bool{}
		for _, e := range list {
			archived[e.Name] = archived[e.Name] || e.Archived
		}
		for _, bp := range b.Policies {
			if archived[bp.Name] {
				return fmt.Errorf("node policy %q is archived here: unarchive or delete it first, nothing was changed", bp.Name)
			}
			targets = append(targets, target{bp.Name, m.Dir(bp.Name), bp.Policy, bp.Current})
		}
	} else {
		name := ""
		if *w.dir == "" {
			name = w.policyName()
		}
		targets = []target{{name, w.resolve(), b.Policies[0].Policy, false}}
	}

	// Every plan first, so a node policy that cannot be imported stops
	// the import before any is changed (RED-045, D150).
	var results []importResult
	for _, t := range targets {
		r, err := importOne(out, t.dir, t.name, *w.dir == "", t.p, opt, false, *noReload, m)
		if err != nil {
			if t.name != "" {
				err = fmt.Errorf("node policy %q: %w", t.name, err)
			}
			return fmt.Errorf("%w; nothing was changed", err)
		}
		results = append(results, r)
	}
	// The digest covers what an apply does: the file, the options and the
	// plan, so -if-digest applies only what was previewed (RED-051, D154).
	dg := importDigest(b.Digest, opt, results)
	if digestGiven && !strings.EqualFold(*ifDigest, dg) {
		return fmt.Errorf("this is not the one previewed: the file, its options (-settings, -full, -replace, -prefer-file, -policy, -dir) or what it would change differ (digest %s, not %s), so nothing was changed: preview it again", dg, *ifDigest)
	}
	if *apply {
		var saved []string
		for i, t := range targets {
			r, err := importOne(out, t.dir, t.name, *w.dir == "", t.p, opt, true, *noReload, m)
			if err != nil {
				if t.name != "" {
					err = fmt.Errorf("node policy %q: %w", t.name, err)
				}
				if len(saved) == 0 {
					return fmt.Errorf("%w; nothing was changed", err)
				}
				return fmt.Errorf("%w; already saved: %s (importing again is safe: it changes nothing twice)", err, strings.Join(saved, ", "))
			}
			results[i] = r
			if r.Saved {
				saved = append(saved, fmt.Sprintf("%q", t.name))
			}
		}
	}
	if *apply && b.Scope == nodepolicy.ScopeNode {
		if _, err := m.Current(); err != nil {
			for i, t := range targets {
				if t.current {
					if err := m.Use(t.name); err != nil {
						return err
					}
					results[i].Current = true
				}
			}
		}
	}
	for i := range results {
		results[i].Digest = dg
	}
	if *asJSON {
		return writeJSON(out, results)
	}
	changes := slices.ContainsFunc(results, func(r importResult) bool { return !r.Plan.Empty() })
	if !*apply && changes {
		// Only a preview with something to apply needs the digest: it is
		// what -apply -if-digest checks (GREEN-043).
		fmt.Fprintf(out, "import digest %s (-apply -if-digest applies only this file, with these options and this plan)\n", dg)
	}
	if b.HasSettings() {
		fmt.Fprintln(out, `note: the file also holds a "settings" section, which this version does not read`)
	}
	for _, n := range pathMapNotes(b, pathMaps, *full) {
		fmt.Fprintln(out, "note: "+n)
	}
	for _, r := range results {
		printPlan(out, r, *apply, settingsOnly)
	}
	if !*apply && changes {
		fmt.Fprintln(out, "dry run: nothing was changed; add -apply to make this change")
	}
	return nil
}

// planLine is one added or removed line of a plan with its value: an
// item of a list as "grind.jobs: {...}", a setting as "k: 3", where the
// field alone ("+ k") did not say what would be set (GREEN-042).
func planLine(field, value string, short func(string) string) string {
	if i := strings.LastIndex(field, ".["); i >= 0 && strings.HasSuffix(field, "]") {
		return termSafe(field[:i]) + ": " + short(field[i+2:len(field)-1])
	}
	if value == "" {
		return short(field)
	}
	return termSafe(field) + ": " + short(value)
}

// termSafe returns s as it may be printed to a terminal: a control
// character (an escape, a newline, a carriage return, a backspace) or a
// bidirectional override becomes its Go escape (\x1b, \n, \u202e), so text
// from a file can neither move the cursor, erase what was printed, start a
// line of its own nor reorder what is shown (RED-053, D156).
func termSafe(s string) string { return plain.Safe(s) }

// importOne plans, and with apply makes, the import into one node policy:
// through the running node when there is one, so the change is made
// under the node's own lock and applied at once, else in node.json under
// the data directory's lock.
func importOne(out io.Writer, dir, name string, managed bool, file nodepolicy.Policy, opt nodepolicy.MergeOptions, apply, noReload bool, m nodepolicy.Manager) (importResult, error) {
	res := importResult{Name: name, Dir: dir, Mode: "merge"}
	if opt.Replace {
		res.Mode = "replace"
	}
	path := filepath.Join(dir, nodepolicy.FileName)
	var nodeErr error
	if _, err := os.Stat(path); err == nil {
		if cl, err := dialNode(out, dir); err == nil {
			r, err := cl.ImportPolicy(rpc.PolicyImportRequest{Policy: file, Options: opt, DryRun: !apply, NoReload: noReload})
			nodeErr = err
			if err == nil {
				res.Running, res.Plan, res.Saved, res.Reloading = true, r.Plan, r.Saved, r.Reloading
				if r.Reloading {
					waitBack(out, dir)
				}
				return res, nil
			}
			var re *rpc.Error
			if errors.As(err, &re) {
				return res, err
			}
			// The node did not answer: change the file instead.
		}
	}
	unlock := func() {}
	if apply {
		u, err := lockPolicy(dir, "node policy", nodeErr)
		if err != nil {
			return res, err
		}
		unlock = u
	}
	defer unlock()
	local, err := nodepolicy.Load(path)
	isNew := errors.Is(err, os.ErrNotExist)
	switch {
	case isNew:
		// A node policy that is not here yet is the file's, whole.
		local = nodepolicy.Default()
		opt.Replace = true
	case err != nil:
		return res, err
	}
	merged, plan, err := nodepolicy.Merge(local, file, opt)
	if err != nil {
		return res, err
	}
	plan.Fit = nodepolicy.Fit(local, merged, thisMachine())
	plan.New = isNew
	if isNew {
		res.Mode = "create"
	}
	res.Plan = plan
	if !apply || plan.Empty() {
		return res, nil
	}
	if managed && name != "" {
		err = m.Save(name, merged)
	} else {
		err = nodepolicy.Save(path, merged)
	}
	if err != nil {
		return res, err
	}
	res.Saved = true
	return res, nil
}

// printPlan prints one node policy's plan for reading.
func printPlan(out io.Writer, r importResult, apply, settingsOnly bool) {
	label := "node policy"
	if r.Name != "" {
		label += fmt.Sprintf(" %q", r.Name)
	}
	state := "not running"
	if r.Running {
		state = "running"
	}
	p := r.Plan
	fmt.Fprintf(out, "%s (%s, %s): %s\n", termSafe(label), termSafe(r.Dir), state, r.Mode)
	// Every value from the file goes through termSafe, so a control
	// character in it cannot move the cursor, erase lines or start a line
	// of its own (RED-053, D156).
	short := func(s string) string {
		s = termSafe(s)
		if len(s) > 60 {
			return s[:57] + "..."
		}
		return s
	}
	if len(p.Defences) > 0 {
		fmt.Fprintf(out, "  read first: these change how this node runs, guards itself or reaches out, or take on many pools: %s\n", listSome(planFields(p.Defences, short), 20))
	}
	if len(p.NowUsed) > 0 {
		fmt.Fprintf(out, "  now used: series %s, listed but not accepted before: the node will follow, mine into and relay %s (RED-056)\n", termSafe(strings.Join(p.NowUsed, ", ")), plural(len(p.NowUsed), "it", "them"))
	}
	for _, f := range p.Fit {
		fmt.Fprintf(out, "  may not fit this machine: %s\n", termSafe(f))
	}
	for _, f := range p.Follows {
		if f.Whole && f.Except == 0 {
			fmt.Fprintf(out, "  series %s: follows every pool (%d)\n", f.Series, f.Pools)
		} else {
			fmt.Fprintf(out, "  series %s: follows %d of %d pools\n", f.Series, f.Followed, f.Pools)
		}
	}
	for _, c := range p.Added {
		fmt.Fprintf(out, "  + %s\n", planLine(c.Field, c.File, short))
	}
	for _, c := range p.Changed {
		fmt.Fprintf(out, "  ~ %s: %s -> %s\n", termSafe(c.Field), short(c.Local), short(c.File))
	}
	for _, c := range p.Removed {
		fmt.Fprintf(out, "  - %s\n", planLine(c.Field, c.Local, short))
	}
	for _, c := range p.Conflicts {
		if c.Took == "file" {
			fmt.Fprintf(out, "  ! %s: took the file's %s (was %s)\n", termSafe(c.Field), short(c.File), short(c.Local))
		} else {
			fmt.Fprintf(out, "  ! %s: kept %s (file: %s); -prefer-file takes the file's\n", termSafe(c.Field), short(c.Local), short(c.File))
		}
	}
	if len(p.Skipped) > 0 {
		fmt.Fprintf(out, "  skipped, machine-local (-full takes them): %s\n", termSafe(strings.Join(p.Skipped, ", ")))
	}
	if len(p.SkippedSettings) > 0 {
		hint := "-settings takes them"
		if settingsOnly {
			hint = "-full takes them"
		}
		fmt.Fprintf(out, "  skipped, node settings (%s): %s\n", hint, listSome(planFields(p.SkippedSettings, short), 20))
	}
	for _, s := range p.Refused {
		fmt.Fprintf(out, "  warning: %s; it is listed but unused until accepted (issuers)\n", termSafe(s))
	}
	for _, s := range p.DroppedReleases {
		fmt.Fprintf(out, "  warning: %s; the release is left out, and the series is followed with the parts proven without it\n", termSafe(s))
	}
	if p.Same > 0 {
		verb := "are"
		if p.Same == 1 {
			verb = "is"
		}
		fmt.Fprintf(out, "  %d of the file's settings %s already the same\n", p.Same, verb)
	}
	switch {
	case p.Empty():
		fmt.Fprintln(out, "  nothing to change")
	case !apply:
	case r.Reloading:
		fmt.Fprintln(out, "  applied: the node reloaded")
	case r.Running:
		fmt.Fprintln(out, "  saved: the node applies it on its next reload or start")
	case r.Saved:
		fmt.Fprintln(out, "  saved")
	}
	if r.Current {
		fmt.Fprintln(out, "  made current")
	}
}

// planFields writes each of a plan's field names as its lines do, an
// item of a list as "p2p.peers: a.example:1" (GREEN-043).
func planFields(fields []string, short func(string) string) []string {
	out := make([]string, len(fields))
	for i, f := range fields {
		out[i] = planLine(f, "", short)
	}
	return out
}

// listSome joins the first n of list, and says how many more there are
// (-json prints them all).
func listSome(list []string, n int) string {
	if len(list) <= n {
		return strings.Join(list, ", ")
	}
	return fmt.Sprintf("%s and %d more (-json lists them all)", strings.Join(list[:n], ", "), len(list)-n)
}

// importDigest is the SHA-256 of what an import applies: the file's
// digest, the options, and the plan of every node policy by name and
// directory (RED-051, D154). A preview prints it; -apply -if-digest
// recomputes it from a fresh plan and refuses a difference.
func importDigest(file string, opt nodepolicy.MergeOptions, results []importResult) string {
	type row struct {
		Name string          `json:"name"`
		Dir  string          `json:"dir"`
		Mode string          `json:"mode"`
		Plan nodepolicy.Plan `json:"plan"`
	}
	rows := make([]row, len(results))
	for i, r := range results {
		rows[i] = row{r.Name, r.Dir, r.Mode, r.Plan}
	}
	b, _ := json.Marshal(struct {
		File    string                  `json:"file"`
		Options nodepolicy.MergeOptions `json:"options"`
		Plans   []row                   `json:"plans"`
	}{strings.ToLower(file), opt, rows})
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// pathMapNotes says when a -map-path cannot change anything: registry
// files are taken only with -full, and a map that matches no registry
// file of the file is most likely mistyped (GREEN-045). They are notes,
// not errors: the plan is the same either way.
func pathMapNotes(b nodepolicy.Bundle, maps []nodepolicy.PathMap, full bool) []string {
	if len(maps) == 0 {
		return nil
	}
	var notes []string
	if !full {
		notes = append(notes, "-map-path rewrites registry files, which only -full takes")
	}
	for _, m := range maps {
		used := false
		for _, bp := range b.Policies {
			used = used || slices.ContainsFunc(bp.Policy.P2P.Registries, m.Maps)
		}
		if !used {
			notes = append(notes, fmt.Sprintf("-map-path %s=%s matches no registry file in the file", m.From, m.To))
		}
	}
	return notes
}
