# pkg/series

## Summary

A series: the published rules every mint in it follows. `Params` is the
human-editable form (also JSON); `New` validates it into a `*Series`, whose
id is `H("PoO-series" ∥ encode(params))` (decision D4). The series answers
every series-tier question the other packages ask: which hash, how many
mints, which pool a serial is in, pool ids, minimum strengths, whether entry
hashes count as credit, which policies are allowed, and the issuer.

## API

- `Params`, `Encode`, `DecodeParams`; `New(params)` and
  `NewWithRegistry(params, reg)`; errors `ErrInvalid`, `ErrUnreadable`.
- `ID`, `Hash`, `PoolCount`, `PoolOf(serial)`, `PoolRange(k)`, `PoolID(k)`,
  `Mint(serial)`, `EntriesHaveNonce`, `MinEntryStrength`, `EntryHashCredit`,
  `AllowsPolicyType`, `AllowsKeyAlg`, `MaxWitnessSize`, `CheckWitness`,
  `Address`, `IssuerMessageTID`, `CheckIssuer`.
- `MintID{Series, Serial}`.

## Guarantees

A `*Series` is immutable and valid: mints, supply and pool size are above
zero, the hash is known, and every listed type and algorithm is known to the
registry. A series naming an unknown hash or type is unreadable, not
invalid: another node may support it.

## Extending

A new series setting changes the byte form, the series_id of every series
and the vectors. Add it at the end, bump the format version, and record a
decision.
