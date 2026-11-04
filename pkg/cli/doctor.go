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
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/nodepolicy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/p2p"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/rpc"
)

// poo doctor checks a node's setup and says what to fix (GREEN-004). It
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
	// FixArgs is the fix as a poo command line, when the fix is one
	// command, so a front end can offer to run exactly that (GREEN-022).
	FixArgs []string `json:"fix_args,omitempty"`
}

// DoctorReport is what poo doctor -json prints.
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
	// -dir, or "poo grind schedule set" would change the current one
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
		for _, c := range rep.Checks {
			fmt.Fprintf(out, "%-4s  %-8s  %s\n", c.Result, c.Name, c.Detail)
			if c.Fix != "" {
				fmt.Fprintf(out, "                fix: %s\n", c.Fix)
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
			Fix: "poo init (or poo init -example to try the worked example), or pick one with -policy or -dir", FixArgs: []string{"init"}})
		skipRest("files", "series", "grind", "rpc", "p2p")
		return rep
	case err != nil:
		add(Check{Name: "policy", Result: checkFail, Detail: err.Error(), Fix: "correct " + path + "; poo node refuses it as it is"})
		skipRest("files", "series", "grind", "rpc", "p2p")
		return rep
	}
	add(Check{Name: "policy", Result: checkOK, Detail: fmt.Sprintf("%s parses and validates (series: %d, follow rules: %d)", path, len(p.Series), len(p.Follow))})
	for _, n := range p.Lowered {
		// No command writes every such field (p2p.check_seconds has
		// none), so the fix names the value to write (GREEN-044).
		fix := fmt.Sprintf("set %s to %d in %s", n.Field, n.Now, path)
		if len(n.FixArgs) > 0 {
			fix = "poo " + strings.Join(n.FixArgs, " ") + " (or edit " + path + ")"
		}
		add(Check{Name: "policy", Result: checkWarn, Detail: n.String(), Fix: fix, FixArgs: n.FixArgs})
	}

	// 2. Who can read the node's files.
	add(filesCheck(dir))

	// 3. The listed series.
	accepted, refused := p.Accepted()
	switch {
	case len(refused) > 0:
		var why []string
		for _, r := range refused {
			why = append(why, fmt.Sprintf("series %d: %v", r.Index, r.Reason))
		}
		add(Check{Name: "series", Result: checkWarn, Detail: fmt.Sprintf("%d accepted, %d not: %s", len(accepted), len(refused), strings.Join(why, "; ")),
			Fix: "a series that is not accepted is not followed; correct or remove it in the policy"})
	case len(accepted) == 0:
		add(Check{Name: "series", Result: checkWarn, Detail: "the policy lists no series, so the node follows nothing",
			Fix: "add a series with poo init -series FILE, then poo follow"})
	default:
		add(Check{Name: "series", Result: checkOK, Detail: fmt.Sprintf("%d accepted", len(accepted))})
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

	// 6. Peers.
	add(p2pCheck(out, dir, p))
	return rep
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
			Fix: "grinding works but slower; poo grind bench shows how fast"}
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
		return Check{Name: "rpc", Result: checkWarn, Detail: "the node is not running (no " + rpc.AddrFile + ")", Fix: "poo node", FixArgs: []string{"node"}}, false
	}
	cl, err := dialNode(out, dir)
	if errors.Is(err, rpc.ErrNoProof) {
		return Check{Name: "rpc", Result: checkFail, Detail: "something that is not this node answers at the address in " + rpc.AddrFile + ": it could not prove it knows " + rpc.TokenFile + ", so the token was not sent",
			Fix: "another program may hold the node's port; stop it, or set rpc.listen to another address, and start the node"}, false
	}
	var dial *net.OpError
	if errors.As(err, &dial) {
		return Check{Name: "rpc", Result: checkFail, Detail: rpc.AddrFile + " is left over: nothing answers at its address (" + dial.Error() + ")",
			Fix: "the node stopped without cleaning up; start it with poo node, which rewrites the file"}, false
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
			Fix: "poo reload", FixArgs: []string{"reload"}}, true
	case err == nil:
		return Check{Name: "rpc", Result: checkOK, Detail: fmt.Sprintf("the node answers, following %d pools", len(st.Pools))}, true
	case errors.As(err, &re) && re.Code == rpc.CodeUnauthorized:
		return Check{Name: "rpc", Result: checkFail, Detail: "the node refuses " + rpc.TokenFile, Fix: "another node may be listening on that address; check " + addrPath}, false
	case errors.As(err, &oe) && oe.Op == "dial", errors.As(err, &ne):
		return Check{Name: "rpc", Result: checkFail, Detail: rpc.AddrFile + " is left over: nothing answers at its address (" + err.Error() + ")",
			Fix: "the node stopped without cleaning up; start it with poo node, which rewrites the file"}, false
	default:
		return Check{Name: "rpc", Result: checkFail, Detail: err.Error()}, false
	}
}

// p2pCheck reports whether the running node accepts peers and has, or can
// find, any.
func p2pCheck(out io.Writer, dir string, p nodepolicy.Policy) Check {
	if p.P2PListenAddr() == "" && len(p.P2P.Peers) == 0 && len(p.P2P.Registries) == 0 {
		return Check{Name: "p2p", Result: checkWarn, Detail: "p2p listen is off and no peers or registries are set, so the node is alone",
			Fix: "add p2p.peers or p2p.registries (poo init -peer ADDR), or set p2p.listen"}
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
