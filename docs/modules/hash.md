# pkg/hash

## Summary

32-byte hashes, the series' hash number list (1 SHA-256, 2 SHA-512/256, 3
SHA3-256, 4 BLAKE2b-256, 5 BLAKE3; decision D80), the domain tags
of every hashed object (decision D3), and strength, the number of leading
zero bits. Hash functions are values of type `Func`, chosen by a series, so
no other package hard-codes SHA-256.

## API

- `Hash`: `String` (hex), `Short` (8 hex digits), `Compare`, `Less`,
  `Strength`, text marshalling for JSON; `Parse`, `MustParse`.
- `ID` and `Lookup(id) (Func, error)`; `ErrUnknown` for a number this node
  does not implement, which makes a series unreadable here. `IDs()` lists
  the known numbers, `ID.Name` and `ByName` map to and from names.
- `Func(parts ...[]byte) Hash` hashes the concatenation of its parts.
- Tags: `TagAddress`, `TagTx`, `TagEntry`, `TagPool`, `TagSeries`,
  `TagWitness`, `TagIssuer`.

## Extending

To add a hash function, give it the next unused number, a name and a
constructor in the `known` table, with a known-answer test, and document
it in [../cryptography.md](../cryptography.md). Its output must be 32
bytes. Never reuse or renumber.
