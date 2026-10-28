# BLUE-005: Preverify only what the checks reach, and latch a failed fsync

| Field | Value |
| --- | --- |
| Fixes | RED-009 (medium), RED-010 (low) |
| Branch | `claude/project-thread-ggo8dt` |
| Base | `claude/m10-watch` @ `deecc80` (code of `0ac8e4e`, Source Round 4 Part C) |
| Decisions | D107 (corrects D105), D108 |

## Summary

Two fixes from Source Round 4. Checking signatures before the node's lock
(ORANGE-004) used to verify every witness of whatever a peer sent, before
the cheap checks and without stopping at a failure. Now it runs the cheap
checks first, verifies each witness only against the address it must
satisfy, and stops at the first bad one, which costs the peer an offence
(RED-009). A failed disk sync now stops that pool's store instead of
being retried and reported as fine (RED-010). No byte form, wire or
consensus change: every item gets the same result as before.

## What changed

- **RED-009.** `pkg/tx`: `CheckForm` (`CheckStateless` without a claim's
  witnesses), `Owners` (`CheckAgainst` short of the witnesses, returning
  each witness's address), and `Preverify(s, t, tid, owners) error`
  (witnesses in order through `CheckCached`, stopping at the first
  failure); `CheckStateless` and `CheckAgainst` are built from them.
  `pkg/entry`: `Owners`. `pkg/mempool`: `Pool.Owners` (`Add`'s checks
  before a signature); `Add` wraps a check's error with `%w`.
  `pkg/ledger`: `Pool.EntryOwners`, and `AddEntry` checks the parent
  before any claim's witness. `pkg/engine`: `PreverifyTx` and
  `PreverifyEntry` use them under the brief lock and verify outside it.
  `pkg/p2p`: a pushed transaction failing with `tx.ErrWitness` is an
  invalid item. `policy.Registry.Preverify` and `series.Series.Preverify`
  are removed.
- **RED-010.** `pkg/store`: `Log.err` latches the first failed write or
  sync (`ErrFailed`, `Log.Err`); `AppendLater`, `Durable`, `Rewrite` and
  `Close` honour it, and `Durable` never retries. `fileSync` is the seam
  for the test. `pkg/node`: `storeFailed` logs a stopped pool once.
- Docs: D107, D108, D105's text, `docs/modules/engine.md`, `policy.md`,
  `store.md`.

## Tests added

- `pkg/engine`: `TestPreverifyOnlyWhatHandleWouldVerify`, under a
  registry whose policy types count satisfaction calls: 200 witnesses on
  missing inputs cost none and none is remembered; a spend with a bad
  first witness costs one ahead and one under the lock and fails with
  `tx.ErrWitness`; an entry on an unknown parent costs none; a valid
  spend is verified once ahead and not again.
- `pkg/p2p`: `TestBadWitnessTxIsOffence`: a spend of a missing portion is
  no offence, a spend with a bad witness is.
- `pkg/store`: `TestFailedSyncIsLatched`.

## How Grey can check it

```sh
go test -race -run 'Preverify' -v ./pkg/engine
go test -race -run 'BadWitnessTx|InvalidEntries' -v ./pkg/p2p
go test -race -run 'FailedSync|GroupCommit' -v ./pkg/store
go test -race ./...
```

Worth a benchmark next to ORANGE-004's numbers: preverify now takes the
engine lock for the input lookups, which are map reads, so throughput
should hold, but Grey or Orange may want to measure it.
