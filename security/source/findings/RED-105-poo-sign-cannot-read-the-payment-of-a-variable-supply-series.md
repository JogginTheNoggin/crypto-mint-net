# RED-105: `poo sign` cannot read a payment in a variable-supply series: it stops with "input too short"

| Field | Value |
| --- | --- |
| Severity | Low (it fails closed) |
| Status | open |
| Found | Source Round 35 Part B, on `main` @ `866a4ac` (D193) |
| Area | `pkg/cli/sign.go` (`tx.Decode`), `pkg/tx` (`DecodeFrom` reads the `Part` only if `series.IsVariableID(id)`) |
| For Black | Not a rule: the byte form is unchanged. It is a tool gap. |

## What is wrong

A transaction in a variable-supply series carries a part number after the
series id. `tx.DecodeFrom` reads it only when `series.IsVariableID` knows the
series id, and that table is filled when a series is *built in the process*.
`poo sign` is meant for an offline machine with no node and no series, so
the table is empty, the part is not read, and every later field is read one
byte off. The command stops with a message that says nothing about the cause.

## How I checked it

`pkg/cli` tests (throwaway, removed; copy kept with Red's notes), two
processes: one built a variable series (part size 4, an issuer), wrote a
payment file with one leg and a key file; a second, new process ran
`poo sign -show` and `poo sign` on it:

```
err=leg of mint 2: tx: decode: encoding: input too short
```

Both modes fail, so a cold-key holder of variable-supply coins has no way
to sign offline.

## Suggested fix (Blue's call)

- Put the series' supply type and part size (or just "variable") in the
  `Unsigned` file next to `series` and `hash`, and have `poo sign` register
  or pass it to the decoder; or decode with the series from `node.json`
  when it has one, and say "unknown series, cannot read its transactions"
  when it does not.
- Whatever it does, replace "input too short" for a leg with a message that
  names the likely cause.

## Not tested

A file made by the real `wallet pay` for a variable series (my file was made
by hand, but with the same encoder).
