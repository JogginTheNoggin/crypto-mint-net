package wallet_test

import (
	"errors"
	"testing"
	"time"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/wallet"
)

// RED-039 (D146): a payment holds the wallet from before it is sent until
// it is saved. What another command saved before the hold is read again
// and kept, the payment's record is never refused, and its change address
// is not one the other command gave out. A save that read the file before
// the hold waits for it, then is refused (D145), having sent nothing.
func TestPaymentHoldsTheWallet(t *testing.T) {
	n := newNode(t)
	alice := newWallet(t, "alice", wallet.Options{})
	bob := newWallet(t, "bob", wallet.Options{})
	c, err := alice.Claim(exS, n, 4)
	must(t, err)
	n.mine(t, 1)
	_, err = alice.Refresh(exS, n, c)
	must(t, err)
	must(t, alice.Save())
	path := alice.Path()

	pay, err := wallet.Open(path)
	must(t, err)
	must(t, pay.Unlock("pass alice"))
	other, err := wallet.Open(path) // a request made while pay waited at its prompt
	must(t, err)
	must(t, other.Unlock("pass alice"))
	asked, err := other.Request(exS, 7, "tea")
	must(t, err)
	must(t, other.Save())
	late, err := wallet.Open(path) // read before the hold, saved during it
	must(t, err)
	must(t, late.Unlock("pass alice"))

	release, err := pay.Hold()
	must(t, err)
	req, err := bob.Request(exS, 30, "coffee")
	must(t, err)
	p, _, err := pay.PayRequest(exS, n, req)
	must(t, err)
	_, err = late.Request(exS, 1, "late")
	must(t, err)
	done := make(chan error, 1)
	go func() { done <- late.Save() }()
	select {
	case err := <-done:
		t.Fatalf("a save went through during the hold: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := pay.Save(); err != nil {
		t.Fatalf("the payment's save was refused: %v", err)
	}
	release()
	if err := <-done; !errors.Is(err, wallet.ErrChanged) {
		t.Fatalf("the save that read before the hold: %v, want ErrChanged", err)
	}
	release() // twice is harmless

	w, err := wallet.Open(path)
	must(t, err)
	if len(w.Sent()) != 2 || w.Sent()[1].Legs[0].TID != p.Legs[0].TID {
		t.Fatalf("the payment's record is missing: %+v", w.Sent())
	}
	if _, ok := w.Find(asked.Address); !ok {
		t.Fatal("the request saved before the hold was lost")
	}
	if _, ok := w.Find(req.Address); ok {
		t.Fatal("bob's request is in alice's wallet")
	}
	if p.Legs[0].Change != 70 {
		t.Fatalf("change %d", p.Legs[0].Change)
	}
	// Each key is used for one thing: the change did not take the key the
	// request saved before the hold had taken.
	labels := map[string]int{}
	for _, k := range w.Keys() {
		if k.Used {
			labels[k.Label]++
		}
	}
	if labels["change"] != 1 || labels["claim of mint 4"] != 1 || len(labels) != 3 {
		t.Fatalf("used keys by label %v", labels)
	}
}

// RED-042 (D148): a Hold or Save that finds the wallet held calls OnBusy
// once before it waits, so the command can say why; one that finds it
// free does not.
func TestHoldSaysWhenBusy(t *testing.T) {
	w := newWallet(t, "w", wallet.Options{})
	other, err := wallet.Open(w.Path())
	must(t, err)
	busy := 0
	other.OnBusy(func() { busy++ })
	release, err := other.Hold()
	must(t, err)
	release()
	if busy != 0 {
		t.Fatal("OnBusy called with the wallet free")
	}
	release, err = w.Hold()
	must(t, err)
	done := make(chan error, 1)
	go func() { done <- other.Save() }()
	time.Sleep(100 * time.Millisecond)
	release()
	if err := <-done; err != nil {
		t.Fatalf("the waiting save: %v", err)
	}
	if busy != 1 {
		t.Fatalf("OnBusy called %d times while held, want 1", busy)
	}
}
