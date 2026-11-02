package wallet_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wallet"
)

// TestSaveNeverUndoesAnotherChange is GUI-019: two commands open the same
// wallet; one changes the passphrase and saves; the other, which waited
// at its prompt, saves after it. The second save is refused, so the new
// passphrase stands (D145).
func TestSaveNeverUndoesAnotherChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	if _, err := wallet.Create(path, "old", wallet.Options{Iterations: 1, Gap: 2}); err != nil {
		t.Fatal(err)
	}
	passwd, err := wallet.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	label, err := wallet.Open(path) // waiting at its prompt
	if err != nil {
		t.Fatal(err)
	}
	must(t, passwd.Unlock("old"))
	must(t, passwd.Rekey("new"))
	must(t, passwd.Save())
	must(t, label.Unlock("old"))
	must(t, label.Rekey("late"))
	if err := label.Save(); !errors.Is(err, wallet.ErrChanged) {
		t.Fatalf("a stale save: %v, want ErrChanged", err)
	}
	w, err := wallet.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Unlock("new"); err != nil {
		t.Fatalf("the new passphrase was undone: %v", err)
	}
	// A wallet saves again over what it wrote itself.
	must(t, passwd.Save())
	must(t, passwd.Save())

	// A new wallet whose file appeared meanwhile is not written over.
	other := filepath.Join(t.TempDir(), "x.json")
	must(t, os.WriteFile(other, []byte("{}"), 0o600))
	if _, err := wallet.Create(other, "p", wallet.Options{Iterations: 1}); !errors.Is(err, wallet.ErrExists) {
		t.Fatalf("create over a file: %v", err)
	}
	// A wallet whose file was removed is not written back.
	must(t, os.Remove(path))
	if err := passwd.Save(); !errors.Is(err, wallet.ErrChanged) {
		t.Fatalf("save over a removed file: %v", err)
	}
}

// TestConcurrentSavesOneWins: of many commands that opened the same file
// and save at once, exactly one writes, under the lock (GUI-019, D145).
func TestConcurrentSavesOneWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	if _, err := wallet.Create(path, "p", wallet.Options{Iterations: 1, Gap: 2}); err != nil {
		t.Fatal(err)
	}
	const n = 8
	ws := make([]*wallet.Wallet, n)
	for i := range ws {
		w, err := wallet.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		must(t, w.Unlock("p"))
		must(t, w.Rekey(fmt.Sprintf("p%d", i)))
		ws[i] = w
	}
	var wg sync.WaitGroup
	errs := make([]error, n)
	for i, w := range ws {
		wg.Add(1)
		go func() { defer wg.Done(); errs[i] = w.Save() }()
	}
	wg.Wait()
	won := 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, wallet.ErrChanged):
			t.Fatal(err)
		}
	}
	if won != 1 {
		t.Fatalf("%d saves wrote, want 1", won)
	}
}
