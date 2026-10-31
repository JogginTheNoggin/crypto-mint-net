package policy

import (
	"crypto/sha256"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/encoding"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
)

// ManyWitnesses is how many witnesses an item must carry for CheckAll to
// remember them as one set and satisfy them on every core (ORANGE-014).
// Below it, each witness is checked and remembered on its own.
const ManyWitnesses = 64

// CheckAll checks ws[i] against addrs[i] for transaction tid, in order,
// and returns the index and error of the first that fails, as calling
// CheckCached on each in turn would; -1 and nil when all pass.
//
// For ManyWitnesses or more it gives the same result faster
// (ORANGE-014):
//   - the size, address and allowed checks run on every witness, every
//     time, as in CheckCached;
//   - v remembers the whole set under one key, so a large transaction
//     checked again (pending, then in an entry, then by Check under the
//     node's lock) costs no signature check, and its witnesses do not
//     push every other remembered witness out of v;
//   - the satisfaction functions run on every core. When one fails, the
//     witnesses before it that no worker reached are checked in order,
//     so the error is the first failure's, and a bad witness costs about
//     one check per core more than in order (RED-009).
func (r *Registry) CheckAll(h hash.Func, rules Rules, ws []Witness, addrs []hash.Hash, tid hash.Hash, v *Verified) (int, error) {
	if len(ws) < ManyWitnesses {
		for i, w := range ws {
			if err := r.CheckCached(h, rules, w, addrs[i], tid, v); err != nil {
				return i, err
			}
		}
		return -1, nil
	}
	// The cheap checks first, in order: the satisfaction functions run
	// only on the witnesses before the first that fails them.
	types := make([]Type, len(ws))
	cheap, cheapErr := len(ws), error(nil)
	for i, w := range ws {
		var err error
		switch {
		case rules.MaxWitnessSize() > 0 && w.Size() > rules.MaxWitnessSize():
			err = ErrTooLarge
		case Address(h, w.Policy) != addrs[i]:
			err = ErrAddressMismatch
		default:
			types[i], err = r.allowed(rules, w.Policy)
		}
		if err != nil {
			cheap, cheapErr = i, err
			break
		}
	}
	var k [sha256.Size]byte
	if cheap == len(ws) && v != nil {
		if k = setKey(ws, tid); v.has(k) {
			return -1, nil
		}
	}
	msg := Message(tid)
	passed := make([]bool, cheap)
	var failed atomic.Bool
	workers := min(runtime.GOMAXPROCS(0), max(1, cheap/ManyWitnesses))
	per := (cheap + workers - 1) / max(workers, 1)
	var wg sync.WaitGroup
	for lo := 0; lo < cheap; lo += per {
		hi := min(lo+per, cheap)
		wg.Add(1)
		go func(lo, hi int) {
			defer wg.Done()
			for i := lo; i < hi && !failed.Load(); i++ {
				if types[i].Satisfy(r, ws[i].Policy.Params, ws[i].Data, msg) != nil {
					failed.Store(true)
					return
				}
				passed[i] = true
			}
		}(lo, hi)
	}
	wg.Wait()
	if failed.Load() {
		for i := range cheap {
			if passed[i] {
				continue
			}
			if err := types[i].Satisfy(r, ws[i].Policy.Params, ws[i].Data, msg); err != nil {
				return i, err
			}
		}
	}
	if cheapErr != nil {
		return cheap, cheapErr
	}
	if v != nil {
		v.add(k)
	}
	return -1, nil
}

// setKey is the key under which a Verified list remembers that every
// witness of ws satisfied its policy for tid.
func setKey(ws []Witness, tid hash.Hash) [sha256.Size]byte {
	d := sha256.New()
	d.Write([]byte("PoO-verified-set-v1"))
	d.Write(tid[:])
	enc := encoding.NewWriter(256)
	enc.U64(uint64(len(ws)))
	d.Write(enc.Bytes())
	for _, w := range ws {
		enc.Reset()
		w.Encode(enc)
		d.Write(enc.Bytes())
	}
	var k [sha256.Size]byte
	d.Sum(k[:0])
	return k
}
