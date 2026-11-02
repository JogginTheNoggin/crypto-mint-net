package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/fsutil"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
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
	full := fs.Bool("full", false, "keep machine-local settings (rpc, no_sync, p2p listen and advertise, grind device and threads): for a backup or a clone of this machine")
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
	}
	if *file == "" {
		return b.Write(out)
	}
	var buf bytes.Buffer
	if err := b.Write(&buf); err != nil {
		return err
	}
	if err := fsutil.WriteFile(*file, buf.Bytes(), 0o600); err != nil {
		return err
	}
	fmt.Fprintf(out, "exported %d node %s (%s) to %s\n", len(b.Policies), plural(len(b.Policies), "policy", "policies"), b.Scope, *file)
	return nil
}

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
	// Digest is the SHA-256 of the file read, the same on every row:
	// pass it back with -if-digest to apply only that file (GUI-023).
	Digest string `json:"digest"`
}

// policyImportCmd reads a bundle (or a node.json) and merges it into the
// node policies here: it shows the plan, and changes them only with
// -apply. A running node takes the change at once.
func policyImportCmd(args []string, out io.Writer) error {
	fs := newFlagSet("policy import", out)
	w := addWhere(fs)
	apply := fs.Bool("apply", false, "make the change (default: only show the plan)")
	prefer := fs.Bool("prefer-file", false, "where both set a value and they differ, take the file's (default: keep this node's)")
	replace := fs.Bool("replace", false, "put the file's node policy in place of this one's instead of merging, keeping this machine's machine-local settings")
	settings := fs.Bool("settings", false, "take the file's node settings too (peers, registries, grind jobs, tracking, the p2p switches and limits); by default only what to follow is taken")
	full := fs.Bool("full", false, "take every setting, machine-local ones too (rpc, no_sync, p2p listen and advertise, grind device and threads, registry files): restoring a backup on this machine")
	noReload := fs.Bool("no-reload", false, "change node.json only; a running node applies it on its next reload or start")
	asJSON := fs.Bool("json", false, "print the plan as JSON")
	ifDigest := fs.String("if-digest", "", "apply only if the file's SHA-256 (hex, as the plan prints it) is this one: so the file applied is the file previewed")
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
		return errors.New("usage: poo policy import FILE|- [-apply] [-prefer-file] [-replace] [-settings] [-full] [-policy NAME | -dir DIR]")
	}
	if *apply && viewOnlyOf(out) {
		return errViewOnly("policy import -apply")
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
	if *ifDigest != "" && !strings.EqualFold(*ifDigest, b.Digest) {
		return fmt.Errorf("the file is not the one previewed (its digest is %s, not %s), so nothing was changed: preview it again", b.Digest, *ifDigest)
	}
	opt := nodepolicy.MergeOptions{PreferFile: *prefer, Replace: *replace, Settings: *settings, Full: *full, SeriesOnly: b.Scope == nodepolicy.ScopeSeries}
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
		results[i].Digest = b.Digest
	}
	if *asJSON {
		return writeJSON(out, results)
	}
	fmt.Fprintf(out, "file digest %s (-if-digest applies only this file)\n", b.Digest)
	if b.HasSettings() {
		fmt.Fprintln(out, "note: the file also holds settings; this version imports node policies only")
	}
	for _, r := range results {
		printPlan(out, r, *apply)
	}
	if !*apply && slices.ContainsFunc(results, func(r importResult) bool { return !r.Plan.Empty() }) {
		fmt.Fprintln(out, "dry run: nothing was changed; add -apply to make this change")
	}
	return nil
}

// planLine is one added or removed line of a plan with its value: an
// item of a list as "grind.jobs: {...}", a setting as "k: 3", where the
// field alone ("+ k") did not say what would be set (GREEN-042).
func planLine(field, value string, short func(string) string) string {
	if i := strings.LastIndex(field, ".["); i >= 0 && strings.HasSuffix(field, "]") {
		return field[:i] + ": " + short(field[i+2:len(field)-1])
	}
	if value == "" {
		return field
	}
	return field + ": " + short(value)
}

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
func printPlan(out io.Writer, r importResult, apply bool) {
	label := "node policy"
	if r.Name != "" {
		label += fmt.Sprintf(" %q", r.Name)
	}
	state := "not running"
	if r.Running {
		state = "running"
	}
	p := r.Plan
	fmt.Fprintf(out, "%s (%s, %s): %s\n", label, r.Dir, state, r.Mode)
	short := func(s string) string {
		if len(s) > 60 {
			return s[:57] + "..."
		}
		return s
	}
	if len(p.Defences) > 0 {
		fmt.Fprintf(out, "  read first: these change a defence of this node or where it reaches out: %s\n", listSome(p.Defences, 20))
	}
	for _, c := range p.Added {
		fmt.Fprintf(out, "  + %s\n", planLine(c.Field, c.File, short))
	}
	for _, c := range p.Changed {
		fmt.Fprintf(out, "  ~ %s: %s -> %s\n", c.Field, short(c.Local), short(c.File))
	}
	for _, c := range p.Removed {
		fmt.Fprintf(out, "  - %s\n", planLine(c.Field, c.Local, short))
	}
	for _, c := range p.Conflicts {
		if c.Took == "file" {
			fmt.Fprintf(out, "  ! %s: took the file's %s (was %s)\n", c.Field, short(c.File), short(c.Local))
		} else {
			fmt.Fprintf(out, "  ! %s: kept %s (file: %s); -prefer-file takes the file's\n", c.Field, short(c.Local), short(c.File))
		}
	}
	if len(p.Skipped) > 0 {
		fmt.Fprintf(out, "  skipped, machine-local (-full takes them): %s\n", strings.Join(p.Skipped, ", "))
	}
	if len(p.SkippedSettings) > 0 {
		fmt.Fprintf(out, "  skipped, node settings (-settings takes them): %s\n", listSome(p.SkippedSettings, 20))
	}
	for _, s := range p.Refused {
		fmt.Fprintf(out, "  warning: %s; it is listed but unused until accepted (issuers)\n", s)
	}
	if p.Same > 0 {
		fmt.Fprintf(out, "  %d of the file's settings are already the same\n", p.Same)
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

// listSome joins the first n of list, and says how many more there are
// (-json prints them all).
func listSome(list []string, n int) string {
	if len(list) <= n {
		return strings.Join(list, ", ")
	}
	return fmt.Sprintf("%s and %d more (-json lists them all)", strings.Join(list[:n], ", "), len(list)-n)
}
