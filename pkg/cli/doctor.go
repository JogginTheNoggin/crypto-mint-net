package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/grind"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/node"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// poon doctor checks a node's setup and says what to fix (GREEN-004). It
// only reads: it never writes a file, takes the data directory's lock or
// dials anything but the node's own local addresses, so it is safe to run
// beside a running node.

// Check results.
const (
	checkOK   = "ok"
	checkWarn = "warn"
	checkFail = "fail"
	checkSkip = "skip"
)

// Check is one line of a doctor report.
type Check struct {
	Name   string `json:"name"`
	Result string `json:"result"`
	Detail string `json:"detail"`
	Fix    string `json:"fix,omitempty"`
	// FixArgs is the fix as a poon command line, when the fix is one
	// command, so a front end can offer to run exactly that (GREEN-022).
	FixArgs []string `json:"fix_args,omitempty"`
}

// DoctorReport is what poon doctor -json prints.
type DoctorReport struct {
	Dir    string  `json:"dir"`
	OK     bool    `json:"ok"` // no check failed
	Checks []Check `json:"checks"`
}

func doctorCmd(args []string, out io.Writer) error {
	fs := newFlagSet("doctor", out)
	w := addWhere(fs)
	asJSON := fs.Bool("json", false, "print the report as JSON (the same as -o json)")
	format := fs.String("o", "", "output: json, or table; default: the readable report")
	if err := fs.Parse(args); err != nil {
		return err
	}
	switch *format {
	case "", "json", "table":
	default:
		return fmt.Errorf("-o must be json or table, not %q", *format)
	}
	rep := doctor(out, w.resolve())
	// A fix run as printed must change the node policy that was checked:
	// when -root, -policy or -dir chose it, each fix_args names it by
	// -dir, or "poon grind schedule set" would change the current one
	// (GREEN-036).
	chosen := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "root" || f.Name == "policy" || f.Name == "dir" {
			chosen = true
		}
	})
	if chosen {
		for i, c := range rep.Checks {
			if len(c.FixArgs) > 0 {
				rep.Checks[i].FixArgs = append(slices.Clone(c.FixArgs), "-dir", rep.Dir)
			}
		}
	}
	if *asJSON || *format != "" {
		f := *format
		if *asJSON {
			f = "json"
		}
		wide := false
		if err := (&outputFlags{format: &f, wide: &wide}).write(out, rep); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(out, "data directory %s\n", rep.Dir)
		// The name column is as wide as the longest name, so a longer one
		// (open-files, D172) keeps every detail and fix in line (GREEN-053).
		w := 8
		for _, c := range rep.Checks {
			w = max(w, len(c.Name))
		}
		for _, c := range rep.Checks {
			fmt.Fprintf(out, "%-4s  %-*s  %s\n", c.Result, w, c.Name, c.Detail)
			if c.Fix != "" {
				fmt.Fprintf(out, "%s  fix: %s\n", strings.Repeat(" ", 4+2+w), c.Fix)
			}
		}
	}
	if !rep.OK {
		return errors.New("doctor: some checks failed")
	}
	return nil
}

// doctor runs every check on the data directory dir. A check that needs an
// earlier one to pass is skipped when it did not.
func doctor(out io.Writer, dir string) DoctorReport {
	rep := DoctorReport{Dir: dir, OK: true}
	add := func(c Check) {
		rep.Checks = append(rep.Checks, c)
		if c.Result == checkFail {
			rep.OK = false
		}
	}
	skipRest := func(names ...string) {
		for _, n := range names {
			add(Check{Name: n, Result: checkSkip, Detail: "needs the checks above"})
		}
	}

	// 1. The node policy.
	path := filepath.Join(dir, nodepolicy.FileName)
	p, err := nodepolicy.Load(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		add(Check{Name: "policy", Result: checkFail, Detail: "no node policy at " + path,
			Fix: "poon init (or poon init -example to try the worked example), or pick one with -policy or -dir", FixArgs: []string{"init"}})
		skipRest("files", "series", "grind", "rpc", "p2p")
		return rep
	case err != nil:
		add(Check{Name: "policy", Result: checkFail, Detail: err.Error(), Fix: "correct " + path + "; poon node refuses it as it is"})
		skipRest("files", "series", "grind", "rpc", "p2p")
		return rep
	}
	add(Check{Name: "policy", Result: checkOK, Detail: fmt.Sprintf("%s parses and validates (series: %d, follow rules: %d)", path, len(p.Series), len(p.Follow))})
	for _, n := range p.Lowered {
		// No command writes every such field (p2p.check_seconds has
		// none), so the fix names the value to write (GREEN-044).
		fix := fmt.Sprintf("set %s to %d in %s", n.Field, n.Now, path)
		if len(n.FixArgs) > 0 {
			fix = "poon " + strings.Join(n.FixArgs, " ") + " (or edit " + path + ")"
		}
		add(Check{Name: "policy", Result: checkWarn, Detail: n.String(), Fix: fix, FixArgs: n.FixArgs})
	}
	for _, c := range unknownSettingsChecks(p) {
		add(c)
	}

	// 2. Who can read the node's files.
	add(filesCheck(dir))

	// 3. The listed series.
	accepted, refused := p.Accepted()
	switch {
	case len(refused) > 0:
		var why []string
		variable := 0
		for _, r := range refused {
			why = append(why, fmt.Sprintf("series %d: %v", r.Index, r.Reason))
			if errors.Is(r.Reason, nodepolicy.ErrNoPartOne) {
				variable++
			}
		}
		// A variable series without its issuer witness is not wrong, only
		// missing part 1's release (D166), so its fix is to add the
		// witness, not to correct the series (GREEN-049).
		fix := "a series that is not accepted is not followed; correct or remove it in the policy"
		switch {
		case variable == len(refused):
			fix = variableNote
		case variable > 0:
			fix = "correct or remove the other series that are not accepted; " + variableNote
		}
		add(Check{Name: "series", Result: checkWarn, Detail: fmt.Sprintf("%d accepted, %d not: %s", len(accepted), len(refused), strings.Join(why, "; ")),
			Fix: fix})
	case len(accepted) == 0:
		add(Check{Name: "series", Result: checkWarn, Detail: "the policy lists no series, so the node follows nothing",
			Fix: "add a series with poon init -series FILE, then poon follow"})
	default:
		add(Check{Name: "series", Result: checkOK, Detail: fmt.Sprintf("%d accepted", len(accepted))})
	}

	// A followed series with no witness ceiling, or one above
	// CeilingLimit: one witness can take seconds to check (RED-083).
	var loose []string
	for _, a := range accepted {
		if w := nodepolicy.CeilingWarning(a.Series.Params()); w != "" {
			loose = append(loose, fmt.Sprintf("series %s: %s", a.Series.ID().Short(), w))
		}
	}
	if len(loose) > 0 {
		add(Check{Name: "ceiling", Result: checkWarn, Detail: strings.Join(loose, "; "),
			Fix: "follow such a series only on purpose; poon policy limits -set series_ceiling=N declines new ones on import"})
	}

	// One store file per pool in use, and room for peers: a limit below
	// that fails later with "too many open files" (ORANGE-037). The
	// limit is this process's, as Go raised it at start, which is the
	// node's when it is started from the same shell or service.
	if fl := nodepolicy.CheckFileLimit(p.FollowedPools(accepted)); fl.Low {
		add(Check{Name: "open-files", Result: checkWarn, Detail: fl.String(),
			Fix: fmt.Sprintf("raise the open file limit to at least %d (ulimit -n, or LimitNOFILE= in a systemd unit), or follow fewer pools", fl.Need)})
	} else if fl.Known {
		add(Check{Name: "open-files", Result: checkOK, Detail: fmt.Sprintf("the open file limit is %d; following these pools needs at most %d", fl.Soft, fl.Need)})
	}

	// A release node.json keeps that does not check is left out, and its
	// series followed with the parts proven without it (RED-073).
	var bad []string
	for _, a := range accepted {
		if a.ReleaseErr != nil {
			bad = append(bad, fmt.Sprintf("series %s: %v", a.Series.ID().Short(), a.ReleaseErr))
		}
	}
	if len(bad) > 0 {
		add(Check{Name: "releases", Result: checkWarn, Detail: "not used: " + strings.Join(bad, "; "),
			Fix: "give the node a valid release with poon series add-release FILE; it replaces the one kept"})
	}

	// 4. Grinding: the devices, and the grind section's.
	add(grindCheck(p, accepted))

	// 5. The local RPC, and through it the running node.
	if p.ListenAddr() == "" {
		add(Check{Name: "rpc", Result: checkSkip, Detail: "rpc.listen is off in the policy, so no command can reach the node"})
		skipRest("p2p")
		return rep
	}
	c, running := rpcCheck(out, dir)
	add(c)
	if !running {
		skipRest("p2p")
		return rep
	}

	// 6. Entry bodies read back from the logs (RED-084), and the pool
	// snapshots (D178).
	if st, ok := nodeStatus(out, dir); ok {
		add(releaseCheck(st))
		add(bodiesCheck(st))
		add(snapshotsCheck(st))
		if c, ok := portMapCheck(st); ok {
			add(c)
		}
	}

	// 7. Peers.
	add(p2pCheck(out, dir, p))
	return rep
}

// unknownSettingsChecks reports the settings in node.json this release
// does not know (W-1, D184): kept ones as a note, and those inside list
// entries, which the next save drops, as a warning (GREEN-057).
func unknownSettingsChecks(p nodepolicy.Policy) []Check {
	var out []Check
	if u := p.UnknownSettings(); len(u) > 0 {
		out = append(out, Check{Name: "policy", Result: checkOK,
			Detail: fmt.Sprintf("%d %s this release does not know, kept on every save but not used (a newer release wrote %s, or %s misspelt): %s",
				len(u), plural(len(u), "setting", "settings"), plural(len(u), "it", "them"), plural(len(u), "it is", "they are"), strings.Join(u, ", "))})
	}
	if d := p.UnknownDropped(); len(d) > 0 {
		out = append(out, Check{Name: "policy", Result: checkWarn,
			Detail: fmt.Sprintf("%d %s this release does not know %s, where the next save of %s drops %s: %s",
				len(d), plural(len(d), "setting", "settings"), plural(len(d), "sits inside a list entry", "sit inside list entries"), nodepolicy.FileName, plural(len(d), "it", "them"), nodepolicy.NameSome(d, 20)),
			Fix: "if a newer release wrote " + plural(len(d), "it", "them") + ", run that release, or copy " + nodepolicy.FileName + " aside before changing settings with this one; if " + plural(len(d), "it is", "they are") + " misspelt, correct " + nodepolicy.FileName})
	}
	if f := p.FoldedSettings(); len(f) > 0 {
		out = append(out, Check{Name: "policy", Result: checkWarn,
			Detail: fmt.Sprintf("%d %s in %s %s a setting's name in another letter case, read as that setting; the next save writes its own name: %s",
				len(f), plural(len(f), "key", "keys"), nodepolicy.FileName, plural(len(f), "spells", "spell"), nodepolicy.NameSome(f, 20)),
			Fix: "write each setting once, under its own name, in " + nodepolicy.FileName})
	}
	return out
}

// releaseCheck compares the running node's release with this poon's
// (W-10, D184): after an upgrade the node keeps running the old binary
// until it restarts (GREEN-057).
func releaseCheck(st rpc.StatusReply) Check {
	c := Check{Name: "release"}
	mine := node.RunningRelease()
	theirs := st.Release
	show := func(r node.Release) string {
		if r.Commit == "" {
			return r.Version
		}
		return r.Version + " (" + r.Commit + ")"
	}
	switch {
	case theirs.Version == "":
		c.Result = checkWarn
		c.Detail = "the node does not say its release, so it runs one older than this poon " + show(mine)
		c.Fix = "restart the node so it runs this release"
	case theirs.Version != mine.Version || (theirs.Commit != "" && mine.Commit != "" && theirs.Commit != mine.Commit):
		c.Result = checkWarn
		c.Detail = "the node runs " + show(theirs) + ", but this poon is " + show(mine)
		c.Fix = "if this poon is the upgrade, restart the node so it runs it; otherwise use the poon that matches the node"
	default:
		c.Result = checkOK
		c.Detail = "the node runs " + show(theirs) + ", as this poon does"
	}
	return c
}

// bodiesCheck warns when the running node failed to read entry bodies
// back from its pool logs: a log damaged or changed under it. Such an
// entry is not held for peers and queries until the node starts again
// (RED-084, D173).
func bodiesCheck(st rpc.StatusReply) Check {
	if n := st.Bodies.Errors; n > 0 {
		return Check{Name: "bodies", Result: checkWarn,
			Detail: fmt.Sprintf("%d reads of entry bodies from the pool logs failed: a log is damaged or was changed under the node, and those entries are not served or shown", n),
			Fix:    "check the disk, then restart the node: on start it cuts a pool log at its first damaged record and fetches the rest again from peers"}
	}
	return Check{Name: "bodies", Result: checkOK, Detail: fmt.Sprintf("%d entry bodies read back from the logs, none failed", st.Bodies.Loads)}
}

// nodeStatus asks the running node for its status.
func nodeStatus(out io.Writer, dir string) (rpc.StatusReply, bool) {
	cl, err := dialNode(out, dir)
	if err != nil {
		return rpc.StatusReply{}, false
	}
	st, err := cl.Status()
	return st, err == nil
}

// snapshotsCheck reports the pool snapshots a start would begin from
// (D178): their age and the most records a start would still replay,
// a snapshot found unusable at the last start, and a -reverify that found
// a snapshot differing from a full replay, which would be a bug to
// report.
func snapshotsCheck(st rpc.StatusReply) Check {
	s, r := st.Snapshots, st.Replay
	c := Check{Name: "snapshots"}
	switch {
	case r.Differed > 0:
		c.Result = checkWarn
		c.Detail = fmt.Sprintf("-reverify found %d pool snapshots that differ from a full replay of their logs; the node runs on the replay", r.Differed)
		c.Fix = "please report this as a bug, with poon version -v; the node keeps working, and the next stop writes snapshots from the replayed pools"
	case len(r.NotCompared) > 0:
		// -reverify was asked for and compared nothing for these pools;
		// that is never "ok" (RED-088, D181).
		c.Result = checkWarn
		n := max(r.NotComparedCount, len(r.NotCompared))
		c.Detail = fmt.Sprintf("-reverify compared nothing for %d %s, whose snapshot does not reach the end of the log: %s", n, plural(n, "pool", "pools"), reasons(r.NotCompared, n))
		c.Fix = "stop the node cleanly, which writes snapshots at the end of each log, then start it with -reverify again"
	case s.Off:
		c.Result = checkOK
		c.Detail = "no_snapshots is on: every start replays the whole pool logs"
	case len(r.Unused) > 0:
		c.Result = checkWarn
		// Every reason, not only the first: a damaged file and an
		// upgrade read differently, and each pool may have its own
		// (GREEN-055).
		n := max(r.UnusedCount, len(r.Unused))
		c.Detail = fmt.Sprintf("%d %s not used at the last start, so %s replayed whole: %s", n, plural(n, "pool snapshot was", "pool snapshots were"), plural(n, "its log was", "their logs were"), reasons(r.Unused, n))
		c.Fix = "nothing is needed if this follows an upgrade or a compaction; otherwise check the disk. A clean stop writes new snapshots"
	case s.Behind > 2*max(s.Every, s.Due):
		c.Result = checkWarn
		c.Detail = fmt.Sprintf("a start would replay up to %d records of one pool log (a snapshot is due every %d)", s.Behind, max(s.Every, s.Due))
		c.Fix = "nothing is lost; a snapshot is written once the node can write every held body; if this lasts, check the node's log for \"pool snapshot not written\""
	default:
		c.Result = checkOK
		age := "none written yet"
		if !s.Oldest.IsZero() {
			age = fmt.Sprintf("the oldest is %s old", time.Duration(s.AgeSeconds)*time.Second)
		}
		c.Detail = fmt.Sprintf("%d of %d pools have a snapshot, %s; a start replays at most %d records of a log", s.Pools-s.Missing, s.Pools, age, s.Behind)
		// A -reverify that found every snapshot equal to its full replay
		// says so, rather than nothing (D178 names it in doctor; GREEN-055).
		if r.Reverified > 0 {
			c.Detail += fmt.Sprintf("; -reverify replayed %d %s whole at the last start and %s its snapshot", r.Reverified, plural(r.Reverified, "pool", "pools"), plural(r.Reverified, "it matched", "each matched"))
		}
	}
	if r.TmpRemoved > 0 {
		c.Detail += fmt.Sprintf("; %d temporary snapshot %s left by a crash %s removed at the last start", r.TmpRemoved, plural(r.TmpRemoved, "file", "files"), plural(r.TmpRemoved, "was", "were"))
	}
	return c
}

// portMapCheck reports the port mapping (p2p.port_map, D182), when the
// policy asks for one.
func portMapCheck(st rpc.StatusReply) (Check, bool) {
	pm := st.PortMap
	if pm == nil || pm.Mode == p2p.PortMapOff {
		return Check{}, false
	}
	c := Check{Name: "port-map"}
	switch pm.State {
	case p2p.PortMapAdvertised:
		c.Result = checkOK
		life := "until the node stops"
		if pm.LeaseSeconds > 0 {
			life = fmt.Sprintf("leased for %s and renewed", time.Duration(pm.LeaseSeconds)*time.Second)
		}
		c.Detail = fmt.Sprintf("the router forwards %s (%s, %s); a dial to it reached this node, so peers are offered it", pm.Advertised, pm.Method, life)
	case p2p.PortMapSkipped:
		c.Result = checkOK
		// The one reason that holds, when the node says which (GREEN-056).
		why := pm.Reason
		if why == "" {
			why = "p2p.advertise is set by hand, which always wins, or the node accepts no peers (p2p.listen is off)"
		}
		c.Detail = "p2p.port_map is set but not used: " + why
	case p2p.PortMapTrying:
		c.Result = checkOK
		c.Detail = "asking the router for a mapping"
	case p2p.PortMapUnconfirmed:
		c.Result = checkWarn
		c.Detail = fmt.Sprintf("the router mapped %s, but a dial to it did not reach this node, so it is not offered and the mapping was removed", pm.External) + nextTry(pm)
		c.Fix = "many routers cannot loop a connection back to their own address; if " + pm.External + " reaches this node from outside, set p2p.advertise to it"
	case p2p.PortMapNotPublic:
		c.Result = checkWarn
		c.Detail = fmt.Sprintf("the router's external address %s is not public: this network is behind another NAT, such as carrier-grade NAT, so no mapping can reach this node", pm.External) + nextTry(pm)
		c.Fix = "the node works with outbound connections only; for inbound ones ask the provider for a public address, or run it where it has one"
	default:
		c.Result = checkWarn
		// Name the router asked, and when the node asks again, so a
		// runner who fixes the router knows whether to wait or reload
		// (GREEN-056).
		asked := "no gateway was found"
		if pm.Gateway != "" {
			asked = "asked the router at " + pm.Gateway
		}
		c.Detail = "the router gave no mapping (" + asked + "): " + pm.Error + nextTry(pm)
		c.Fix = "turn on NAT-PMP, PCP or UPnP in the router, or set p2p.port_map_gateway if that is not your router; otherwise forward the port by hand and set p2p.advertise. The node keeps trying, and poon reload tries again at once"
	}
	return c, true
}

// nextTry says when the node asks the router again, if it said.
func nextTry(pm *p2p.PortMapStatus) string {
	if pm.NextTry.IsZero() {
		return ""
	}
	in := time.Until(pm.NextTry).Round(time.Second)
	if in < 0 {
		in = 0
	}
	return fmt.Sprintf("; it asks again in %s", in)
}

// reasons joins the per-pool reasons a status reply carries; when it
// carries only the first of n, it says the node's start log names every
// pool (D184).
func reasons(l []string, n int) string {
	s := strings.Join(l, "; ")
	if n > len(l) {
		s += fmt.Sprintf("; and %d more, each named in the node's start log", n-len(l))
	}
	return s
}

// grindCheck reports the devices this machine grinds on, a device the
// grind section names that is not there, and a hash of the followed
// series that only the generic grinder can grind (package grind, M11).
func grindCheck(p nodepolicy.Policy, accepted []nodepolicy.Accepted) Check {
	devs := grind.Default.Detect()
	want := ""
	if p.Grind != nil {
		want = p.Grind.Device
		for _, j := range p.Grind.Jobs {
			if j.Device != "" && j.Device != grind.Auto && want == "" {
				want = j.Device
			}
		}
	}
	ids := make([]string, len(devs))
	for i, d := range devs {
		ids[i] = d.ID
	}
	var slow []string
	for _, a := range accepted {
		id := a.Series.Params().HashID
		pl, err := grind.Default.Choose(devs, id, want, 0)
		if err != nil {
			return Check{Name: "grind", Result: checkFail, Detail: err.Error(),
				Fix: "set grind.device in the policy to auto or one of: " + strings.Join(ids, ", ")}
		}
		if pl.Fallback {
			slow = append(slow, pl.Reason)
		}
	}
	if len(slow) > 0 {
		return Check{Name: "grind", Result: checkWarn, Detail: strings.Join(slow, "; "),
			Fix: "grinding works but slower; poon grind bench shows how fast"}
	}
	jobs := 0
	if p.Grind != nil {
		jobs = len(p.Grind.Jobs)
	}
	return Check{Name: "grind", Result: checkOK, Detail: fmt.Sprintf("devices: %s; jobs in the policy: %d", strings.Join(ids, ", "), jobs)}
}

// filesCheck reports a data directory or token others can read.
func filesCheck(dir string) Check {
	if runtime.GOOS == "windows" {
		return Check{Name: "files", Result: checkSkip, Detail: "permission bits are not checked on Windows"}
	}
	var loose []string
	for _, f := range []string{dir, filepath.Join(dir, rpc.TokenFile), filepath.Join(dir, rpc.ViewTokenFile)} {
		st, err := os.Stat(f)
		if err != nil {
			continue // a token is made on first start
		}
		if st.Mode().Perm()&0o077 != 0 {
			loose = append(loose, fmt.Sprintf("%s is %v", f, st.Mode().Perm()))
		}
	}
	if len(loose) > 0 {
		return Check{Name: "files", Result: checkWarn, Detail: "others can read: " + strings.Join(loose, ", "),
			Fix: "chmod 700 " + dir + "; chmod 600 " + filepath.Join(dir, "rpc.*token")}
	}
	return Check{Name: "files", Result: checkOK, Detail: "only the node's user can read the data directory and tokens"}
}

// rpcCheck reports whether the node answers, and so whether it runs.
func rpcCheck(out io.Writer, dir string) (Check, bool) {
	addrPath := filepath.Join(dir, rpc.AddrFile)
	if _, err := os.Stat(addrPath); errors.Is(err, fs.ErrNotExist) {
		return Check{Name: "rpc", Result: checkWarn, Detail: "the node is not running (no " + rpc.AddrFile + ")", Fix: "poon node", FixArgs: []string{"node"}}, false
	}
	cl, err := dialNode(out, dir)
	if errors.Is(err, rpc.ErrNoProof) {
		return Check{Name: "rpc", Result: checkFail, Detail: "something that is not this node answers at the address in " + rpc.AddrFile + ": it could not prove it knows " + rpc.TokenFile + ", so the token was not sent",
			Fix: "another program may hold the node's port; stop it, or set rpc.listen to another address, and start the node"}, false
	}
	var dial *net.OpError
	if errors.As(err, &dial) {
		return Check{Name: "rpc", Result: checkFail, Detail: rpc.AddrFile + " is left over: nothing answers at its address (" + dial.Error() + ")",
			Fix: "the node stopped without cleaning up; start it with poon node, which rewrites the file"}, false
	}
	if err != nil {
		return Check{Name: "rpc", Result: checkFail, Detail: err.Error(), Fix: "restart the node; it writes a new token if " + rpc.TokenFile + " is missing"}, false
	}
	st, err := cl.Status()
	var re *rpc.Error
	var ne net.Error
	var oe *net.OpError
	switch {
	case err == nil && st.PolicyChanged:
		return Check{Name: "rpc", Result: checkWarn, Detail: fmt.Sprintf("the node answers, following %d pools, but %s has changed since it loaded it, and the change waits for a reload", len(st.Pools), nodepolicy.FileName),
			Fix: "poon reload", FixArgs: []string{"reload"}}, true
	case err == nil:
		return Check{Name: "rpc", Result: checkOK, Detail: fmt.Sprintf("the node answers, following %d pools", len(st.Pools))}, true
	case errors.As(err, &re) && re.Code == rpc.CodeUnauthorized:
		return Check{Name: "rpc", Result: checkFail, Detail: "the node refuses " + rpc.TokenFile, Fix: "another node may be listening on that address; check " + addrPath}, false
	case errors.As(err, &oe) && oe.Op == "dial", errors.As(err, &ne):
		return Check{Name: "rpc", Result: checkFail, Detail: rpc.AddrFile + " is left over: nothing answers at its address (" + err.Error() + ")",
			Fix: "the node stopped without cleaning up; start it with poon node, which rewrites the file"}, false
	default:
		return Check{Name: "rpc", Result: checkFail, Detail: err.Error()}, false
	}
}

// p2pCheck reports whether the running node accepts peers and has, or can
// find, any.
func p2pCheck(out io.Writer, dir string, p nodepolicy.Policy) Check {
	if p.P2PListenAddr() == "" && len(p.P2P.Peers) == 0 && len(p.P2P.Registries) == 0 {
		return Check{Name: "p2p", Result: checkWarn, Detail: "p2p listen is off and no peers or registries are set, so the node is alone",
			Fix: "add p2p.peers or p2p.registries (poon init -peer ADDR), or set p2p.listen"}
	}
	cl, err := dialNode(out, dir)
	if err != nil {
		return Check{Name: "p2p", Result: checkFail, Detail: err.Error()}
	}
	pr, err := cl.Peers()
	if err != nil {
		return Check{Name: "p2p", Result: checkWarn, Detail: "the node reports no p2p host: " + err.Error()}
	}
	var notes []string
	result := checkOK
	if pr.Listen != "" {
		_, port, _ := net.SplitHostPort(pr.Listen)
		conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", port), 2*time.Second)
		if err != nil {
			result = checkWarn
			notes = append(notes, "nothing accepts on "+pr.Listen+" from loopback")
		} else {
			conn.Close()
			notes = append(notes, "accepting on "+pr.Listen)
		}
	}
	notes = append(notes, fmt.Sprintf("%d connected, %d in the peer book", len(pr.Peers), len(pr.Book)))
	if cr, err := cl.Chases(); err == nil {
		for _, g := range cr.Groups {
			if g.Waiting == p2p.WaitDisk {
				// Black's RED-022 condition (D124): a heavier ledger
				// waiting on disk space is not a healthy node.
				result = checkWarn
				notes = append(notes, fmt.Sprintf("a chase in pool %s has waited for disk space since %s", g.Pool, g.WaitingSince.Format(time.RFC3339)))
			}
		}
	}
	c := Check{Name: "p2p", Result: result, Detail: strings.Join(notes, "; ")}
	if c.Result == checkWarn && strings.Contains(c.Detail, "disk space") {
		c.Fix = "free disk space where the node keeps its data (its parts directory); the chase goes on by itself"
	}
	if len(pr.Peers) == 0 && len(p.P2P.Peers) == 0 && len(p.P2P.Registries) == 0 {
		c.Result = checkWarn
		c.Fix = "no peers are set; add p2p.peers or p2p.registries so the node can find others"
	}
	return c
}
