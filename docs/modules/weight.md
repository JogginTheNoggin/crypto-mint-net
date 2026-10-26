# pkg/weight

## Summary

Credit and weight as immutable arbitrary-precision integers (decision D10).
A layer of strength s credits 2^s; a ledger's weight is the sum of the credit
on it. Weights never go negative and are compared exactly.

## API

`Credit(strength)`, `Zero`, `Add`, `Sub` (panics if the result would be
negative, which would be a bug), `Cmp`, `Greater`, `IsZero`, `Uint64`,
`Bytes`/`FromBytes` (minimal big-endian, refusing leading zeros), text
marshalling as a decimal string.

## Guarantees

Values are never mutated after creation, so they can be shared freely
between the ledger, the engine and callers.
