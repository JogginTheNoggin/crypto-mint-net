package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wallet"
)

// RED-039 (D146): when the record of something already sent cannot be
// saved, it is printed and the error says not to send it again; when
// nothing was sent, the send's error is kept.
func TestSaveSentSaysDoNotRepeat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "w.json")
	w, err := wallet.Create(path, "p", wallet.Options{Iterations: 1, Memory: 32, Gap: 2})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil { // the save will be refused
		t.Fatal(err)
	}
	var out strings.Builder
	sent := &wallet.Sent{Amount: 30, Legs: []wallet.Leg{{Amount: 30, State: wallet.StatePending}}}
	err = saveSent(w, &out, "payment", sent, nil)
	if err == nil || !strings.Contains(err.Error(), "Do not run the command again") || strings.Contains(err.Error(), "nothing was saved") {
		t.Fatalf("a sent payment not saved: %v", err)
	}
	if !strings.Contains(out.String(), `"amount": 30`) {
		t.Fatalf("the record was not printed: %q", out.String())
	}
	out.Reset()
	refused := errors.New("node refused it")
	dropped := &wallet.Sent{Amount: 30, Legs: []wallet.Leg{{Amount: 30, State: wallet.StateDropped}}}
	if err := saveSent(w, &out, "payment", dropped, refused); !errors.Is(err, refused) || out.Len() != 0 {
		t.Fatalf("nothing sent: %v, printed %q", err, out.String())
	}
}
