package engine

import (
	"errors"
	"fmt"

	"github.com/JogginTheNoggin/crypto-mint-net/pkg/hash"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/ledger"
	"github.com/JogginTheNoggin/crypto-mint-net/pkg/tx"
)

// ErrPoolInUse: Restore was asked for a pool that already holds entries.
var ErrPoolInUse = errors.New("engine: the pool already holds entries; a snapshot restores only an empty one")

// Snapshot writes pool k's ledger snapshot (ledger.Pool.Snapshot) to
// ledgerRec, then gives its pending transactions, oldest first, to
// pending, all under the engine's lock, so both describe the same moment
// (D178). It changes nothing.
func (e *Engine) Snapshot(k PoolKey, ledgerRec func([]byte) error, pending func(*tx.Tx) error) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failed != nil {
		return e.failed
	}
	p, ok := e.pools[k]
	if !ok {
		return ledger.ErrNotSnapshottable
	}
	if err := p.ledger.Snapshot(ledgerRec); err != nil {
		return err
	}
	for _, it := range p.pending.Ready() {
		if err := pending(it.Tx); err != nil {
			return err
		}
	}
	return nil
}

// Restore gives pool k the ledger read from a snapshot (ledger.Restore),
// with the pool's configuration now. The pool must hold no entry yet. On
// an error the pool is left as it was. Pending transactions are not part
// of it: the caller gives them again with HandleTx (D178).
func (e *Engine) Restore(k PoolKey, next func() ([]byte, error)) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.failed != nil {
		return e.failed
	}
	p, created, err := e.poolFor(k)
	if err != nil {
		return err
	}
	if p.ledger.Len() > 0 {
		return ErrPoolInUse
	}
	lc := e.cfg.Ledger
	if e.cfg.Source != nil {
		lc.Source = e.cfg.Source(k)
	}
	pl, err := ledger.Restore(p.s, k.Index, lc, next)
	if err != nil {
		return err
	}
	p.ledger = pl
	if created {
		e.pools[k] = p
	}
	return nil
}

// Fingerprint returns pool k's ledger fingerprint (ledger.Pool.Fingerprint),
// for tests and -reverify.
func (e *Engine) Fingerprint(k PoolKey) (hash.Hash, error) {
	var out hash.Hash
	err := e.Ledger(k, func(p *ledger.Pool) { out = p.Fingerprint() })
	if err != nil {
		return out, fmt.Errorf("engine: %w", err)
	}
	return out, nil
}

// Forget drops pool k's state, as if the engine had never taken anything
// for it: a restore whose snapshot turned out unusable after the ledger
// was built is undone this way before the log is replayed.
func (e *Engine) Forget(k PoolKey) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.pools, k)
}
