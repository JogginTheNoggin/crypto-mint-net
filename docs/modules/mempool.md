# pkg/mempool

## Summary

The pending transactions of one pool: valid against the selected ledger and
not yet in it. First come, first served: a transaction spending a portion
that a pending one already spends is refused. Transactions carried back
after a switch take precedence and may wait on each other (decision D13).

## API

- `New(series, limit)`; `DefaultLimit`.
- `Ledger` interface: a `tx.View` of the selected state plus `Locate(tid)`.
- `Add(l, t, tid)`: refuses `ErrKnown`, `ErrInSelected`, `ErrConflict`,
  `ErrDependsOnPending`, `ErrFull`, or `ErrInvalid` (wrapping the reason).
- `AddCarried(l, items)`: puts carried transactions back, evicting
  conflicting pending ones; dependents wait.
- `Revalidate(l)`: after the selected ledger changes, drops what is now
  included or invalid and promotes waiting transactions; oldest first.
- `Ready` (in arrival order, ready to bundle), `Waiting`, `Remove`, `Has`,
  `Get`, `Len`.
