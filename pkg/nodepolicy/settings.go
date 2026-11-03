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

// apply rewrites loc when From starts it at a path boundary.
func (m PathMap) apply(loc string) (string, bool) {
	rest, ok := strings.CutPrefix(loc, m.From)
	if !ok {
		return loc, false
	}
	if rest != "" && !strings.HasSuffix(m.From, "/") && !strings.HasSuffix(m.From, `\`) && rest[0] != '/' && rest[0] != '\\' {
		// /data must not rewrite /database.
		return loc, false
	}
	return m.To + rest, true
}

// Maps reports whether m rewrites loc, so a -map-path that matches no
// path of a file can be named (GREEN-045).
func (m PathMap) Maps(loc string) bool {
	if isURL(loc) {
		return false // mapPaths never rewrites a registry URL
	}
	_, ok := m.apply(loc)
	return ok
}

// mapPaths returns p with its local paths rewritten: the registry files
// (an http or https registry is not a path).
func mapPaths(p Policy, maps []PathMap) Policy {
	if len(maps) == 0 || len(p.P2P.Registries) == 0 {
		return p
	}
	p = clone(p)
	for i, r := range p.P2P.Registries {
		if isURL(r) {
			continue
		}
		for _, m := range maps {
			if to, ok := m.apply(r); ok {
				p.P2P.Registries[i] = to
				break
			}
		}
	}
	return p
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
			if !isURL(r) && !m.Exists(r) {
				out = append(out, fmt.Sprintf("p2p.registries[%s]: no such file here (-map-path OLD=NEW points it at one)", r))
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
