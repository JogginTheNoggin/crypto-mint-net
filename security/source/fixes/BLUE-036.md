# BLUE-036: What poo sign shows and checks; raw keys self-tested; key files never echoed; no unrecorded pay; ledger files read twice

| Field | Value |
| --- | --- |
| Fixes | RED-104 (medium), RED-105, RED-106, RED-107, RED-108 (low), GUI-032 (Source part), GUI-GREEN-061 (Source asks 1 and 3) |
| Branch | `claude/project-thread-ggo8dt` (PR against `main`) |
| Base | `main` @ `58273f7` (Source Round 35 Part C), with GREEN-060 (#289) and ORANGE-045 (#286) merged in |
| Decisions | D194 |
| With | GREEN-060 (#289) and ORANGE-045 (#286), both carried: they change the same `sign.go`, `pay.go` and `main.go`, and merged cleanly. |
| Review | No consensus change. The transaction byte form is unchanged; `variable` is a field in a tool's file. For White: payment files gain `"variable"`, and an older `poo sign` ignores it (it still fails on variable series, as before). `poo ledger import` from a pipe now refuses a file whose entries come first. |

## Summary

- **RED-104.** `poo sign` shows `series`, `hash` and `spends`; a series
  mismatch is refused; signatures are checked before writing.
- **RED-105.** `Unsigned.Variable` and `tx.DecodeVariable`.
- **RED-106.** `NewRawSigner` self-tests every key.
- **RED-107.** No spool: entries-first files are read twice; pipes refused.
- **RED-108.** The whole header reaches the dry run's checks.
- **GUI-032.** Key and seed file errors carry no path or value; `key:`
  prompt for raw keys.
- **GUI-GREEN-061.** Pay with no `-out` that needs other signatures is
  refused before anything is recorded; `passwd` refuses watch-only first.

## Consensus

None.

## Tests

- `pkg/cli/blue036_test.go`: `TestBlue036ColdKeySteps`, `TestBlue036LedgerFileOrder`
- `pkg/wallet/blue036_test.go`: `TestBlue036VariableLegRead`
- `pkg/policy/blue036_test.go`: `TestBlue036RawKeySelfTested`
- Each fails on the code before the fix (the CLI tests by build, then the pay, passwd and echo parts run against the old `wallet.go` and `node.go`).
