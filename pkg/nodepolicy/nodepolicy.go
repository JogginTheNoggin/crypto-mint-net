// Package nodepolicy holds a full node's node policy: every choice a node
// makes for itself, as opposed to the rules every node must follow to agree
// on who owns what. That is which series it knows and which issuers it
// accepts, what it follows, how many inactive branches it keeps, how many
// pending transactions it holds, how it writes to disk and where its local
// RPC listens.
//
// A node policy is a JSON file, node.json, in the node's data directory.
// Package nodepolicy also manages several of them side by side, with the
// design's three operations: save (keep it and pick up where it left off),
// archive (keep it apart; using it again starts from scratch) and delete.
// See Manager.
//
// Nothing here changes what is valid. Two nodes with different node
// policies agree on ownership of everything they both follow.
package nodepolicy

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

// Format is the version of the node.json layout this code reads.
const Format = 1

// FileName is the node policy's file name inside a data directory.
const FileName = "node.json"

// DefaultListen is the default address of the local RPC: loopback only.
const DefaultListen = "127.0.0.1:8645"

// ErrInvalid marks a node policy that cannot be used as written.
var ErrInvalid = errors.New("nodepolicy: invalid")

// Policy is one node policy.
type Policy struct {
	Format int `json:"format"`
	// Series lists the series this node knows. Series information comes
	// from outside the network, never from peers.
	Series []SeriesInfo `json:"series"`
	// Issuers says which series are accepted.
	Issuers Issuers `json:"issuers"`
	// Follow lists the mint policies this node follows.
	Follow []Follow `json:"follow"`
	// K is how many inactive branches to keep per pool; nil means the
	// default, -1 keeps every branch.
	K *int `json:"k,omitempty"`
	// Ties is how many branches tying the selected weight to keep beyond K
	// (0: default).
	Ties int `json:"ties,omitempty"`
	// MaxPending is the pending transaction limit per pool (0: default).
	MaxPending int `json:"max_pending,omitempty"`
	// SeenSize is the size of the seen list (0: default).
	SeenSize int `json:"seen_size,omitempty"`
	// NoSync skips the fsync after each write to the store. Faster; a power
	// loss can then lose the latest items, which peers can send again.
	NoSync bool `json:"no_sync,omitempty"`
	// RPC configures the local control interface.
	RPC RPC `json:"rpc"`
}

// SeriesInfo is one known series, with the issuer's witness when the series
// names an issuer.
type SeriesInfo struct {
	Params series.Params `json:"params"`
	// IssuerWitness is the hex of the witness's byte form, signing the
	// series' issuer message. Required for a series accepted by issuer.
	IssuerWitness string `json:"issuer_witness,omitempty"`
}

// Issuers is the rule for which series a node accepts.
type Issuers struct {
	// Any accepts every listed series, including ones that name no issuer.
	// An issuer witness that is present must still be valid.
	Any bool `json:"any"`
	// Accept lists issuer addresses whose series are accepted, each with a
	// valid issuer witness.
	Accept []hash.Hash `json:"accept,omitempty"`
}

// Follow is one mint policy: an issuer, a series, some pools of a series or
// some mints of a series. Exactly one of Issuer and Series is set. A series
// with no Pools and no Mints means the whole series.
type Follow struct {
	Issuer *hash.Hash `json:"issuer,omitempty"`
	Series *hash.Hash `json:"series,omitempty"`
	Pools  []uint64   `json:"pools,omitempty"`
	Mints  []uint64   `json:"mints,omitempty"`
}

// RPC configures the local control interface.
type RPC struct {
	// Listen is the TCP address; empty means DefaultListen, "off" disables
	// the interface.
	Listen string `json:"listen,omitempty"`
	// AllowRemote permits a non-loopback Listen address. The interface
	// controls the node, so this is off by default.
	AllowRemote bool `json:"allow_remote,omitempty"`
}

// Default returns a node policy that knows no series and follows nothing.
// Issuers.Any is on, because there is no project issuer yet (decision D24).
func Default() Policy {
	return Policy{Format: Format, Issuers: Issuers{Any: true}}
}

// LedgerConfig returns the ledger settings this policy asks for.
func (p Policy) LedgerConfig() ledger.Config {
	c := ledger.Config{K: ledger.DefaultK, Ties: p.Ties}
	if p.K != nil {
		c.K = *p.K
	}
	return c
}

// ListenAddr returns the RPC address, or "" when the interface is off.
func (p Policy) ListenAddr() string {
	switch p.RPC.Listen {
	case "":
		return DefaultListen
	case "off":
		return ""
	}
	return p.RPC.Listen
}

// Validate checks the policy for mistakes that would stop a node from
// starting. It does not check issuer witnesses; Accepted does.
func (p Policy) Validate() error {
	if p.Format != Format {
		return fmt.Errorf("%w: format %d, this node reads %d", ErrInvalid, p.Format, Format)
	}
	if p.K != nil && *p.K < ledger.Unlimited {
		return fmt.Errorf("%w: k must be -1 (keep all) or more", ErrInvalid)
	}
	if p.Ties < 0 || p.MaxPending < 0 || p.SeenSize < 0 {
		return fmt.Errorf("%w: limits must not be negative", ErrInvalid)
	}
	for i, f := range p.Follow {
		if (f.Issuer == nil) == (f.Series == nil) {
			return fmt.Errorf("%w: follow %d must name exactly one of issuer and series", ErrInvalid, i)
		}
		if f.Issuer != nil && (len(f.Pools) > 0 || len(f.Mints) > 0) {
			return fmt.Errorf("%w: follow %d: an issuer has no pools or mints", ErrInvalid, i)
		}
	}
	if addr := p.ListenAddr(); addr != "" {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			return fmt.Errorf("%w: rpc listen %q: %v", ErrInvalid, addr, err)
		}
		if !p.RPC.AllowRemote && !isLoopback(host) {
			return fmt.Errorf("%w: rpc listen %q is not a loopback address; set allow_remote to permit it", ErrInvalid, addr)
		}
	}
	return nil
}

func isLoopback(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// Accepted is a known series the node accepts.
type Accepted struct {
	Series *series.Series
	// Issuer is the verified issuer, if the series names one and its
	// witness checked.
	Issuer *hash.Hash
}

// Refused is a listed series the node does not accept, and why.
type Refused struct {
	Index  int
	Reason error
}

// Accepted builds every listed series and applies the issuer rule. A series
// that cannot be built, is unreadable at this node, or is not accepted is
// listed in refused rather than failing the whole policy, so one bad series
// does not stop a node.
func (p Policy) Accepted() (accepted []Accepted, refused []Refused) {
	allow := map[hash.Hash]bool{}
	for _, a := range p.Issuers.Accept {
		allow[a] = true
	}
	for i, si := range p.Series {
		s, err := series.New(si.Params)
		if err != nil {
			refused = append(refused, Refused{i, err})
			continue
		}
		var issuer *hash.Hash
		if si.IssuerWitness != "" {
			w, err := DecodeWitness(si.IssuerWitness)
			if err == nil {
				err = s.CheckIssuer(w)
			}
			if err != nil {
				refused = append(refused, Refused{i, fmt.Errorf("issuer witness: %w", err)})
				continue
			}
			issuer = si.Params.Issuer
		}
		switch {
		case issuer != nil && allow[*issuer]:
		case p.Issuers.Any:
		case issuer == nil && si.Params.Issuer != nil && allow[*si.Params.Issuer]:
			refused = append(refused, Refused{i, errors.New("issuer accepted, but the series has no issuer witness")})
			continue
		default:
			refused = append(refused, Refused{i, errors.New("issuer not accepted")})
			continue
		}
		accepted = append(accepted, Accepted{Series: s, Issuer: issuer})
	}
	return accepted, refused
}

// EncodeWitness returns the hex form of a witness used in node.json.
func EncodeWitness(w policy.Witness) string {
	enc := encoding.NewWriter(w.Size())
	w.Encode(enc)
	return hex.EncodeToString(enc.Bytes())
}

// DecodeWitness reads the hex form written by EncodeWitness.
func DecodeWitness(s string) (policy.Witness, error) {
	b, err := hex.DecodeString(s)
	if err != nil {
		return policy.Witness{}, err
	}
	r := encoding.NewReader(b)
	w := policy.DecodeWitness(r)
	if err := r.Finish(); err != nil {
		return policy.Witness{}, err
	}
	return w, nil
}

// Load reads and validates a node policy file.
func Load(path string) (Policy, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Policy{}, err
	}
	var p Policy
	if err := json.Unmarshal(b, &p); err != nil {
		return Policy{}, fmt.Errorf("%w: %s: %v", ErrInvalid, path, err)
	}
	if err := p.Validate(); err != nil {
		return Policy{}, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

// Save validates p and writes it to path atomically.
func Save(path string, p Policy) error {
	if err := p.Validate(); err != nil {
		return err
	}
	b, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
