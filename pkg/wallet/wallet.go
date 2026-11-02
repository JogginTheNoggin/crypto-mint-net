// Package wallet keeps a holder's keys and spends their portions: one seed
// sealed with a passphrase, fresh single-key addresses derived from it,
// shared k-of-n policies, payments built and signed from the portions the
// wallet holds, payment requests a payer can check, and the weight behind
// every payment sent or expected (the design's sections 7 and 17.6).
//
// Everything here is a wallet habit, never a protocol rule: the protocol
// sees only transactions and witnesses. The wallet talks to a full node
// through the Node interface, so it runs against a local node, a remote
// one, or an engine in a test, and never decides validity itself.
//
// A Wallet is not safe for concurrent use.
package wallet

import (
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/series"
)

// FileName is a wallet's default file name.
const FileName = "wallet.json"

// DefaultGap is how many unused keys past the last used one a wallet
// derives and watches (decision D63).
const DefaultGap = 20

// Errors.
var (
	ErrLocked  = errors.New("wallet: locked; unlock it with its passphrase")
	ErrExists  = errors.New("wallet: file exists")
	ErrNoFunds = errors.New("wallet: not enough spendable units")
)

// KeyInfo is one derived key. Public keys are kept in the clear so the
// wallet can list and watch its addresses while locked.
type KeyInfo struct {
	Index  uint32 `json:"index"`
	Public Hex    `json:"public"`
	// Label says what the key's address was given out for.
	Label string `json:"label,omitempty"`
	// Used: the address was given out (for a payment, as change, or in a
	// policy).
	Used bool `json:"used,omitempty"`
}

// PolicyKey is one key of a shared policy: one of this wallet's (Wallet
// names its index) or someone else's public key.
type PolicyKey struct {
	Wallet *uint32 `json:"wallet,omitempty"`
	Alg    uint32  `json:"alg"`
	Public Hex     `json:"public"`
}

// PolicyInfo is a k-of-n policy the wallet holds keys of.
type PolicyInfo struct {
	Name  string      `json:"name"`
	K     uint16      `json:"k"`
	Keys  []PolicyKey `json:"keys"`
	Label string      `json:"label,omitempty"`
}

// Params returns the policy's threshold params.
func (p PolicyInfo) Params() policy.ThresholdParams {
	tp := policy.ThresholdParams{K: p.K}
	for _, k := range p.Keys {
		tp.Keys = append(tp.Keys, policy.Key{Alg: policy.AlgID(k.Alg), Public: k.Public})
	}
	return tp
}

// Watch lists the mints of one series the wallet looks for its portions
// in.
type Watch struct {
	Series  hash.Hash `json:"series"`
	Serials []uint64  `json:"serials"`
}

// File is the wallet file.
type File struct {
	Format   int          `json:"format"`
	KDF      KDF          `json:"kdf"`
	Sealed   Hex          `json:"sealed"`
	Keys     []KeyInfo    `json:"keys"`
	Gap      int          `json:"gap"`
	Policies []PolicyInfo `json:"policies,omitempty"`
	Watch    []Watch      `json:"watch,omitempty"`
	Sent     []*Sent      `json:"sent,omitempty"`
	Expected []*Expected  `json:"expected,omitempty"`
}

// Wallet is an open wallet.
type Wallet struct {
	path string
	f    File
	seed []byte // nil while locked
}

// Options are the settings of a new wallet. Zero values mean the defaults.
type Options struct {
	// Seed restores a wallet from its seed; nil makes a new random one.
	Seed []byte
	// Iterations is the PBKDF2 work factor.
	Iterations int
	// Gap is how many unused keys to keep ahead.
	Gap int
}

// Create makes a new wallet at path, sealed with passphrase, and returns it
// unlocked. It refuses to overwrite a file.
func Create(path, passphrase string, o Options) (*Wallet, error) {
	if _, err := os.Stat(path); err == nil {
		return nil, ErrExists
	}
	seed := o.Seed
	if seed == nil {
		seed = make([]byte, SeedSize)
		if _, err := rand.Read(seed); err != nil {
			return nil, err
		}
	}
	if len(seed) != SeedSize {
		return nil, fmt.Errorf("wallet: a seed is %d bytes", SeedSize)
	}
	if o.Iterations == 0 {
		o.Iterations = DefaultIterations
	}
	if o.Gap <= 0 {
		o.Gap = DefaultGap
	}
	kdf, sealed, err := seal(seed, passphrase, o.Iterations)
	if err != nil {
		return nil, err
	}
	w := &Wallet{path: path, f: File{Format: 1, KDF: kdf, Sealed: sealed, Gap: o.Gap}, seed: append([]byte(nil), seed...)}
	w.topUp()
	return w, w.Save()
}

// Open reads the wallet at path, locked.
func Open(path string) (*Wallet, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	w := &Wallet{path: path}
	if err := json.Unmarshal(b, &w.f); err != nil {
		return nil, fmt.Errorf("wallet: %s: %w", path, err)
	}
	if w.f.Format != 1 {
		return nil, fmt.Errorf("wallet: %s: unknown format %d", path, w.f.Format)
	}
	if w.f.Gap <= 0 {
		w.f.Gap = DefaultGap
	}
	return w, nil
}

// Path returns the wallet's file.
func (w *Wallet) Path() string { return w.path }

// Unlock opens the seed with passphrase and derives any keys missing
// from the gap.
func (w *Wallet) Unlock(passphrase string) error {
	seed, err := open(w.f.KDF, w.f.Sealed, passphrase)
	if err != nil {
		return err
	}
	w.seed = seed
	w.topUp()
	return nil
}

// Lock forgets the seed.
func (w *Wallet) Lock() {
	for i := range w.seed {
		w.seed[i] = 0
	}
	w.seed = nil
}

// Unlocked reports whether the seed is open.
func (w *Wallet) Unlocked() bool { return w.seed != nil }

// Seed returns a copy of the seed, for a backup.
func (w *Wallet) Seed() ([]byte, error) {
	if w.seed == nil {
		return nil, ErrLocked
	}
	return append([]byte(nil), w.seed...), nil
}

// Rekey seals the seed again under a new passphrase.
func (w *Wallet) Rekey(passphrase string) error {
	if w.seed == nil {
		return ErrLocked
	}
	kdf, sealed, err := seal(w.seed, passphrase, w.f.KDF.Iterations)
	if err != nil {
		return err
	}
	w.f.KDF, w.f.Sealed = kdf, sealed
	return nil
}

// Save writes the wallet atomically, readable only by its owner.
func (w *Wallet) Save() error {
	b, err := json.MarshalIndent(w.f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(w.path), 0o700); err != nil {
		return err
	}
	tmp := w.path + ".tmp"
	if err := os.WriteFile(tmp, append(b, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, w.path)
}

// topUp derives keys until Gap unused ones follow the last used one. It
// does nothing while locked.
func (w *Wallet) topUp() {
	if w.seed == nil {
		return
	}
	last := -1
	for i, k := range w.f.Keys {
		if k.Used {
			last = i
		}
	}
	for len(w.f.Keys) < last+1+w.f.Gap {
		i := uint32(len(w.f.Keys))
		w.f.Keys = append(w.f.Keys, KeyInfo{Index: i, Public: derive(w.seed, i).Public()})
	}
}

// signer returns the signer for key index; the wallet must be unlocked.
func (w *Wallet) signer(index uint32) (policy.Signer, error) {
	if w.seed == nil {
		return nil, ErrLocked
	}
	return derive(w.seed, index), nil
}

// takeKey marks the first unused key as used for label and returns it.
func (w *Wallet) takeKey(label string) (KeyInfo, error) {
	for i := range w.f.Keys {
		if !w.f.Keys[i].Used {
			w.f.Keys[i].Used, w.f.Keys[i].Label = true, label
			k := w.f.Keys[i]
			w.topUp()
			return k, nil
		}
	}
	if w.seed == nil {
		return KeyInfo{}, ErrLocked
	}
	w.topUp()
	return w.takeKey(label)
}

// Keys returns the derived keys.
func (w *Wallet) Keys() []KeyInfo { return append([]KeyInfo(nil), w.f.Keys...) }

// Owner says what in the wallet controls an address: a key or a policy.
type Owner struct {
	Address hash.Hash     `json:"address"`
	Policy  policy.Policy `json:"-"`
	// Key is the index of a single-key address; Shared names a policy.
	Key    *uint32 `json:"key,omitempty"`
	Shared string  `json:"policy,omitempty"`
	Label  string  `json:"label,omitempty"`
	Used   bool    `json:"used,omitempty"`
}

// keyPolicy is the 1-of-1 policy of a derived key.
func keyPolicy(k KeyInfo) policy.Policy { return policy.SingleKey(policy.AlgEd25519, k.Public) }

// Owners returns everything the wallet can spend from in series s: each
// derived key's address (the unused gap included, so payments to them are
// found) and each shared policy's.
func (w *Wallet) Owners(s *series.Series) map[hash.Hash]Owner {
	out := map[hash.Hash]Owner{}
	for _, k := range w.f.Keys {
		p := keyPolicy(k)
		a := s.Address(p)
		idx := k.Index
		out[a] = Owner{Address: a, Policy: p, Key: &idx, Label: k.Label, Used: k.Used}
	}
	for _, pi := range w.f.Policies {
		p := pi.Params().Policy()
		a := s.Address(p)
		out[a] = Owner{Address: a, Policy: p, Shared: pi.Name, Label: pi.Label, Used: true}
	}
	return out
}

// NewAddress gives out a fresh single-key address of series s for label,
// the design's habit of one address per payment.
func (w *Wallet) NewAddress(s *series.Series, label string) (Owner, error) {
	k, err := w.takeKey(label)
	if err != nil {
		return Owner{}, err
	}
	p := keyPolicy(k)
	a := s.Address(p)
	idx := k.Index
	return Owner{Address: a, Policy: p, Key: &idx, Label: label, Used: true}, nil
}

// AddPolicy records a k-of-n policy. Keys with Wallet set to nil and no
// public key take a fresh key of this wallet; the others are kept as
// given. It returns the policy, which the caller can check against a
// series with CheckPolicy.
func (w *Wallet) AddPolicy(name string, k uint16, keys []PolicyKey, label string) (PolicyInfo, error) {
	if name == "" {
		return PolicyInfo{}, errors.New("wallet: a policy needs a name")
	}
	for _, p := range w.f.Policies {
		if p.Name == name {
			return PolicyInfo{}, fmt.Errorf("wallet: policy %q exists", name)
		}
	}
	pi := PolicyInfo{Name: name, K: k, Label: label}
	for _, pk := range keys {
		switch {
		case pk.Wallet != nil:
			if int(*pk.Wallet) >= len(w.f.Keys) {
				return PolicyInfo{}, fmt.Errorf("wallet: no key %d", *pk.Wallet)
			}
			ki := &w.f.Keys[*pk.Wallet]
			ki.Used = true
			idx := ki.Index
			pk = PolicyKey{Wallet: &idx, Alg: uint32(policy.AlgEd25519), Public: ki.Public}
		case len(pk.Public) == 0:
			ki, err := w.takeKey("policy " + name)
			if err != nil {
				return PolicyInfo{}, err
			}
			idx := ki.Index
			pk = PolicyKey{Wallet: &idx, Alg: uint32(policy.AlgEd25519), Public: ki.Public}
		default:
			if pk.Alg == 0 {
				pk.Alg = uint32(policy.AlgEd25519)
			}
		}
		pi.Keys = append(pi.Keys, pk)
	}
	if _, err := policy.DecodeThresholdParams(policy.Default(), pi.Params().Encode()); err != nil {
		return PolicyInfo{}, err
	}
	w.topUp()
	w.f.Policies = append(w.f.Policies, pi)
	return pi, nil
}

// Policies returns the shared policies.
func (w *Wallet) Policies() []PolicyInfo { return append([]PolicyInfo(nil), w.f.Policies...) }

// CheckPolicy reports whether s allows p and its keys, and this software
// can satisfy it: a payer's check before paying an address it was given
// with its policy (the design's section 17.6).
func CheckPolicy(s *series.Series, p policy.Policy) error {
	if !s.AllowsPolicyType(p.Type) {
		return fmt.Errorf("wallet: series does not allow policy type %d", p.Type)
	}
	if p.Type != policy.TypeThreshold {
		return fmt.Errorf("wallet: policy type %d is not one this wallet recognizes", p.Type)
	}
	tp, err := policy.DecodeThresholdParams(policy.Default(), p.Params)
	if err != nil {
		return err
	}
	for _, k := range tp.Keys {
		if !s.AllowsKeyAlg(k.Alg) {
			return fmt.Errorf("wallet: series does not allow key algorithm %d", k.Alg)
		}
	}
	return nil
}

// WatchMints adds serials of s to the mints the wallet looks in.
func (w *Wallet) WatchMints(s *series.Series, serials ...uint64) error {
	for _, n := range serials {
		if n >= s.Mints() {
			return fmt.Errorf("wallet: serial %d out of range", n)
		}
	}
	var wt *Watch
	for i := range w.f.Watch {
		if w.f.Watch[i].Series == s.ID() {
			wt = &w.f.Watch[i]
		}
	}
	if wt == nil {
		w.f.Watch = append(w.f.Watch, Watch{Series: s.ID()})
		wt = &w.f.Watch[len(w.f.Watch)-1]
	}
	have := map[uint64]bool{}
	for _, n := range wt.Serials {
		have[n] = true
	}
	for _, n := range serials {
		if !have[n] {
			wt.Serials = append(wt.Serials, n)
			have[n] = true
		}
	}
	sort.Slice(wt.Serials, func(i, j int) bool { return wt.Serials[i] < wt.Serials[j] })
	return nil
}

// Watched returns the mints of s the wallet looks in.
func (w *Wallet) Watched(s hash.Hash) []uint64 {
	for _, wt := range w.f.Watch {
		if wt.Series == s {
			return append([]uint64(nil), wt.Serials...)
		}
	}
	return nil
}

// Pools returns the pools of the watched mints of s: what a node serving
// this wallet must follow.
func (w *Wallet) Pools(s *series.Series) []uint64 {
	seen := map[uint64]bool{}
	var out []uint64
	for _, n := range w.Watched(s.ID()) {
		if p := s.PoolOf(n); !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func now() time.Time { return time.Now().UTC().Truncate(time.Second) }
