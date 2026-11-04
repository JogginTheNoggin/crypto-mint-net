# pkg/tx

## Summary

Transactions: transfers within one mint that spend whole portions and create
new ones of the same total. The tid is `H("PoO-tx" ∥ body)`; witnesses sit
outside it. Validity is split into stateless checks (anything that needs
only the series) and checks against a `View` of pool state, so the cheap
checks can run before any state lookup.

## API

- `Tx{Mint, Inputs, Outputs, Witnesses}`, `Input{TID, Index}`,
  `Output{Address, Amount}`; `Body`, `TID`, `Encode`, `EncodeTo`, `Decode`,
  `DecodeFrom`, `OutputSum`, `IsClaim`. A variable series' transaction
  carries its part after the series in the body (D166); a fixed one has
  the v1 body, byte for byte.
- `CheckMint(s, m)`: the mint's part and serial against the series
  (`ErrWrongPart`, `ErrSerialRange`, `ErrWaiting` for a part not released
  at this node: it waits and is never invalid).
- `Portion{Serial, Address, Amount}` and `View` (`Unspent`, `Claimed`).
- `CheckStateless(s, t, tid)`: series, serial in range, at least one output,
  no zero amounts, no repeated output address, no overflow, no repeated
  input; a claim must output exactly the supply with one valid witness per
  output; a spend must have one witness per input.
- `CheckAgainst(s, t, tid, view)`: a claim needs an unclaimed mint; a spend
  needs a claimed mint, every input unspent and of the same mint, inputs
  equal to outputs, and each witness valid for the spent portion's address.
- `Check` runs both. Errors are sentinels (`ErrConservation`,
  `ErrMissingInput` and so on) for `errors.Is`.

## Guarantees

Witnesses are verified last, after every cheaper rule.

## Tests

`tx_test.go` covers every error path and the round trip of the encoding.
