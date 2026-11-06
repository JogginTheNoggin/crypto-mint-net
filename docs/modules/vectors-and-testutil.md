# pkg/vectors and pkg/testutil

## Summary

`pkg/testutil` gives tests and the demo deterministic keys, the example
series and short builders for signed claims and spends. `pkg/vectors`
computes the published test vectors from them and a golden test keeps them
fixed, so any change to a byte form or hash fails loudly.

## testutil

- `ExampleParams` / `ExampleSeries`: 8 mints, supply 100, pool size 4,
  SHA-256, threshold policy, Ed25519, no minimum strengths.
- `NewKey(label)`: Ed25519 seed `SHA-256("PoON-test-key:" + label)`. These
  keys are public by construction; never use them for real value.
- `NewKeys(s)` with `Get`, `Addr`, `Claim(serial, outs...)`,
  `Spend(serial, ins, outs...)`, `Out(tx, index, owner)`, `Pay`; `P(to,
  amount)`; `Must`.

## vectors

`go test ./pkg/vectors` compares `Compute()` with
`testdata/vectors.json`. After an intended change, record a decision and run
`go test ./pkg/vectors -update`. The values are listed in
[../protocol/encoding.md](../protocol/encoding.md#test-vectors).

The vectors of a variable supply series (format version 2, D163) are in
`testdata/variable.json`, computed by `ComputeVariable` and checked by
`TestGoldenVariable`. They are kept apart, so the frozen v1 file never
changes.
