package cli

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wallet"
)

// RED-042 (D148): while another command holds the wallet, status, which
// changes no leg here, answers without waiting, and a command that must
// wait to save says so on its error output, then saves once the holder
// lets go.
func TestHeldWalletWaitersSayWhy(t *testing.T) {
	createIterations = 1
	defer func() { createIterations = 0 }()
	t.Setenv("POON_PASSPHRASE", "pw")
	root := t.TempDir()
	r := []string{"-root", root}
	cli(t, nil, append([]string{"init", "-example", "-listen", "127.0.0.1:0", "-p2p", "127.0.0.1:0"}, r...)...)
	stop := startNode(t, filepath.Join(root, "policies", "default"))
	defer stop()
	path := filepath.Join(root, "w.json")
	aw := append([]string{"-wallet", path}, r...)
	cli(t, nil, append([]string{"wallet", "create"}, aw...)...)
	cli(t, nil, append([]string{"wallet", "claim", "-serial", "5"}, aw...)...)

	holder, err := wallet.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	release, err := holder.Hold()
	if err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	defer once.Do(release)

	// status asks the node with the wallet held elsewhere and does not wait.
	done := make(chan error, 1)
	go func() {
		var out bytes.Buffer
		done <- Run(context.Background(), append([]string{"wallet", "status"}, aw...), Stdio{Out: &out, Err: &out})
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("status: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("status waited for the held wallet")
	}

	// address must save: it says it waits, then saves after the release.
	var mu sync.Mutex
	var errOut bytes.Buffer
	go func() {
		var out bytes.Buffer
		done <- Run(context.Background(), append([]string{"wallet", "address", "-label", "later"}, aw...), Stdio{Out: &out, Err: lockedWriter{&mu, &errOut}})
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		mu.Lock()
		said := strings.Contains(errOut.String(), "waiting for another poon wallet command")
		mu.Unlock()
		if said {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the waiting command said nothing")
		}
		time.Sleep(20 * time.Millisecond)
	}
	select {
	case err := <-done:
		t.Fatalf("address finished while the wallet was held: %v", err)
	default:
	}
	once.Do(release)
	if err := <-done; err != nil {
		t.Fatalf("address after the release: %v", err)
	}
}

type lockedWriter struct {
	mu *sync.Mutex
	w  *bytes.Buffer
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}
