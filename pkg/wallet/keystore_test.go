package wallet_test

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wallet"
)

// readKDF returns the key derivation settings written in a wallet file.
func readKDF(t *testing.T, path string) wallet.KDF {
	t.Helper()
	b, err := os.ReadFile(path)
	must(t, err)
	var f wallet.File
	must(t, json.Unmarshal(b, &f))
	return f.KDF
}

// TestNewWalletUsesArgon2id pins decision D95: a wallet made with the
// default options seals its seed with Argon2id at RFC 9106's settings.
func TestNewWalletUsesArgon2id(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	_, err := wallet.Create(path, "secret", wallet.Options{})
	must(t, err)
	k := readKDF(t, path)
	if k.Name != wallet.KDFArgon2id || k.Iterations != wallet.DefaultArgonPasses ||
		k.Memory != wallet.DefaultArgonMemory || k.Threads != wallet.DefaultArgonThreads || len(k.Salt) != 16 {
		t.Fatalf("kdf %+v", k)
	}
	w, err := wallet.Open(path)
	must(t, err)
	if err := w.Unlock("wrong"); !errors.Is(err, wallet.ErrPassphrase) {
		t.Fatalf("wrong passphrase: %v", err)
	}
	must(t, w.Unlock("secret"))
}

// TestPBKDF2WalletOpensAndRekeyUpgrades: wallets sealed before D95 still
// open, and a new passphrase moves them to Argon2id.
func TestPBKDF2WalletOpensAndRekeyUpgrades(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	_, err := wallet.Create(path, "old", wallet.Options{KDF: wallet.KDFPBKDF2, Iterations: 1})
	must(t, err)
	if k := readKDF(t, path); k.Name != wallet.KDFPBKDF2 || k.Memory != 0 || k.Threads != 0 {
		t.Fatalf("kdf %+v", k)
	}
	w, err := wallet.Open(path)
	must(t, err)
	must(t, w.Unlock("old"))
	seed, _ := w.Seed()
	must(t, w.Rekey("new"))
	must(t, w.Save())
	if k := readKDF(t, path); k.Name != wallet.KDFArgon2id || k.Memory != wallet.DefaultArgonMemory {
		t.Fatalf("rekey kept %+v", k)
	}
	w2, err := wallet.Open(path)
	must(t, err)
	must(t, w2.Unlock("new"))
	if got, _ := w2.Seed(); string(got) != string(seed) {
		t.Fatal("seed changed")
	}
}

// TestRekeyKeepsArgon2idSettings: an Argon2id wallet keeps the settings
// its owner chose.
func TestRekeyKeepsArgon2idSettings(t *testing.T) {
	w := newWallet(t, "a", wallet.Options{Iterations: 2, Memory: 64})
	must(t, w.Rekey("other"))
	must(t, w.Save())
	if k := readKDF(t, w.Path()); k.Iterations != 2 || k.Memory != 64 {
		t.Fatalf("kdf %+v", k)
	}
}

func TestUnknownKDFRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	if _, err := wallet.Create(path, "p", wallet.Options{KDF: "scrypt"}); err == nil {
		t.Fatal("made a wallet with an unknown key derivation")
	}
	if _, err := os.Stat(path); err == nil {
		t.Fatal("wrote a wallet file")
	}
}

// TestCraftedKDFSettingsRefused: a damaged or crafted wallet file cannot
// make Unlock allocate gigabytes or run for hours. Each must fail at once.
func TestCraftedKDFSettingsRefused(t *testing.T) {
	w := newWallet(t, "c", wallet.Options{})
	path := w.Path()
	b, err := os.ReadFile(path)
	must(t, err)
	salt := readKDF(t, path).Salt
	cases := map[string]wallet.KDF{
		"argon2 memory":     {Name: wallet.KDFArgon2id, Iterations: 1, Memory: 1 << 30, Threads: 4},
		"argon2 passes":     {Name: wallet.KDFArgon2id, Iterations: 1 << 30, Memory: 32, Threads: 4},
		"argon2 no threads": {Name: wallet.KDFArgon2id, Iterations: 1, Memory: 32},
		"argon2 low memory": {Name: wallet.KDFArgon2id, Iterations: 1, Memory: 8, Threads: 4},
		"pbkdf2 iterations": {Name: wallet.KDFPBKDF2, Iterations: 1 << 40},
		"pbkdf2 zero":       {Name: wallet.KDFPBKDF2},
		"pbkdf2 memory":     {Name: wallet.KDFPBKDF2, Iterations: 1, Memory: 32},
		"unknown":           {Name: "md5", Iterations: 1},
		"short salt":        {Name: wallet.KDFArgon2id, Iterations: 1, Memory: 32, Threads: 4, Salt: []byte{1}},
	}
	for name, k := range cases {
		var f map[string]any
		must(t, json.Unmarshal(b, &f))
		if k.Salt == nil {
			k.Salt = salt
		}
		f["kdf"] = k
		out, err := json.Marshal(f)
		must(t, err)
		must(t, os.WriteFile(path, out, 0o600))
		c, err := wallet.Open(path)
		must(t, err)
		start := time.Now()
		if err := c.Unlock("pass c"); err == nil || errors.Is(err, wallet.ErrPassphrase) {
			t.Errorf("%s: unlock gave %v, want a settings error", name, err)
		}
		if d := time.Since(start); d > time.Second {
			t.Errorf("%s: took %v", name, d)
		}
	}
}
