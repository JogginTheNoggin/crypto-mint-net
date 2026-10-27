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
  included, then rechecks the rest oldest first until nothing changes:
  invalid ones are dropped and waiting ones whose parents are now in the
  ledger become pending, a whole chain in one call (decision D72).
- `Ready` (in arrival order, ready to bundle), `Waiting`, `Remove`, `Has`,
  `Get`, `Len`.
