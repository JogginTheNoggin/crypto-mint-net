# pkg/layer

## Summary

Layers: unsigned work on an entry. A layer is an entry hash and an 8-byte
nonce; its hash is `H(entry hash ∥ nonce)` (decision D6), its strength the
leading zero bits of that hash, and its credit 2^strength. A layer adds
credit to every ledger that contains its entry.

## API

`Layer{Entry, Nonce}`, `Hash`, `Strength`, `Credit`, `Encode`, `EncodeTo`,
`Decode`, `DecodeFrom`, `Size` (40); `Grind(h, entry, start, tries)` returns
the strongest layer in a nonce range; `GrindTo` stops at a target strength.
`GrindHash` does the same with the hash's `hash.Grinder` (ORANGE-006), and
`GrindWith(ctx, newHasher, entry, start, tries, workers, tried)` with any
hasher on several workers (ORANGE-007): it finds what `Grind` finds, looks
at ctx every 4096 tries, and returns the best layer so far with ctx's
error. Tries 0 means until ctx ends.

## Notes

Grinding here is for tests, the demo and small builders. A real worker would
run outside the node and submit layers through the engine.
