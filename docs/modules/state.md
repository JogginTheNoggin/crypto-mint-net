# pkg/state

## Summary

The state of one pool: which portions are unspent and which mints are
claimed. Applying a bundle returns an undo record so a switch can revert it
exactly. An overlay is a cheap child state over a base, used to check an
entry on another branch without touching the selected state.

## API

- `New`, `Overlay`; `Unspent`, `Claimed` (so `*State` is a `tx.View`).
- `Apply(items) Undo`, `ApplyTx(item, *Undo)`, `Revert(items, undo)`.
- `ForEach`, `Portions(serial)` (sorted), `Totals` (per mint), `Len`.
- `Item{TID, Tx}`, `Holding{Input, Portion}`, `Spent`, `Undo`.

## Guarantees

- `Apply` assumes the items were checked; a missing input panics, because it
  means a caller skipped validation.
- `Revert(Apply(x))` restores the exact previous state.
- An overlay never changes its base.
