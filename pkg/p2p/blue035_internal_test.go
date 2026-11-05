package p2p

import (
	"testing"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/peers"
)

// RED-103: each inbox queue is bound in bytes as well as messages, and an
// empty queue still takes a message larger than the bound.
func TestBlue035InboxBoundInBytes(t *testing.T) {
	b := newInbox([4]int{1, 1, 1, 1}, 1024, 100)
	if !b.put(peers.CatUnlisted, job{size: 500}) {
		t.Fatal("an empty queue refused a large message")
	}
	if b.put(peers.CatUnlisted, job{size: 1}) {
		t.Fatal("a queue over its bytes took more")
	}
	if !b.put(peers.CatPriority, job{size: 60}) || !b.put(peers.CatPriority, job{size: 40}) || b.put(peers.CatPriority, job{size: 1}) {
		t.Fatal("the byte bound is not per queue, or not exact")
	}
	b.round(t.Context()) // takes one from each queue
	if b.bytes[peers.CatUnlisted] != 0 || b.bytes[peers.CatPriority] != 40 {
		t.Fatalf("bytes after a round %v", b.bytes)
	}
	if !b.put(peers.CatPriority, job{size: 60}) {
		t.Fatal("room freed by a round was not reused")
	}
}
