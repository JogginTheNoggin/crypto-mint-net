package wallet

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"

	"golang.org/x/crypto/argon2"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
)

// SeedSize is the size of a wallet seed. Every key of the wallet derives
// from it, so it is the one secret to back up.
const SeedSize = 32

// The key derivations a wallet file can name. New wallets use Argon2id
// (decision D95); PBKDF2-SHA256 wallets (decision D64) still open.
const (
	KDFArgon2id = "argon2id"
	KDFPBKDF2   = "pbkdf2-sha256"
)

// DefaultIterations is the PBKDF2-SHA256 work factor of a wallet sealed
// with PBKDF2 (decision D64).
const DefaultIterations = 600_000

// The Argon2id settings sealing a new wallet's seed: RFC 9106's second
// recommended option, 3 passes over 64 MiB with 4 lanes (decision D95).
const (
	DefaultArgonPasses  = 3
	DefaultArgonMemory  = 64 * 1024 // KiB
	DefaultArgonThreads = 4
)

// Limits on the settings a wallet file may name, so a damaged or crafted
// file cannot make opening it take minutes or exhaust a modest machine's
// memory (decisions D95, D97). Argon2id may use at most 1 GiB, and its
// passes times its memory at most 8 GiB, so 1 GiB allows 8 passes and the
// default 64 MiB allows 100.
const (
	maxIterations  = 10_000_000
	maxArgonPasses = 100
	maxArgonMemory = 1 << 20 // KiB, 1 GiB
	maxArgonWork   = 8 << 20 // passes × KiB
)

// ErrPassphrase is returned when a passphrase does not open the seed.
var ErrPassphrase = errors.New("wallet: wrong passphrase")

// Hex is bytes written as hex in JSON.
type Hex []byte

// MarshalText writes the bytes as hex.
func (h Hex) MarshalText() ([]byte, error) { return []byte(hex.EncodeToString(h)), nil }

// UnmarshalText reads hex.
func (h *Hex) UnmarshalText(b []byte) error {
	out, err := hex.DecodeString(string(b))
	if err != nil {
		return err
	}
	*h = out
	return nil
}

// KDF says how the key that seals the seed comes from the passphrase.
type KDF struct {
	Name string `json:"name"` // KDFArgon2id or KDFPBKDF2
	// Iterations is the PBKDF2 work factor, or the Argon2id passes.
	Iterations int `json:"iterations"`
	// Memory (KiB) and Threads are Argon2id's; PBKDF2 has neither.
	Memory  uint32 `json:"memory,omitempty"`
	Threads uint8  `json:"threads,omitempty"`
	Salt    Hex    `json:"salt"`
}

// newKDF returns the settings, without a salt, for a derivation named
// name with work factor iterations and, for Argon2id, memory in KiB.
// Zero values mean the defaults, and name "" means Argon2id.
func newKDF(name string, iterations int, memory uint32) (KDF, error) {
	k := KDF{Name: name, Iterations: iterations}
	switch name {
	case "", KDFArgon2id:
		k.Name, k.Memory, k.Threads = KDFArgon2id, memory, DefaultArgonThreads
		if k.Iterations == 0 {
			k.Iterations = DefaultArgonPasses
		}
		if k.Memory == 0 {
			k.Memory = DefaultArgonMemory
		}
	case KDFPBKDF2:
		if k.Iterations == 0 {
			k.Iterations = DefaultIterations
		}
	default:
		return k, fmt.Errorf("wallet: unknown key derivation %q", name)
	}
	return k, k.check()
}

// CheckKDF reports whether o's key derivation settings are within the
// limits a wallet may use, without deriving anything.
func CheckKDF(o Options) error {
	_, err := newKDF(o.KDF, o.Iterations, o.Memory)
	return err
}

// check refuses settings that are unknown, too weak to be meant, or so
// costly that opening the wallet would hang or exhaust memory.
func (k KDF) check() error {
	switch k.Name {
	case KDFPBKDF2:
		if k.Iterations < 1 || k.Iterations > maxIterations || k.Memory != 0 || k.Threads != 0 {
			return errors.New("wallet: bad key derivation settings")
		}
	case KDFArgon2id:
		if k.Iterations < 1 || k.Iterations > maxArgonPasses || k.Threads < 1 ||
			k.Memory < 8*uint32(k.Threads) || k.Memory > maxArgonMemory ||
			uint64(k.Iterations)*uint64(k.Memory) > maxArgonWork {
			return errors.New("wallet: bad key derivation settings")
		}
	default:
		return fmt.Errorf("wallet: unknown key derivation %q", k.Name)
	}
	return nil
}

// sealAAD binds a sealed seed to this use, so a ciphertext made for
// something else cannot be passed off as one.
var sealAAD = []byte("PoO-wallet-seed-v1")

func (k KDF) key(passphrase string) ([]byte, error) {
	if err := k.check(); err != nil {
		return nil, err
	}
	if len(k.Salt) < 16 {
		return nil, errors.New("wallet: bad key derivation settings")
	}
	if k.Name == KDFArgon2id {
		return argon2.IDKey([]byte(passphrase), k.Salt, uint32(k.Iterations), k.Memory, k.Threads, 32), nil
	}
	return pbkdf2.Key(sha256.New, passphrase, k.Salt, k.Iterations, 32)
}

func newAEAD(key []byte) (cipher.AEAD, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}

// seal encrypts seed under passphrase with a fresh salt and nonce:
// AES-256-GCM, its key from k's derivation. The result is nonce ∥
// ciphertext ∥ tag.
func seal(seed []byte, passphrase string, k KDF) (KDF, Hex, error) {
	k.Salt = make([]byte, 16)
	if _, err := rand.Read(k.Salt); err != nil {
		return k, nil, err
	}
	key, err := k.key(passphrase)
	if err != nil {
		return k, nil, err
	}
	aead, err := newAEAD(key)
	if err != nil {
		return k, nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return k, nil, err
	}
	return k, aead.Seal(nonce, nonce, seed, sealAAD), nil
}

// open decrypts a sealed seed.
func open(k KDF, sealed Hex, passphrase string) ([]byte, error) {
	key, err := k.key(passphrase)
	if err != nil {
		return nil, err
	}
	aead, err := newAEAD(key)
	if err != nil {
		return nil, err
	}
	if len(sealed) < aead.NonceSize() {
		return nil, errors.New("wallet: sealed seed too short")
	}
	seed, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], sealAAD)
	if err != nil {
		return nil, ErrPassphrase
	}
	if len(seed) != SeedSize {
		return nil, errors.New("wallet: sealed seed has the wrong size")
	}
	return seed, nil
}

// derive returns the signer of algorithm alg for key index of seed. The
// child seed is HMAC-SHA256(seed, label ∥ index as U32), with label
// "PoO-wallet-ed25519-v1" for Ed25519 (decision D63, unchanged) and
// "PoO-wallet-<algorithm name>-v1" for the others; policy.NewSigner turns
// it into the key (decision D81). A wallet's keys are of one algorithm,
// so the same seed under two algorithms gives unrelated keys.
func derive(seed []byte, alg policy.AlgID, index uint32) (policy.Signer, error) {
	a, ok := policy.Default().Alg(alg)
	if !ok {
		return nil, fmt.Errorf("wallet: unknown key algorithm %d", alg)
	}
	m := hmac.New(sha256.New, seed)
	// The label never follows a rename (see pkg/brand): it fixes every key.
	m.Write([]byte("PoO-wallet-" + a.Name() + "-v1"))
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], index)
	m.Write(b[:])
	return policy.NewSigner(alg, m.Sum(nil))
}
