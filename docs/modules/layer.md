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

## Notes

Grinding here is for tests, the demo and small builders. A real worker would
run outside the node and submit layers through the engine.
