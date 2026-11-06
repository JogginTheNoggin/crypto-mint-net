// Package peerport writes a node's peer book to one shareable file and
// reads one back (decision D157): every address it knows, or the ones in a
// chosen set of categories, with the mint policies each serves, the
// owner's labels, and the blocks the node holds. A user can then give a
// new node somewhere to start, keep a backup, or share a block list.
//
// An import changes nothing about how this node judges a peer:
//
//   - every imported address is unlisted (peers.Unlisted): in none of the
//     node's categories and not a candidate, whatever category the file
//     names, so a file never makes a priority or backup peer. The node
//     tries it later, as any other address, and it earns a category by
//     the node's own checks and points;
//   - blocks are applied only when asked (Options.Blocks), and a range
//     block only with Options.Ranges. A file never unblocks or shortens a
//     block, never blocks an address the node policy lists (p2p.peers),
//     and never touches points, flags or measures, which it does not
//     carry;
//   - the book's own guards hold: a full book or network range refuses
//     more, a blocked address is refused, and a local address is refused
//     unless the node allows local peers.
//
// Nothing here is consensus: the peer book and the tracker are node
// policy, local to one node.
package peerport

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/brand"
	"io"
	"net"
	"net/netip"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tracking"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wire"
)

// Kind marks a peer file.
const Kind = brand.KindPrefix + "peers"

// Format is the version of the layout this code reads and writes.
const Format = 1

// MaxSize bounds a peer file, MaxPeers its addresses, MaxBlocks its
// blocks and MaxPolicies the mint policies of one address.
const (
	MaxSize     = 16 << 20
	MaxPeers    = 1 << 16
	MaxBlocks   = 1 << 16
	MaxPolicies = 256
)

// The categories an export can choose, besides the book states
// (peers.States): the addresses the tracker has blocked or timed out.
const (
	Blocked  = "blocked"
	TimedOut = "timed-out"
)

// Categories lists every category an export can choose.
func Categories() []string {
	out := []string{}
	for _, st := range peers.States {
		out = append(out, st.String())
	}
	return append(out, Blocked, TimedOut)
}

// File is one peer file.
type File struct {
	Kind     string `json:"kind"`
	Format   int    `json:"format"`
	Exported string `json:"exported,omitempty"`
	// Categories are the categories exported; empty means all of them.
	Categories []string `json:"categories,omitempty"`
	Peers      []Peer   `json:"peers"`
	Blocks     []Block  `json:"blocks,omitempty"`
	// Digest is the SHA-256 of the file as read (hex), set by Read. It
	// is not written.
	Digest string `json:"-"`
}

// Peer is one address and the mint policies it serves.
type Peer struct {
	Addr     string   `json:"addr"`
	Policies []Policy `json:"policies"`
	Label    string   `json:"label,omitempty"`
}

// Policy is one mint policy of an address, with its state in the
// exporting book. An import reads the state for the plan only: every
// address arrives unlisted.
type Policy struct {
	Section string `json:"section"`
	State   string `json:"state,omitempty"`
}

// Block is one block the exporting node held: an address (as the tracker
// keys it) or a network range (as peers.Range names it), until when.
type Block struct {
	Addr  string    `json:"addr,omitempty"`
	Range string    `json:"range,omitempty"`
	Until time.Time `json:"until"`
}

// forever is how far off a block "until unblocked" ends (tracking blocks
// it for a hundred years); a block ending later than this from now is
// imported as one until unblocked.
const forever = 50 * 365 * 24 * time.Hour

// Export builds a peer file from a book's records and a tracker's
// statuses and blocked ranges, keeping the categories asked for (none:
// all). Points, flags, measures and node ids are never in it.
func Export(recs []peers.Record, tracked map[string]tracking.Status, ranges map[string]time.Time, categories []string, now time.Time) (File, error) {
	want := map[string]bool{}
	for _, c := range categories {
		if !slices.Contains(Categories(), c) {
			return File{}, fmt.Errorf("peerport: no category %q (%s)", c, strings.Join(Categories(), ", "))
		}
		want[c] = true
	}
	all := len(want) == 0
	f := File{Kind: Kind, Format: Format, Exported: now.UTC().Format(time.RFC3339), Peers: []Peer{}}
	if !all {
		f.Categories = slices.Sorted(func(yield func(string) bool) {
			for c := range want {
				if !yield(c) {
					return
				}
			}
		})
	}
	status := func(addr string) tracking.Status {
		if st, ok := tracked[trackKey(addr)]; ok {
			return st
		}
		return tracked[addr]
	}
	for _, r := range recs {
		st := status(r.Addr)
		keep := all || (want[TimedOut] && st.TimedOut) || (want[Blocked] && st.Blocked)
		var ps []Policy
		for _, p := range r.Policies {
			if all || want[p.State.String()] {
				keep = true
			}
			ps = append(ps, Policy{Section: p.Text, State: p.State.String()})
		}
		if !keep || len(ps) == 0 {
			continue
		}
		f.Peers = append(f.Peers, Peer{Addr: r.Addr, Policies: ps, Label: r.Label})
	}
	if all || want[Blocked] {
		for _, k := range sortedKeys(tracked) {
			if st := tracked[k]; st.Blocked {
				f.Blocks = append(f.Blocks, Block{Addr: k, Until: st.BlockedUntil.UTC()})
			}
		}
		for _, k := range sortedKeys(ranges) {
			if ranges[k].After(now) && !strings.HasPrefix(k, "loopback:") {
				f.Blocks = append(f.Blocks, Block{Range: k, Until: ranges[k].UTC()})
			}
		}
	}
	return f, nil
}

// trackKey is the tracker's key for addr when it is a plain IP:port (see
// tracking.Tracker.Key): the IP, except on loopback.
func trackKey(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	ip := net.ParseIP(host)
	switch {
	case ip == nil:
		return strings.ToLower(host)
	case ip.IsLoopback():
		return addr
	}
	return ip.String()
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// Write writes the file as indented JSON.
func (f File) Write(w io.Writer) error {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(b, '\n'))
	return err
}

// ErrFile marks a file that is not a peer file this node can read.
var ErrFile = errors.New("peerport: not a peer file")

// Read reads a peer file: at most MaxSize, of this Kind and Format, with
// no field this version does not know, and checked (Check).
func Read(r io.Reader) (File, error) {
	b, err := io.ReadAll(io.LimitReader(r, MaxSize+1))
	if err != nil {
		return File{}, err
	}
	if len(b) > MaxSize {
		return File{}, fmt.Errorf("%w: over %d MiB", ErrFile, MaxSize>>20)
	}
	var head struct {
		Kind   string `json:"kind"`
		Format int    `json:"format"`
	}
	if err := json.Unmarshal(b, &head); err != nil {
		return File{}, fmt.Errorf("%w: %v", ErrFile, err)
	}
	if head.Kind != Kind {
		return File{}, fmt.Errorf("%w: kind %q, want %q", ErrFile, head.Kind, Kind)
	}
	if head.Format != Format {
		return File{}, fmt.Errorf("%w: format %d; this node reads format %d", ErrFile, head.Format, Format)
	}
	var f File
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&f); err != nil {
		return File{}, fmt.Errorf("%w: %v", ErrFile, err)
	}
	if dec.More() {
		return File{}, fmt.Errorf("%w: more than one JSON value", ErrFile)
	}
	sum := sha256.Sum256(b)
	f.Digest = hex.EncodeToString(sum[:])
	return f, f.Check()
}

// Check checks the file's bounds and that every address, mint policy,
// label and block can be read. A bad entry refuses the whole file, so
// nothing in it is half applied.
func (f File) Check() error {
	if len(f.Peers) > MaxPeers {
		return fmt.Errorf("%w: over %d addresses", ErrFile, MaxPeers)
	}
	if len(f.Blocks) > MaxBlocks {
		return fmt.Errorf("%w: over %d blocks", ErrFile, MaxBlocks)
	}
	for _, c := range f.Categories {
		if !slices.Contains(Categories(), c) {
			return fmt.Errorf("%w: no category %q", ErrFile, c)
		}
	}
	for i, p := range f.Peers {
		if !plainText(p.Addr) {
			return fmt.Errorf("%w: address %d has a space or control character", ErrFile, i)
		}
		if err := wire.CheckAddr(p.Addr); err != nil {
			return fmt.Errorf("%w: address %d: %v", ErrFile, i, err)
		}
		if len(p.Policies) == 0 || len(p.Policies) > MaxPolicies {
			return fmt.Errorf("%w: address %d: 1 to %d mint policies", ErrFile, i, MaxPolicies)
		}
		for _, pp := range p.Policies {
			if _, err := wire.ParseSection(pp.Section); err != nil {
				return fmt.Errorf("%w: address %d: %v", ErrFile, i, err)
			}
			if pp.State != "" && !slices.ContainsFunc(peers.States, func(s peers.State) bool { return s.String() == pp.State }) {
				return fmt.Errorf("%w: address %d: no state %q", ErrFile, i, pp.State)
			}
		}
		if err := peers.CheckLabel(p.Label); err != nil {
			return fmt.Errorf("%w: address %d: %v", ErrFile, i, err)
		}
	}
	for i, b := range f.Blocks {
		if !plainText(b.Addr) || !plainText(b.Range) {
			return fmt.Errorf("%w: block %d has a space or control character", ErrFile, i)
		}
		if (b.Addr == "") == (b.Range == "") {
			return fmt.Errorf("%w: block %d names exactly one of addr and range", ErrFile, i)
		}
		if b.Addr != "" {
			if _, err := blockAddr(b.Addr); err != nil {
				return fmt.Errorf("%w: block %d: %v", ErrFile, i, err)
			}
		}
		if b.Range != "" {
			if _, err := rangeAddr(b.Range); err != nil {
				return fmt.Errorf("%w: block %d: %v", ErrFile, i, err)
			}
		}
	}
	return nil
}

// blockAddr checks a blocked address as the tracker keys it: an IP, a
// host name, or host:port.
func blockAddr(a string) (string, error) {
	if wire.CheckAddr(a) == nil {
		return a, nil
	}
	if _, err := netip.ParseAddr(a); err == nil {
		return a, nil
	}
	if wire.CheckAddr(a+":1") == nil {
		return a, nil
	}
	return "", fmt.Errorf("address %q", printable(a))
}

// rangeAddr returns an address inside a range named as peers.Range names
// one, for tracking's BlockRange.
func rangeAddr(rng string) (string, error) {
	kind, v, ok := strings.Cut(rng, ":")
	if ok {
		switch kind {
		case "v4":
			var a, b uint8
			if n, err := fmt.Sscanf(v, "%d.%d", &a, &b); err == nil && n == 2 && fmt.Sprintf("%d.%d", a, b) == v {
				return fmt.Sprintf("%d.%d.0.0", a, b), nil
			}
		case "v6":
			if raw, err := hex.DecodeString(v); err == nil && len(raw) == 4 {
				var ip [16]byte
				copy(ip[:], raw)
				return netip.AddrFrom16(ip).String(), nil
			}
		case "host":
			if wire.CheckAddr(v+":1") == nil {
				return v, nil
			}
		}
	}
	return "", fmt.Errorf("network range %q", printable(rng))
}

// plainText reports whether s has no space or control character, as an
// address never does, so a file's text cannot rewrite the terminal a plan
// is printed on (RED-053).
func plainText(s string) bool {
	for _, r := range s {
		if unicode.IsSpace(r) || unicode.IsControl(r) || !unicode.IsPrint(r) {
			return false
		}
	}
	return utf8.ValidString(s)
}

// printable quotes s with its control characters escaped, for an error
// or a plan line (RED-053).
func printable(s string) string {
	q := fmt.Sprintf("%q", s)
	return q[1 : len(q)-1]
}
