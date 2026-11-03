package nodepolicy

import (
	"fmt"
	"net"
	"slices"
	"strings"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

// Settings export and import (decision D155). A node's settings are every
// part of its node policy but what it follows (Followed): its peers and
// registries, limits, p2p switches, tracking, logs and grind jobs, and,
// in a full file, the machine-local ones (MachineLocal). They travel in a
// settings bundle (ScopeSettings), so a user can carry them to another
// machine, keep a backup or share a setup, and they come back in through
// the same Merge, plan and digest as a node policy (MergeOptions.
// SettingsOnly). Nothing personal is in one: node.json holds no key, seed,
// passphrase or RPC token, and the wallet is a file of its own.
//
// A file made on one machine may not fit another. MapPaths points its
// local paths at this machine's, and Fit names what may not fit, so the
// preview says it before anything is saved.

// ExportSettings returns p's settings as they go into a settings bundle:
// without what it follows, and without machine-local settings unless
// full.
func ExportSettings(p Policy, full bool) Policy {
	return settingsPart(ExportPolicy(p, full))
}

// settingsPart returns p without the parts Followed names.
func settingsPart(p Policy) Policy {
	p = clone(p)
	p.Series, p.Follow, p.Issuers.Accept = nil, nil, nil
	return p
}

// isSettingsOnly reports whether p holds no part Followed names.
func isSettingsOnly(p Policy) bool {
	return len(p.Series) == 0 && len(p.Follow) == 0 && len(p.Issuers.Accept) == 0
}

// PathMap rewrites a local path that starts with From to start with To
// instead (MergeOptions.MapPaths).
type PathMap struct {
	From string `json:"from"`
	To   string `json:"to"`
}

// ParsePathMap reads OLD=NEW, as poo settings import -map-path takes it.
func ParsePathMap(s string) (PathMap, error) {
	from, to, ok := strings.Cut(s, "=")
	if !ok || from == "" || to == "" {
		return PathMap{}, fmt.Errorf("a path map is OLD=NEW, both set, not %q", s)
	}
	return PathMap{From: from, To: to}, nil
}

// apply rewrites loc when From starts it at a path boundary. A From
// written the Windows way (a drive letter or a backslash) matches in any
// case and with either separator, as Windows does. The rest of the path
// is joined to To with one separator, To's own (a backslash when To is
// written the Windows way, else a slash), so /home/alice/=/srv/bob gives
// /srv/bob/reg/a.json, not /srv/bobreg/a.json (RED-055, D156).
func (m PathMap) apply(loc string) (string, bool) {
	from := m.From
	var rest string
	if windowsPath(from) {
		if len(loc) < len(from) || !strings.EqualFold(slashed(loc[:len(from)]), slashed(from)) {
			return loc, false
		}
		rest = loc[len(from):]
	} else {
		var ok bool
		if rest, ok = strings.CutPrefix(loc, from); !ok {
			return loc, false
		}
	}
	if rest != "" && !isSep(from[len(from)-1]) && !isSep(rest[0]) {
		// /data must not rewrite /database.
		return loc, false
	}
	rest = strings.TrimLeft(rest, `/\`)
	if rest == "" {
		return m.To, true
	}
	sep := "/"
	if windowsPath(m.To) {
		sep = `\`
	}
	rest = strings.NewReplacer("/", sep, `\`, sep).Replace(rest)
	to := strings.TrimRight(m.To, `/\`)
	if to == "" && sep == "/" {
		return "/" + rest, true
	}
	return to + sep + rest, true
}

func isSep(c byte) bool { return c == '/' || c == '\\' }

// slashed returns p with its backslashes as slashes.
func slashed(p string) string { return strings.ReplaceAll(p, `\`, "/") }

// windowsPath reports whether p is written the Windows way: with a drive
// letter (C:) or a backslash.
func windowsPath(p string) bool {
	if len(p) >= 2 && p[1] == ':' && (p[0]|0x20 >= 'a' && p[0]|0x20 <= 'z') {
		return true
	}
	return strings.Contains(p, `\`)
}

// mapPaths returns p with its local paths rewritten: the registry files
// (an http or https registry is not a path). mapped holds the rewritten
// locations: a registry file the user pointed at this machine is taken
// without -full (RED-055, D156).
func mapPaths(p Policy, maps []PathMap) (out Policy, mapped map[string]bool) {
	if len(maps) == 0 || len(p.P2P.Registries) == 0 {
		return p, nil
	}
	p = clone(p)
	mapped = map[string]bool{}
	for i, r := range p.P2P.Registries {
		if isURL(r) {
			continue
		}
		for _, m := range maps {
			if to, ok := m.apply(r); ok {
				p.P2P.Registries[i] = to
				mapped[to] = true
				break
			}
		}
	}
	return p, mapped
}

// Machine describes the machine an import lands on, for Fit. A field left
// zero is not checked.
type Machine struct {
	// CPUs is how many threads the machine runs at once.
	CPUs int
	// Devices are the grinding device ids it has ("cpu", "gpu:0").
	Devices []string
	// Addrs are the IP addresses of its interfaces.
	Addrs []net.IP
	// Exists reports whether a local file is there.
	Exists func(path string) bool
}

// Fit lists what after has, and before did not, that may not fit m: a
// grinding device or a thread count the machine lacks, a registry file it
// does not have, a listen address that is none of its own, an advertised
// address made on another machine, and a grind job for a series this node
// policy does not list. It changes nothing and refuses nothing: the
// preview shows it (Plan.Fit) so the user can fix it first.
func Fit(before, after Policy, m Machine) []string {
	had := map[string]bool{}
	for _, l := range fitLines(before, m) {
		had[l] = true
	}
	out := []string{}
	for _, l := range fitLines(after, m) {
		if !had[l] {
			out = append(out, l)
		}
	}
	if after.P2P.Advertise != "" && after.P2P.Advertise != before.P2P.Advertise {
		out = append(out, fmt.Sprintf("p2p.advertise: %s is where peers reach the machine it was made on; check it reaches this one", after.P2P.Advertise))
	}
	return out
}

// fitLines lists everything in p that may not fit m.
func fitLines(p Policy, m Machine) []string {
	out := []string{}
	device := func(field, id string) {
		if id == "" || id == "auto" || len(m.Devices) == 0 || slices.Contains(m.Devices, id) {
			return
		}
		out = append(out, fmt.Sprintf("%s: no device %q here (poo grind devices lists them)", field, id))
	}
	threads := func(field string, n int) {
		if m.CPUs > 0 && n > m.CPUs {
			out = append(out, fmt.Sprintf("%s: %d, but this machine runs %d at once", field, n, m.CPUs))
		}
	}
	if g := p.Grind; g != nil {
		device("grind.device", g.Device)
		threads("grind.threads", g.Threads)
		ids := map[hash.Hash]bool{}
		for _, si := range p.Series {
			if s, err := series.New(si.Params); err == nil {
				ids[s.ID()] = true
			}
		}
		for i, j := range g.Jobs {
			field := fmt.Sprintf("grind.jobs[%d] (%s)", i, j.Target)
			device(field+".device", j.Device)
			threads(field+".threads", j.Threads)
			if j.Series != nil && !ids[*j.Series] {
				out = append(out, fmt.Sprintf("%s: series %s is not listed in this node policy, so the job finds no work", field, j.Series.Short()))
			}
		}
	}
	if m.Exists != nil {
		for _, r := range p.P2P.Registries {
			switch {
			case isURL(r):
			case networkPath(r):
				// A preview reaches nothing: opening \\host\share would
				// connect to host, and on Windows offer it the user's
				// sign-in (RED-053, D156).
				out = append(out, fmt.Sprintf("p2p.registries[%s]: a network or device path; the preview does not open it, the node will when it reads the registry", r))
			case !m.Exists(r):
				out = append(out, fmt.Sprintf("p2p.registries[%s]: no such file here (-map-path OLD=NEW rewrites where the file's paths point)", r))
			}
		}
	}
	if len(m.Addrs) > 0 {
		for _, l := range []struct{ field, addr string }{{"rpc.listen", p.RPC.Listen}, {"p2p.listen", p.P2P.Listen}} {
			if l.addr == "" || l.addr == "off" {
				continue
			}
			host, _, err := net.SplitHostPort(l.addr)
			if err != nil || host == "" {
				continue
			}
			ip := net.ParseIP(host)
			if ip == nil || ip.IsUnspecified() || ip.IsLoopback() {
				continue
			}
			if !slices.ContainsFunc(m.Addrs, ip.Equal) {
				out = append(out, fmt.Sprintf("%s: %s is not an address of this machine, so the node cannot listen there", l.field, l.addr))
			}
		}
	}
	return out
}

// networkPath reports whether a local registry path names another machine
// or a device rather than a file here: a UNC path (\\host\share, //host/share)
// or a Windows device path (\\.\, \\?\). Fit does not open one.
func networkPath(p string) bool {
	if len(p) < 2 {
		return false
	}
	sep := func(c byte) bool { return c == '/' || c == '\\' }
	return sep(p[0]) && sep(p[1])
}
