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

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/policy"
)

// SeedSize is the size of a wallet seed. Every key of the wallet derives
// from it, so it is the one secret to back up.
const SeedSize = 32

// DefaultIterations is the PBKDF2-SHA256 work factor sealing a new wallet's
// seed (decision D64).
const DefaultIterations = 600_000

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
	Name       string `json:"name"` // "pbkdf2-sha256"
	Iterations int    `json:"iterations"`
	Salt       Hex    `json:"salt"`
}

// sealAAD binds a sealed seed to this use, so a ciphertext made for
// something else cannot be passed off as one.
var sealAAD = []byte("PoO-wallet-seed-v1")

func (k KDF) key(passphrase string) ([]byte, error) {
	if k.Name != "pbkdf2-sha256" {
		return nil, fmt.Errorf("wallet: unknown key derivation %q", k.Name)
	}
	if k.Iterations < 1 || len(k.Salt) < 16 {
		return nil, errors.New("wallet: bad key derivation settings")
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
// AES-256-GCM, its key from PBKDF2-SHA256. The result is nonce ∥
// ciphertext ∥ tag.
func seal(seed []byte, passphrase string, iterations int) (KDF, Hex, error) {
	k := KDF{Name: "pbkdf2-sha256", Iterations: iterations, Salt: make([]byte, 16)}
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
	m.Write([]byte("PoO-wallet-" + a.Name() + "-v1"))
	var b [4]byte
	binary.BigEndian.PutUint32(b[:], index)
	m.Write(b[:])
	return policy.NewSigner(alg, m.Sum(nil))
}
