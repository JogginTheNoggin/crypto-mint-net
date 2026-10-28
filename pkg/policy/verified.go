package policy

import (
	"crypto/sha256"
	"sync"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// DefaultVerifiedSize is how many satisfied witnesses a Verified list
// remembers by default.
const DefaultVerifiedSize = 1 << 14

// Verified remembers witnesses whose policy type's satisfaction function
// has passed, so a node that checks the same transaction again (pending,
// then in an entry, then in a rival entry or after a switch) does not
// verify its signatures again. It is a node's local cache, never part of
// any rule: only passes are remembered, each under a key that covers
// everything the satisfaction function reads (the policy, the witness
// data and the tid), and Registry.CheckCached still runs the size,
// address and allowed checks every time. It is a fixed-size ring that
// forgets its oldest key, and is safe for concurrent use.
type Verified struct {
	mu   sync.Mutex
	size int
	ring [][sha256.Size]byte // grows to size, then overwrites its oldest key
	next int
	set  map[[sha256.Size]byte]struct{}
}

// NewVerified returns a list that remembers up to size witnesses, or
// DefaultVerifiedSize when size is not positive.
func NewVerified(size int) *Verified {
	if size <= 0 {
		size = DefaultVerifiedSize
	}
	return &Verified{size: size, set: map[[sha256.Size]byte]struct{}{}}
}

func verifiedKey(w Witness, tid hash.Hash) [sha256.Size]byte {
	enc := encoding.NewWriter(64 + w.Size() + hash.Size)
	enc.Raw([]byte("PoO-verified-v1"))
	w.Encode(enc)
	enc.Raw(tid[:])
	return sha256.Sum256(enc.Bytes())
}

func (v *Verified) has(k [sha256.Size]byte) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	_, ok := v.set[k]
	return ok
}

func (v *Verified) add(k [sha256.Size]byte) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if _, ok := v.set[k]; ok {
		return
	}
	if len(v.ring) < v.size {
		v.ring = append(v.ring, k)
	} else {
		delete(v.set, v.ring[v.next])
		v.ring[v.next] = k
		v.next = (v.next + 1) % v.size
	}
	v.set[k] = struct{}{}
}

// Len returns how many witnesses are remembered.
func (v *Verified) Len() int {
	v.mu.Lock()
	defer v.mu.Unlock()
	return len(v.set)
}
