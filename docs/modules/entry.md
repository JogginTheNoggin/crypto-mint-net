# pkg/entry

## Summary

Entries: bundles of transactions of one pool, each pointing to its parent
entry (or the pool_id). The entry hash is
`H("PoON-entry" ∥ parent ∥ tids [∥ nonce])`, with the nonce only when the
series sets a minimum entry strength (decision D5). Transactions are in
canonical order, by serial and then tid, and no transaction may depend on
another in the same entry.

## API

- `Entry{Parent, Nonce, Txs}`; `TIDs`, `HashOf`, `Hash`, `Encode`,
  `EncodeTo`, `Decode`, `DecodeFrom`, `Items`, `SortItems`.
- `CheckStructure(s, pool, e) (hash, items, error)`: nonce rule, every
  transaction in the right series and pool (`ErrSeriesMismatch`,
  `ErrWrongPool`), strictly increasing order (`ErrOrder`), stateless
  validity (`ErrInvalidTx`), minimum entry strength (`ErrWeak`).
- `CheckAgainst(s, view, items)`: each transaction valid against the state
  after the parent, and no two spending the same portion or claiming the
  same mint (`ErrConflict`). A dependency inside the entry shows up as a
  missing input.
- `Build(s, parent, txs, maxTries)`: sorts and, if needed, grinds the nonce;
  `ErrNotFound` if no nonce is strong enough. `BuildWorkers` searches on
  several workers, and `BuildWith(ctx, newHasher, s, parent, txs, maxTries,
  workers, tried)` with any hasher, stopping when ctx ends and counting
  tries (M11). Every one finds the same nonce as `Build`.

## Guarantees

Empty entries are valid (decision D16).
