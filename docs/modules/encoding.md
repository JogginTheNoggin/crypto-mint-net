# pkg/encoding

## Summary

The canonical byte form (byte form v1, decision D2) that every hash and every
message is computed over. A `Writer` appends big-endian integers,
length-prefixed bytes and counted lists; a `Reader` reads them back with
limits and refuses anything left over. Nothing in this package knows about
the design's objects.

## API

- `NewWriter(capacity)` then `U8`, `U16`, `U32`, `U64`, `Bool`, `VarBytes`,
  `Count`, `Raw`; `Bytes()` returns the result. Calls chain.
- `NewReader(b)` then the same reads plus `Into(dst)`. `VarBytes(max)` and
  `Count(max, minItemSize)` refuse sizes that are too large or cannot fit
  the remaining input. `Fail` records a caller's error.
- `Err()` holds the first error; reads after an error return zero values.
  `Finish()` returns the error, or an error if bytes remain.

## Guarantees

- One value has exactly one encoding: booleans other than 0 and 1 are
  refused, and trailing bytes are an error.
- A hostile length or count never causes a large allocation.

## Extending

Add primitives only if an object cannot be built from these. Any change here
changes the vectors.
